package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed assets/icon.svg
var iconSVG []byte

//go:embed assets/icon-active.svg
var iconActiveSVG []byte

var (
	actions        = []string{"Single click", "Double click"}
	defaultStart   = Hotkey{Key: 6}
	defaultStop    = Hotkey{Key: 7}
	defaultSeconds = 1
)

type window struct {
	app        fyne.App
	win        fyne.Window
	clicker    *Clicker
	icon       fyne.Resource
	activeIcon fyne.Resource

	hours, minutes, seconds, millis *spinner
	button, action                  *widget.Select
	startMod, startKey              *widget.Select
	stopMod, stopKey                *widget.Select
	hotkeyStatus                    *widget.Label
	status                          *widget.Label
	startButton, stopButton         *widget.Button
	trayMenu                        *fyne.Menu
	trayStart, trayStop             *fyne.MenuItem

	running  Settings
	starting bool
	rebind   chan [2]Hotkey

	hotkeysMu sync.Mutex
	hotkeys   Hotkeys
	cleanOnce sync.Once
}

func runGUI(inst *instance) {
	a := app.NewWithID(appID)
	a.Settings().SetTheme(newCompactTheme())
	icon := fyne.NewStaticResource("autmouseclicker.svg", iconSVG)
	a.SetIcon(icon)
	w := &window{
		app:        a,
		win:        a.NewWindow("AutMouseClicker"),
		clicker:    NewClicker(newInjector),
		icon:       icon,
		activeIcon: fyne.NewStaticResource("autmouseclicker-active.svg", iconActiveSVG),
		rebind:     make(chan [2]Hotkey, 1),
	}
	w.clicker.OnStop = func(err error) { fyne.Do(func() { w.stopped(err) }) }
	w.build()
	w.setupTray()
	w.win.SetCloseIntercept(w.exit)
	w.win.SetFixedSize(true)
	a.Lifecycle().SetOnStopped(w.cleanup)
	inst.OnActivate(func() { fyne.Do(w.show) })
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		fyne.Do(a.Quit)
	}()
	go w.hotkeyWorker()
	w.requestBind()
	go w.refresh()
	w.win.ShowAndRun()
}

func unitLabel(text string) *widget.Label {
	return widget.NewLabel(text)
}

func fieldLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Alignment = fyne.TextAlignTrailing
	return l
}

func hotkeySelects(selected Hotkey, changed func()) (*widget.Select, *widget.Select) {
	mod := widget.NewSelect(modifierNames, nil)
	mod.SetSelected(selected.Mod.String())
	key := widget.NewSelect(FunctionKeys(), nil)
	key.SetSelected(selected.KeyName())
	mod.OnChanged = func(string) { changed() }
	key.OnChanged = func(string) { changed() }
	return mod, key
}

func hotkeyFrom(mod, key *widget.Select) Hotkey {
	m, _ := ParseModifier(mod.Selected)
	return Hotkey{Mod: m, Key: functionKeyNumber(key.Selected)}
}

func savedHotkey(prefs fyne.Preferences, name string, fallback Hotkey) Hotkey {
	if h, err := ParseHotkey(prefs.String(name)); err == nil {
		return h
	}
	return fallback
}

func (w *window) build() {
	prefs := w.app.Preferences()
	millis := prefs.IntWithFallback("millis", -1)
	if millis < 0 {
		millis = prefs.IntWithFallback("tenths", 0)*100 + prefs.IntWithFallback("hundredths", 0)*10
	}
	w.hours = newSpinner(0, 99, 1, prefs.IntWithFallback("hours", 0))
	w.minutes = newSpinner(0, 59, 1, prefs.IntWithFallback("minutes", 0))
	w.seconds = newSpinner(0, 59, 1, prefs.IntWithFallback("seconds", defaultSeconds))
	w.millis = newSpinner(0, 999, 10, millis)
	interval := container.NewHBox(
		w.hours, unitLabel("h"), w.minutes, unitLabel("min"), w.seconds, unitLabel("s"), w.millis, unitLabel("ms"))

	w.button = widget.NewSelect(buttonNames, nil)
	w.button.SetSelected(prefs.StringWithFallback("button", buttonNames[0]))
	w.action = widget.NewSelect(actions, nil)
	w.action.SetSelected(prefs.StringWithFallback("action", actions[0]))

	w.startMod, w.startKey = hotkeySelects(savedHotkey(prefs, "startHotkey", defaultStart), w.keysChanged)
	w.stopMod, w.stopKey = hotkeySelects(savedHotkey(prefs, "stopHotkey", defaultStop), w.keysChanged)
	w.hotkeyStatus = widget.NewLabel("Setting up hotkeys…")
	w.hotkeyStatus.SizeName = theme.SizeNameCaptionText
	w.hotkeyStatus.Wrapping = fyne.TextWrapWord

	form := container.New(layout.NewFormLayout(),
		fieldLabel("Click every"), interval,
		fieldLabel("Mouse"), container.NewHBox(w.button, w.action),
		fieldLabel("Start key"), container.NewHBox(w.startMod, w.startKey),
		fieldLabel("Stop key"), container.NewHBox(w.stopMod, w.stopKey),
	)

	w.status = widget.NewLabel("Stopped. Clicks go where the pointer is.")
	w.status.Truncation = fyne.TextTruncateEllipsis
	w.startButton = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), w.start)
	w.startButton.Importance = widget.HighImportance
	w.stopButton = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), w.stop)
	w.stopButton.Disable()
	footer := container.NewBorder(nil, nil, nil, container.NewHBox(w.startButton, w.stopButton), w.status)

	w.win.SetContent(container.NewPadded(container.NewVBox(form, w.hotkeyStatus, widget.NewSeparator(), footer)))
}

func (w *window) setupTray() {
	desk, ok := w.app.(desktop.App)
	if !ok {
		return
	}
	w.trayStart = fyne.NewMenuItem("Start clicking", w.start)
	w.trayStop = fyne.NewMenuItem("Stop clicking", w.stop)
	w.trayMenu = fyne.NewMenu("AutMouseClicker", w.trayStart, w.trayStop, fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Show window", w.show))
	desk.SetSystemTrayMenu(w.trayMenu)
	desk.SetSystemTrayWindow(w.win)
	w.updateTray(false)
}

func (w *window) updateTray(active bool) {
	desk, ok := w.app.(desktop.App)
	if !ok || w.trayMenu == nil {
		return
	}
	w.trayStart.Disabled = active || w.starting
	w.trayStop.Disabled = !active
	w.trayMenu.Refresh()
	if active {
		desk.SetSystemTrayIcon(w.activeIcon)
	} else {
		desk.SetSystemTrayIcon(w.icon)
	}
}

func (w *window) show() {
	w.win.Show()
	w.win.RequestFocus()
}

func (w *window) settings() (Settings, error) {
	button, err := ParseButton(w.button.Selected)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Interval: Interval(w.hours.Value(), w.minutes.Value(), w.seconds.Value(), w.millis.Value()),
		Button:   button,
		Double:   w.action.Selected == actions[1],
	}, nil
}

func (w *window) save() {
	prefs := w.app.Preferences()
	prefs.SetInt("hours", w.hours.Value())
	prefs.SetInt("minutes", w.minutes.Value())
	prefs.SetInt("seconds", w.seconds.Value())
	prefs.SetInt("millis", w.millis.Value())
	prefs.SetString("button", w.button.Selected)
	prefs.SetString("action", w.action.Selected)
	prefs.SetString("startHotkey", hotkeyFrom(w.startMod, w.startKey).String())
	prefs.SetString("stopHotkey", hotkeyFrom(w.stopMod, w.stopKey).String())
	for _, old := range []string{"tenths", "hundredths", "startKey", "stopKey"} {
		prefs.RemoveValue(old)
	}
}

func describe(s Settings, clicks int64) string {
	kind := "Clicking"
	if s.Double {
		kind = "Double-clicking"
	}
	return fmt.Sprintf("%s %s every %v: %d clicks", kind, strings.ToLower(s.Button.String()), s.Interval, clicks)
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
	w.updateTray(false)
	w.status.SetText("Starting… the desktop may ask to let AutMouseClicker control the pointer.")
	go func() {
		err := w.clicker.Start(s)
		fyne.Do(func() {
			w.starting = false
			if err != nil {
				w.startButton.Enable()
				w.updateTray(false)
				w.status.SetText("Could not start: " + err.Error())
				return
			}
			w.running = s
			w.stopButton.Enable()
			w.updateTray(true)
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

func (w *window) toggle() {
	if w.clicker.Running() {
		w.stop()
		return
	}
	w.start()
}

func (w *window) hotkeyStart() {
	if hotkeyFrom(w.startMod, w.startKey) == hotkeyFrom(w.stopMod, w.stopKey) {
		w.toggle()
		return
	}
	w.start()
}

func (w *window) stopped(err error) {
	w.stopButton.Disable()
	if !w.starting {
		w.startButton.Enable()
	}
	w.updateTray(false)
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
	w.save()
	w.requestBind()
}

func (w *window) requestBind() {
	keys := [2]Hotkey{hotkeyFrom(w.startMod, w.startKey), hotkeyFrom(w.stopMod, w.stopKey)}
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
			created, err := newHotkeys(func() { fyne.Do(w.hotkeyStart) }, func() { fyne.Do(w.stop) })
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
				w.hotkeyStatus.SetText("Hotkeys not set: " + err.Error())
				return
			}
			w.hotkeyStatus.SetText("Hotkeys work in any window: " + bound + ".")
		})
	}
}

func (w *window) exit() {
	w.win.Hide()
	w.app.Quit()
}

func (w *window) cleanup() {
	w.cleanOnce.Do(func() {
		_ = w.clicker.Close()
		w.hotkeysMu.Lock()
		h := w.hotkeys
		w.hotkeys = nil
		w.hotkeysMu.Unlock()
		if h != nil {
			_ = h.Close()
		}
	})
}
