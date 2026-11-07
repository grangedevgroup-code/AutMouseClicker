//go:build linux

package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	kgaService        = "org.kde.kglobalaccel"
	kgaPath           = dbus.ObjectPath("/kglobalaccel")
	kgaIface          = "org.kde.KGlobalAccel"
	kgaComponentIface = "org.kde.kglobalaccel.Component"
	kgaSetPresent     = uint32(2)
	kgaNoAutoloading  = uint32(4)
	kdeComponent      = "autmouseclicker"
	kdeFriendly       = "AutMouseClicker"
)

var errNoKDE = errors.New("KDE global shortcuts are not available")

var kdeActions = [2][2]string{{"start", "Start clicking"}, {"stop", "Stop clicking"}}

type kdeShortcutInfo struct {
	Action            string
	ActionFriendly    string
	Component         string
	ComponentFriendly string
	Context           string
	ContextFriendly   string
	Keys              []int32
	DefaultKeys       []int32
}

type kdeHotkeys struct {
	conn      *dbus.Conn
	component string
	onStart   func()
	onStop    func()
	signals   chan *dbus.Signal

	mu   sync.Mutex
	path dbus.ObjectPath
}

func kdeAvailable(conn *dbus.Conn) bool {
	var owned bool
	err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, kgaService).Store(&owned)
	return err == nil && owned
}

func newKDEHotkeys(component string, onStart, onStop func()) (*kdeHotkeys, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, errNoKDE
	}
	if !kdeAvailable(conn) {
		conn.Close()
		return nil, errNoKDE
	}
	h := &kdeHotkeys{conn: conn, component: component, onStart: onStart, onStop: onStop, signals: make(chan *dbus.Signal, 16)}
	conn.Signal(h.signals)
	go h.loop()
	return h, nil
}

func (h *kdeHotkeys) actionID(i int) []string {
	return []string{h.component, kdeActions[i][0], kdeFriendly, kdeActions[i][1]}
}

func (h *kdeHotkeys) call(method string, args ...any) *dbus.Call {
	return h.conn.Object(kgaService, kgaPath).Call(kgaIface+"."+method, 0, args...)
}

func (h *kdeHotkeys) Bind(start, stop Hotkey) (string, error) {
	keys := [2][]int32{{start.qtCode()}, {stop.qtCode()}}
	if start == stop {
		keys[1] = []int32{}
	}
	taken, err := h.bind(keys)
	if err != nil {
		return "", err
	}
	var problems []string
	for i, key := range [2]Hotkey{start, stop} {
		if taken[i] {
			problems = append(problems, fmt.Sprintf("%s is taken by %s.", key, h.holders(key.qtCode())))
		}
	}
	if len(problems) > 0 {
		return "", errors.New(strings.Join(problems, " ") + " Pick another key.")
	}
	return describeHotkeys(start, stop), nil
}

func (h *kdeHotkeys) bind(keys [2][]int32) ([2]bool, error) {
	var taken [2]bool
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range kdeActions {
		if err := h.call("doRegister", h.actionID(i)).Err; err != nil {
			return taken, fmt.Errorf("KDE shortcuts: %w", err)
		}
		if err := h.call("setShortcut", h.actionID(i), []int32{}, kgaSetPresent|kgaNoAutoloading).Err; err != nil {
			return taken, fmt.Errorf("KDE shortcuts: %w", err)
		}
	}
	for i := range kdeActions {
		var got []int32
		if err := h.call("setShortcut", h.actionID(i), keys[i], kgaSetPresent|kgaNoAutoloading).Store(&got); err != nil {
			return taken, fmt.Errorf("KDE shortcuts: %w", err)
		}
		taken[i] = len(keys[i]) > 0 && !slices.Contains(got, keys[i][0])
	}
	if h.path == "" {
		var path dbus.ObjectPath
		if err := h.call("getComponent", h.component).Store(&path); err != nil {
			return taken, fmt.Errorf("KDE shortcuts: %w", err)
		}
		err := h.conn.AddMatchSignal(dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(kgaComponentIface),
			dbus.WithMatchMember("globalShortcutPressed"))
		if err != nil {
			return taken, fmt.Errorf("KDE shortcuts: %w", err)
		}
		h.path = path
	}
	return taken, nil
}

func (h *kdeHotkeys) holders(code int32) string {
	var found []kdeShortcutInfo
	err := h.call("globalShortcutsByKey", struct{ Keys []int32 }{[]int32{code}}, struct{ Match int32 }{0}).Store(&found)
	var names []string
	for _, s := range found {
		if s.Component != h.component {
			names = append(names, s.ComponentFriendly+" ("+s.ActionFriendly+")")
		}
	}
	if err != nil || len(names) == 0 {
		return "another shortcut"
	}
	return strings.Join(names, ", ")
}

func (h *kdeHotkeys) loop() {
	for s := range h.signals {
		if s.Name != kgaComponentIface+".globalShortcutPressed" || len(s.Body) < 2 {
			continue
		}
		h.mu.Lock()
		path := h.path
		h.mu.Unlock()
		if s.Path != path {
			continue
		}
		if component, _ := s.Body[0].(string); component != h.component {
			continue
		}
		switch action, _ := s.Body[1].(string); action {
		case kdeActions[0][0]:
			h.onStart()
		case kdeActions[1][0]:
			h.onStop()
		}
	}
}

func (h *kdeHotkeys) Close() error {
	h.mu.Lock()
	for i := range kdeActions {
		h.call("setInactive", h.actionID(i))
	}
	h.mu.Unlock()
	h.conn.RemoveSignal(h.signals)
	close(h.signals)
	return h.conn.Close()
}

func forgetKDEHotkeys() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer conn.Close()
	if !kdeAvailable(conn) {
		return
	}
	for _, a := range kdeActions {
		conn.Object(kgaService, kgaPath).Call(kgaIface+".unregister", 0, kdeComponent, a[0])
	}
}
