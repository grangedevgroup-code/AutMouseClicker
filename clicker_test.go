package main

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeInjector struct {
	mu      sync.Mutex
	events  []string
	failAt  int
	closed  bool
	pressed map[Button]bool
}

func (f *fakeInjector) record(kind string, b Button) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAt > 0 && len(f.events)+1 >= f.failAt {
		return errors.New("injector failed")
	}
	f.events = append(f.events, kind+" "+b.String())
	if f.pressed == nil {
		f.pressed = map[Button]bool{}
	}
	f.pressed[b] = kind == "press"
	return nil
}

func (f *fakeInjector) Press(b Button) error   { return f.record("press", b) }
func (f *fakeInjector) Release(b Button) error { return f.record("release", b) }
func (f *fakeInjector) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeInjector) snapshot() ([]string, map[Button]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	held := map[Button]bool{}
	for b, down := range f.pressed {
		held[b] = down
	}
	return append([]string(nil), f.events...), held
}

func TestIntervalAddsTheFieldsAndNeverGoesBelowTheMinimum(t *testing.T) {
	cases := []struct {
		h, m, s, ms int
		want        time.Duration
	}{
		{0, 0, 5, 0, 5 * time.Second},
		{1, 2, 3, 450, time.Hour + 2*time.Minute + 3*time.Second + 450*time.Millisecond},
		{0, 0, 0, 0, MinInterval},
		{0, 0, 0, 5, MinInterval},
		{0, 0, 0, 100, 100 * time.Millisecond},
	}
	for _, c := range cases {
		if got := Interval(c.h, c.m, c.s, c.ms); got != c.want {
			t.Errorf("Interval(%d,%d,%d,%d) = %v, want %v", c.h, c.m, c.s, c.ms, got, c.want)
		}
	}
}

func TestAFailedInjectorIsReplacedOnTheNextStart(t *testing.T) {
	var opened []*fakeInjector
	c := NewClicker(func() (Injector, error) {
		f := &fakeInjector{}
		if len(opened) == 0 {
			f.failAt = 1
		}
		opened = append(opened, f)
		return f, nil
	})
	stopped := make(chan error, 1)
	c.OnStop = func(err error) { stopped <- err }
	if err := c.Start(Settings{Interval: 10 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-stopped:
		if err == nil {
			t.Fatal("the injector error was not reported")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("never stopped after the injector failed")
	}
	if !opened[0].closed {
		t.Fatal("the failed injector was not closed")
	}
	if err := c.Start(Settings{Interval: time.Hour}); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	deadline := time.Now().Add(time.Second)
	for c.Clicks() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(opened) != 2 || c.Clicks() != 1 {
		t.Fatalf("opened %d injectors, %d clicks; want a fresh injector that clicks", len(opened), c.Clicks())
	}
}

func TestParseButtonAcceptsAnyCase(t *testing.T) {
	for name, want := range map[string]Button{"left": Left, "RIGHT": Right, "Middle": Middle} {
		got, err := ParseButton(name)
		if err != nil || got != want {
			t.Errorf("ParseButton(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
	if _, err := ParseButton("side"); err == nil {
		t.Error("ParseButton accepted an unknown button")
	}
}

func TestClickerClicksAtTheIntervalAndStopsWithNothingHeld(t *testing.T) {
	f := &fakeInjector{}
	c := NewClicker(func() (Injector, error) { return f, nil })
	if err := c.Start(Settings{Interval: 20 * time.Millisecond, Button: Right}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(Settings{Interval: 20 * time.Millisecond}); !errors.Is(err, ErrRunning) {
		t.Fatalf("second Start returned %v, want ErrRunning", err)
	}
	time.Sleep(110 * time.Millisecond)
	c.Stop()
	if c.Running() {
		t.Fatal("still running after Stop")
	}
	events, held := f.snapshot()
	if n := c.Clicks(); n < 3 || n > 8 {
		t.Fatalf("%d clicks in 110 ms at 20 ms, want 3 to 8", n)
	}
	if len(events) != int(2*c.Clicks()) {
		t.Fatalf("%d events for %d clicks", len(events), c.Clicks())
	}
	for i, e := range events {
		want := "press Right"
		if i%2 == 1 {
			want = "release Right"
		}
		if e != want {
			t.Fatalf("event %d is %q, want %q", i, e, want)
		}
	}
	if held[Right] {
		t.Fatal("the button was left pressed")
	}
}

func TestDoubleClickSendsTwoClicksPerRound(t *testing.T) {
	f := &fakeInjector{}
	c := NewClicker(func() (Injector, error) { return f, nil })
	if err := c.Start(Settings{Interval: time.Hour, Double: true}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for c.Clicks() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	c.Stop()
	events, _ := f.snapshot()
	want := []string{"press Left", "release Left", "press Left", "release Left"}
	if len(events) != len(want) {
		t.Fatalf("events %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events %v, want %v", events, want)
		}
	}
}

func TestAnInjectorErrorStopsClickingAndIsReported(t *testing.T) {
	f := &fakeInjector{failAt: 4}
	reported := make(chan error, 1)
	c := NewClicker(func() (Injector, error) { return f, nil })
	c.OnStop = func(err error) { reported <- err }
	if err := c.Start(Settings{Interval: 10 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-reported:
		if err == nil {
			t.Fatal("OnStop got a nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the error was never reported")
	}
	if c.Running() {
		t.Fatal("still running after an injector error")
	}
}

func TestOpenErrorsAreReturnedAndTheClickerCanBeRetried(t *testing.T) {
	calls := 0
	f := &fakeInjector{}
	c := NewClicker(func() (Injector, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("permission denied")
		}
		return f, nil
	})
	if err := c.Start(Settings{Interval: time.Hour}); err == nil {
		t.Fatal("Start ignored the open error")
	}
	if err := c.Start(Settings{Interval: time.Hour}); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	c.Stop()
	if err := c.Close(); err != nil || !f.closed {
		t.Fatalf("Close = %v, injector closed %v", err, f.closed)
	}
}

func TestRoundsStopTheClickerByItselfAndReportNoError(t *testing.T) {
	f := &fakeInjector{}
	finished := make(chan error, 1)
	c := NewClicker(func() (Injector, error) { return f, nil })
	c.OnStop = func(err error) { finished <- err }
	if err := c.Start(Settings{Interval: 10 * time.Millisecond, Double: true, Rounds: 3}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("finished with %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("never finished")
	}
	if c.Running() || c.Clicks() != 6 {
		t.Fatalf("running %v, %d clicks; want stopped after 6", c.Running(), c.Clicks())
	}
}
