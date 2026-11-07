package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Button int

const (
	Left Button = iota
	Right
	Middle
)

var buttonNames = []string{"Left", "Right", "Middle"}

func (b Button) String() string {
	if b < Left || b > Middle {
		return fmt.Sprintf("Button(%d)", int(b))
	}
	return buttonNames[b]
}

func ParseButton(name string) (Button, error) {
	for i, n := range buttonNames {
		if equalFold(n, name) {
			return Button(i), nil
		}
	}
	return Left, fmt.Errorf("unknown mouse button %q: use left, right or middle", name)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

type Injector interface {
	Press(Button) error
	Release(Button) error
	Close() error
}

type Settings struct {
	Interval time.Duration
	Button   Button
	Double   bool
	Rounds   int
}

const (
	MinInterval = 10 * time.Millisecond
	holdTime    = 5 * time.Millisecond
	doubleGap   = 40 * time.Millisecond
)

func Interval(hours, minutes, seconds, millis int) time.Duration {
	d := time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(millis)*time.Millisecond
	return max(d, MinInterval)
}

type Clicker struct {
	open   func() (Injector, error)
	OnStop func(error)

	mu     sync.Mutex
	inject Injector
	stop   chan struct{}
	done   chan struct{}
	clicks atomic.Int64
}

func NewClicker(open func() (Injector, error)) *Clicker {
	return &Clicker{open: open}
}

var ErrRunning = errors.New("already clicking")

func (c *Clicker) Start(s Settings) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stop != nil {
		return ErrRunning
	}
	if c.inject == nil {
		inject, err := c.open()
		if err != nil {
			return err
		}
		c.inject = inject
	}
	if s.Interval < MinInterval {
		s.Interval = MinInterval
	}
	c.clicks.Store(0)
	stop, done := make(chan struct{}), make(chan struct{})
	c.stop, c.done = stop, done
	go c.loop(c.inject, s, stop, done)
	return nil
}

func (c *Clicker) loop(inject Injector, s Settings, stop, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	finish := func(err error) {
		go func() {
			c.Stop()
			if err != nil {
				c.mu.Lock()
				if c.inject == inject {
					_ = inject.Close()
					c.inject = nil
				}
				c.mu.Unlock()
			}
			if c.OnStop != nil {
				c.OnStop(err)
			}
		}()
	}
	for round := 1; ; round++ {
		if err := c.click(inject, s); err != nil {
			finish(err)
			return
		}
		if s.Rounds > 0 && round >= s.Rounds {
			finish(nil)
			return
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

func (c *Clicker) click(inject Injector, s Settings) error {
	times := 1
	if s.Double {
		times = 2
	}
	for i := range times {
		if i > 0 {
			time.Sleep(doubleGap)
		}
		if err := inject.Press(s.Button); err != nil {
			return err
		}
		time.Sleep(holdTime)
		if err := inject.Release(s.Button); err != nil {
			return err
		}
		c.clicks.Add(1)
	}
	return nil
}

func (c *Clicker) Stop() {
	c.mu.Lock()
	stop, done := c.stop, c.done
	c.stop, c.done = nil, nil
	c.mu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

func (c *Clicker) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stop != nil
}

func (c *Clicker) Clicks() int64 { return c.clicks.Load() }

func (c *Clicker) Close() error {
	c.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inject == nil {
		return nil
	}
	err := c.inject.Close()
	c.inject = nil
	return err
}
