package main

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed assets/icon.svg
var iconSVG []byte

var (
	fieldKeys     = [5]string{"hours", "minutes", "seconds", "tenths", "hundredths"}
	fieldLabels   = [5]string{"Hours", "Minutes", "Seconds", "1/10 s", "1/100 s"}
	fieldDefaults = [5]int{0, 0, 1, 0, 0}
	actions       = []string{"Single click", "Double click"}
)

type window struct {
	app          fyne.App
	win          fyne.Window
	clicker      *Clicker
	fields       [5]*widget.Entry
	button       *widget.Select
	action       *widget.Select
	startKey     *widget.Select
	stopKey      *widget.Select
	status       *widget.Label
	hotkeyStatus *widget.Label
	startButton  *widget.Button
	stopButton   *widget.Button
	running      Settings
	starting     bool
	rebind       chan [2]string
	hotkeysMu    sync.Mutex
	hotkeys      Hotkeys
}

func runGUI() {
	a := app.NewWithID(appID)
	a.SetIcon(fyne.NewStaticResource("autmouseclicker.svg", iconSVG))
	w := &window{
		app:     a,
		win:     a.NewWindow("AutMouseClicker"),
		clicker: NewClicker(newInjector),
		rebind:  make(chan [2]string, 1),
	}
	w.clicker.OnStop = func(err error) { fyne.Do(func() { w.stopped(err) }) }
	w.build()
	w.win.SetCloseIntercept(w.exit)
	go w.hotkeyWorker()
	w.requestBind()
	go w.refresh()
	w.win.ShowAndRun()
}

func digitsOnly(e *widget.Entry) func(string) {
	return func(text string) {
		clean := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, text)
		if len(clean) > 3 {
			clean = clean[:3]
		}
		if clean != text {
			e.SetText(clean)
		}
	}
}

func (w *window) build() {
	prefs := w.app.Preferences()
	columns := make([]fyne.CanvasObject, len(w.fields))
	for i := range w.fields {
		e := widget.NewEntry()
		e.SetText(strconv.Itoa(prefs.IntWithFallback(fieldKeys[i], fieldDefaults[i])))
		e.OnChanged = digitsOnly(e)
		w.fields[i] = e
		columns[i] = container.NewVBox(widget.NewLabel(fieldLabels[i]), e)
	}
	interval := widget.NewCard("Click interval", "", container.NewGridWithColumns(len(columns), columns...))

	where := widget.NewRadioGroup([]string{"Where the mouse pointer is"}, nil)
	where.SetSelected("Where the mouse pointer is")
	where.Required = true
	point := widget.NewCard("Click point", "", where)

	w.button = widget.NewSelect(buttonNames, nil)
	w.button.SetSelected(prefs.StringWithFallback("button", buttonNames[0]))
	w.action = widget.NewSelect(actions, nil)
	w.action.SetSelected(prefs.StringWithFallback("action", actions[0]))
	options := widget.NewCard("Click options", "", container.New(layout.NewFormLayout(),
		widget.NewLabel("Mouse button"), w.button,
		widget.NewLabel("Mouse action"), w.action))

	w.startKey = widget.NewSelect(FunctionKeys(), nil)
	w.startKey.SetSelected(prefs.StringWithFallback("startKey", "F2"))
	w.stopKey = widget.NewSelect(FunctionKeys(), nil)
	w.stopKey.SetSelected(prefs.StringWithFallback("stopKey", "F3"))
	w.startKey.OnChanged = func(string) { w.keysChanged() }
	w.stopKey.OnChanged = func(string) { w.keysChanged() }
	w.hotkeyStatus = widget.NewLabel("Setting up hotkeys…")
	w.hotkeyStatus.Wrapping = fyne.TextWrapWord
	hotkeys := widget.NewCard("Hotkeys", "", container.NewVBox(
		container.New(layout.NewFormLayout(),
			widget.NewLabel("Start"), container.NewHBox(widget.NewLabel("Ctrl +"), w.startKey),
			widget.NewLabel("Stop"), container.NewHBox(widget.NewLabel("Ctrl +"), w.stopKey)),
		w.hotkeyStatus))

	w.status = widget.NewLabel("Stopped. Input: " + backendName())
	w.status.Wrapping = fyne.TextWrapWord
	w.startButton = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), w.start)
	w.startButton.Importance = widget.HighImportance
	w.stopButton = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), w.stop)
	w.stopButton.Disable()
	exit := widget.NewButtonWithIcon("Exit", theme.LogoutIcon(), w.exit)

	w.win.SetContent(container.NewPadded(container.NewVBox(
		container.NewBorder(nil, nil, point, nil, interval),
		container.NewGridWithColumns(2, hotkeys, options),
		container.NewBorder(nil, nil, nil, container.NewHBox(w.startButton, w.stopButton, exit), w.status),
	)))
}

func (w *window) settings() (Settings, error) {
	var v [5]int
	for i, e := range w.fields {
		text := strings.TrimSpace(e.Text)
		if text == "" {
			continue
		}
		n, err := strconv.Atoi(text)
		if err != nil || n < 0 || n > 999 {
			return Settings{}, fmt.Errorf("%s must be a whole number from 0 to 999", fieldLabels[i])
		}
		v[i] = n
	}
	button, err := ParseButton(w.button.Selected)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Interval: Interval(v[0], v[1], v[2], v[3], v[4]),
		Button:   button,
		Double:   w.action.Selected == actions[1],
	}, nil
}

func (w *window) save() {
	prefs := w.app.Preferences()
	for i, e := range w.fields {
		n, _ := strconv.Atoi(strings.TrimSpace(e.Text))
		prefs.SetInt(fieldKeys[i], n)
	}
	prefs.SetString("button", w.button.Selected)
	prefs.SetString("action", w.action.Selected)
	prefs.SetString("startKey", w.startKey.Selected)
	prefs.SetString("stopKey", w.stopKey.Selected)
}

func describe(s Settings, clicks int64) string {
	kind := "Clicking"
	if s.Double {
		kind = "Double-clicking"
	}
	return fmt.Sprintf("%s the %s button every %v: %d clicks", kind, strings.ToLower(s.Button.String()), s.Interval, clicks)
}

func (w *window) start() {
	if w.starting || w.clicker.Running() {
		return
	}
	s, err := w.settings()
	if err != nil {
		w.status.SetText(err.Error())
		return
	}
	w.save()
	w.starting = true
	w.startButton.Disable()
	w.status.SetText("Starting… the desktop may ask you to allow AutMouseClicker to control the pointer.")
	go func() {
		err := w.clicker.Start(s)
		fyne.Do(func() {
			w.starting = false
			if err != nil {
				w.startButton.Enable()
				w.status.SetText("Could not start: " + err.Error())
				return
			}
			w.running = s
			w.stopButton.Enable()
			w.status.SetText(describe(s, w.clicker.Clicks()))
		})
	}()
}

func (w *window) stop() {
	if !w.clicker.Running() {
		return
	}
	w.clicker.Stop()
	w.stopped(nil)
}

func (w *window) stopped(err error) {
	w.stopButton.Disable()
	if !w.starting {
		w.startButton.Enable()
	}
	if err != nil {
		w.status.SetText("Stopped: " + err.Error())
		return
	}
	w.status.SetText(fmt.Sprintf("Stopped after %d clicks.", w.clicker.Clicks()))
}

func (w *window) refresh() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if !w.clicker.Running() {
			continue
		}
		clicks := w.clicker.Clicks()
		fyne.Do(func() {
			if w.clicker.Running() {
				w.status.SetText(describe(w.running, clicks))
			}
		})
	}
}

func (w *window) keysChanged() {
	if w.startKey.Selected == w.stopKey.Selected {
		w.hotkeyStatus.SetText("Choose two different keys for start and stop.")
		return
	}
	w.save()
	w.requestBind()
}

func (w *window) requestBind() {
	keys := [2]string{w.startKey.Selected, w.stopKey.Selected}
	select {
	case <-w.rebind:
	default:
	}
	w.rebind <- keys
}

func (w *window) hotkeyWorker() {
	for keys := range w.rebind {
		w.hotkeysMu.Lock()
		h := w.hotkeys
		w.hotkeysMu.Unlock()
		if h == nil {
			created, err := newHotkeys(func() { fyne.Do(w.start) }, func() { fyne.Do(w.stop) })
			if err != nil {
				fyne.Do(func() { w.hotkeyStatus.SetText("Hotkeys unavailable: " + err.Error()) })
				continue
			}
			h = created
			w.hotkeysMu.Lock()
			w.hotkeys = h
			w.hotkeysMu.Unlock()
		}
		bound, err := h.Bind(keys[0], keys[1])
		fyne.Do(func() {
			if err != nil {
				w.hotkeyStatus.SetText("Hotkeys unavailable: " + err.Error())
				return
			}
			w.hotkeyStatus.SetText("Hotkeys: " + bound)
		})
	}
}

func (w *window) exit() {
	w.win.Hide()
	go func() {
		_ = w.clicker.Close()
		w.hotkeysMu.Lock()
		h := w.hotkeys
		w.hotkeysMu.Unlock()
		if h != nil {
			_ = h.Close()
		}
		fyne.Do(w.app.Quit)
	}()
}
