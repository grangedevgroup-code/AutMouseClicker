package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestSpinnerStepsWithArrowsKeysAndWheelWithinItsRange(t *testing.T) {
	test.NewTempApp(t)
	s := newSpinner(0, 59, 1, 58)
	w := test.NewWindow(s)
	defer w.Close()
	w.Resize(s.MinSize().AddWidthHeight(10, 10))
	s.Resize(s.MinSize())

	stepper := s.ActionItem.(*stepper)
	stepper.Resize(fyne.NewSize(stepperWidth, 20))
	test.TapAt(stepper, fyne.NewPos(5, 2))
	if s.Value() != 59 || s.Text != "59" {
		t.Fatalf("after the up arrow: value %d, text %q; want 59", s.Value(), s.Text)
	}
	test.TapAt(stepper, fyne.NewPos(5, 2))
	if s.Value() != 59 {
		t.Fatalf("went past the maximum: %d", s.Value())
	}
	test.TapAt(stepper, fyne.NewPos(5, 18))
	if s.Value() != 58 {
		t.Fatalf("after the down arrow: %d, want 58", s.Value())
	}

	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyPageDown})
	if s.Value() != 47 {
		t.Fatalf("after Down and Page Down: %d, want 47", s.Value())
	}
	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: 1}})
	if s.Value() != 48 {
		t.Fatalf("after scrolling up: %d, want 48", s.Value())
	}
	for range 60 {
		s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: -1}})
	}
	if s.Value() != 0 || s.Text != "0" {
		t.Fatalf("went below the minimum: value %d, text %q", s.Value(), s.Text)
	}
}

func TestSpinnerTypingKeepsDigitsAndClampsToTheMaximum(t *testing.T) {
	test.NewTempApp(t)
	s := newSpinner(0, 999, 10, 0)
	w := test.NewWindow(s)
	defer w.Close()

	s.SetText("")
	test.Type(s, "4a2")
	if s.Value() != 42 || s.Text != "42" {
		t.Fatalf("typed 4a2: value %d, text %q; want 42", s.Value(), s.Text)
	}
	s.SetText("12345")
	if s.Value() != 123 || s.Text != "123" {
		t.Fatalf("typed 12345: value %d, text %q; want 123 (three digits)", s.Value(), s.Text)
	}
	s.SetText("")
	s.FocusLost()
	if s.Text != "123" {
		t.Fatalf("an empty field should show the last value again, got %q", s.Text)
	}
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if s.Value() != 133 {
		t.Fatalf("the step is 10: got %d, want 133", s.Value())
	}
	s.SetValue(5000)
	if s.Value() != 999 {
		t.Fatalf("SetValue above the maximum gave %d", s.Value())
	}
}

func TestSpinnerReportsChanges(t *testing.T) {
	test.NewTempApp(t)
	s := newSpinner(0, 9, 1, 3)
	var seen []int
	s.OnValueChanged = func(v int) { seen = append(seen, v) }
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	s.SetValue(4)
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if len(seen) != 2 || seen[0] != 4 || seen[1] != 3 {
		t.Fatalf("changes %v, want [4 3]", seen)
	}
}
