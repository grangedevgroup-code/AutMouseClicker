package main

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const stepperWidth = 14

type spinner struct {
	widget.Entry
	min, max, step int
	value          int
	OnValueChanged func(int)
}

func newSpinner(min, max, step, value int) *spinner {
	s := &spinner{min: min, max: max, step: step}
	s.Wrapping = fyne.TextWrapOff
	s.Scroll = fyne.ScrollNone
	s.ActionItem = newStepper(func() { s.add(s.step) }, func() { s.add(-s.step) })
	s.ExtendBaseWidget(s)
	s.Entry.OnChanged = s.typed
	s.SetValue(value)
	return s
}

func (s *spinner) Value() int { return s.value }

func (s *spinner) SetValue(v int) {
	v = min(max(v, s.min), s.max)
	changed := v != s.value
	s.value = v
	if text := strconv.Itoa(v); s.Text != text {
		s.SetText(text)
	}
	if changed && s.OnValueChanged != nil {
		s.OnValueChanged(v)
	}
}

func (s *spinner) add(delta int) { s.SetValue(s.value + delta) }

func (s *spinner) typed(text string) {
	clean := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, text)
	if limit := len(strconv.Itoa(s.max)); len(clean) > limit {
		clean = clean[:limit]
	}
	if clean != text {
		s.SetText(clean)
		return
	}
	if clean == "" {
		return
	}
	n, _ := strconv.Atoi(clean)
	s.SetValue(n)
}

func (s *spinner) TypedRune(r rune) {
	if r < '0' || r > '9' {
		return
	}
	if len(s.Text) >= len(strconv.Itoa(s.max)) && s.SelectedText() == "" {
		return
	}
	s.Entry.TypedRune(r)
}

func (s *spinner) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyUp:
		s.add(s.step)
	case fyne.KeyDown:
		s.add(-s.step)
	case fyne.KeyPageUp:
		s.add(10 * s.step)
	case fyne.KeyPageDown:
		s.add(-10 * s.step)
	default:
		s.Entry.TypedKey(ev)
	}
}

func (s *spinner) Scrolled(ev *fyne.ScrollEvent) {
	switch {
	case ev.Scrolled.DY > 0:
		s.add(s.step)
	case ev.Scrolled.DY < 0:
		s.add(-s.step)
	}
}

func (s *spinner) FocusLost() {
	s.Entry.FocusLost()
	if s.Text == "" {
		s.SetText(strconv.Itoa(s.value))
	}
}

func (s *spinner) MinSize() fyne.Size {
	digits := fyne.MeasureText(strings.Repeat("0", len(strconv.Itoa(s.max))), theme.TextSize(), fyne.TextStyle{})
	width := digits.Width + 2*theme.InnerPadding() + 2*theme.InputBorderSize() + stepperWidth + 2*theme.Padding()
	return fyne.NewSize(width, s.Entry.MinSize().Height)
}

type stepper struct {
	widget.BaseWidget
	up, down func()
}

func newStepper(up, down func()) *stepper {
	s := &stepper{up: up, down: down}
	s.ExtendBaseWidget(s)
	return s
}

func (s *stepper) Tapped(ev *fyne.PointEvent) {
	if ev.Position.Y < s.Size().Height/2 {
		s.up()
	} else {
		s.down()
	}
}

func (s *stepper) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (s *stepper) MinSize() fyne.Size { return fyne.NewSize(stepperWidth, 2*stepperWidth) }

func (s *stepper) CreateRenderer() fyne.WidgetRenderer {
	up := canvas.NewImageFromResource(theme.MenuDropUpIcon())
	up.FillMode = canvas.ImageFillContain
	down := canvas.NewImageFromResource(theme.MenuDropDownIcon())
	down.FillMode = canvas.ImageFillContain
	return &stepperRenderer{up: up, down: down}
}

type stepperRenderer struct {
	up, down *canvas.Image
}

func (r *stepperRenderer) Layout(size fyne.Size) {
	half := fyne.NewSize(size.Width, size.Height/2)
	r.up.Resize(half)
	r.up.Move(fyne.NewPos(0, 0))
	r.down.Resize(half)
	r.down.Move(fyne.NewPos(0, size.Height/2))
}

func (r *stepperRenderer) MinSize() fyne.Size { return fyne.NewSize(stepperWidth, 2*stepperWidth) }

func (r *stepperRenderer) Refresh() {
	r.up.Resource = theme.MenuDropUpIcon()
	r.down.Resource = theme.MenuDropDownIcon()
	r.up.Refresh()
	r.down.Refresh()
}

func (r *stepperRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.up, r.down} }

func (r *stepperRenderer) Destroy() {}
