//go:build linux

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	portalName      = "org.freedesktop.portal.Desktop"
	portalPath      = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	requestIface    = "org.freedesktop.portal.Request"
	remoteIface     = "org.freedesktop.portal.RemoteDesktop"
	shortcutsIface  = "org.freedesktop.portal.GlobalShortcuts"
	sessionIface    = "org.freedesktop.portal.Session"
	pointerDevice   = uint32(2)
	persistAlways   = uint32(2)
	dialogTimeout   = 5 * time.Minute
	callTimeout     = 30 * time.Second
	responseSuccess = uint32(0)
	responseCancel  = uint32(1)
)

var errDenied = errors.New("the desktop did not allow it (the request was cancelled or denied)")

type portal struct {
	conn    *dbus.Conn
	obj     dbus.BusObject
	sender  string
	mu      sync.Mutex
	waiting map[dbus.ObjectPath]chan *dbus.Signal
	nextID  int
	signals map[int]signalHandler
}

type signalHandler struct {
	name  string
	match []dbus.MatchOption
	fn    func(*dbus.Signal)
}

var (
	portalOnce   sync.Once
	sharedPortal *portal
	portalErr    error
)

func getPortal() (*portal, error) {
	portalOnce.Do(func() { sharedPortal, portalErr = openPortal() })
	return sharedPortal, portalErr
}

func openPortal() (*portal, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("no D-Bus session: %w", err)
	}
	names := conn.Names()
	if len(names) == 0 {
		return nil, errors.New("the D-Bus connection has no name")
	}
	p := &portal{
		conn:    conn,
		obj:     conn.Object(portalName, portalPath),
		sender:  strings.ReplaceAll(strings.TrimPrefix(names[0], ":"), ".", "_"),
		waiting: map[dbus.ObjectPath]chan *dbus.Signal{},
		signals: map[int]signalHandler{},
	}
	p.obj.Call("org.freedesktop.host.portal.Registry.Register", 0, appID, map[string]dbus.Variant{})
	ch := make(chan *dbus.Signal, 32)
	conn.Signal(ch)
	go p.dispatch(ch)
	return p, nil
}

func (p *portal) dispatch(ch chan *dbus.Signal) {
	for s := range ch {
		p.mu.Lock()
		waiter := p.waiting[s.Path]
		var handlers []func(*dbus.Signal)
		for _, h := range p.signals {
			if h.name == s.Name {
				handlers = append(handlers, h.fn)
			}
		}
		p.mu.Unlock()
		if s.Name == requestIface+".Response" && waiter != nil {
			waiter <- s
			continue
		}
		for _, h := range handlers {
			h(s)
		}
	}
}

func (p *portal) onSignal(iface, member string, path dbus.ObjectPath, fn func(*dbus.Signal)) (int, error) {
	match := []dbus.MatchOption{dbus.WithMatchInterface(iface), dbus.WithMatchMember(member)}
	if path != "" {
		match = append(match, dbus.WithMatchObjectPath(path))
	}
	if err := p.conn.AddMatchSignal(match...); err != nil {
		return 0, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	p.signals[p.nextID] = signalHandler{name: iface + "." + member, match: match, fn: fn}
	return p.nextID, nil
}

func (p *portal) offSignal(id int) {
	p.mu.Lock()
	h, ok := p.signals[id]
	delete(p.signals, id)
	p.mu.Unlock()
	if ok {
		_ = p.conn.RemoveMatchSignal(h.match...)
	}
}

func token() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "amc" + hex.EncodeToString(b)
}

func (p *portal) request(method string, timeout time.Duration, args func(handle string) []any) (map[string]dbus.Variant, error) {
	handle := token()
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + p.sender + "/" + handle)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(requestIface), dbus.WithMatchMember("Response")}
	waiter := make(chan *dbus.Signal, 1)
	p.mu.Lock()
	p.waiting[path] = waiter
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.waiting, path)
		p.mu.Unlock()
		_ = p.conn.RemoveMatchSignal(match...)
	}()
	if err := p.conn.AddMatchSignal(match...); err != nil {
		return nil, err
	}
	if call := p.obj.Call(method, 0, args(handle)...); call.Err != nil {
		return nil, fmt.Errorf("%s: %w", method, call.Err)
	}
	select {
	case s := <-waiter:
		if len(s.Body) < 2 {
			return nil, fmt.Errorf("%s: malformed response", method)
		}
		code, _ := s.Body[0].(uint32)
		results, _ := s.Body[1].(map[string]dbus.Variant)
		switch code {
		case responseSuccess:
			return results, nil
		case responseCancel:
			return nil, errDenied
		default:
			return nil, fmt.Errorf("%s failed (response %d)", method, code)
		}
	case <-time.After(timeout):
		return nil, fmt.Errorf("%s: no answer from the desktop within %v", method, timeout)
	}
}

func (p *portal) createSession(iface string) (dbus.ObjectPath, error) {
	res, err := p.request(iface+".CreateSession", callTimeout, func(handle string) []any {
		return []any{map[string]dbus.Variant{
			"handle_token":         dbus.MakeVariant(handle),
			"session_handle_token": dbus.MakeVariant(token()),
		}}
	})
	if err != nil {
		return "", err
	}
	switch v := res["session_handle"].Value().(type) {
	case string:
		return dbus.ObjectPath(v), nil
	case dbus.ObjectPath:
		return v, nil
	}
	return "", errors.New("the desktop returned no session")
}

func (p *portal) closeSession(session dbus.ObjectPath) {
	if session != "" {
		p.conn.Object(portalName, session).Call(sessionIface+".Close", 0)
	}
}

func restoreTokenPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "autmouseclicker", "remote-desktop-token")
}

var errRemoteEnded = errors.New("the desktop ended control of the pointer; press Start to allow it again")

type portalInjector struct {
	p       *portal
	session dbus.ObjectPath
	watch   int
	closed  atomic.Bool
}

func newPortalInjector() (Injector, error) {
	p, err := getPortal()
	if err != nil {
		return nil, err
	}
	session, err := p.createSession(remoteIface)
	if err != nil {
		return nil, fmt.Errorf("remote desktop session: %w", err)
	}
	opts := map[string]dbus.Variant{
		"types":        dbus.MakeVariant(pointerDevice),
		"persist_mode": dbus.MakeVariant(persistAlways),
	}
	tokenFile := restoreTokenPath()
	if saved, err := os.ReadFile(tokenFile); err == nil && len(saved) > 0 {
		opts["restore_token"] = dbus.MakeVariant(strings.TrimSpace(string(saved)))
	}
	if _, err := p.request(remoteIface+".SelectDevices", callTimeout, func(handle string) []any {
		opts["handle_token"] = dbus.MakeVariant(handle)
		return []any{session, opts}
	}); err != nil {
		p.closeSession(session)
		return nil, fmt.Errorf("choosing the pointer: %w", err)
	}
	res, err := p.request(remoteIface+".Start", dialogTimeout, func(handle string) []any {
		return []any{session, "", map[string]dbus.Variant{"handle_token": dbus.MakeVariant(handle)}}
	})
	if err != nil {
		p.closeSession(session)
		return nil, fmt.Errorf("permission to click: %w", err)
	}
	if devices, ok := res["devices"].Value().(uint32); ok && devices&pointerDevice == 0 {
		p.closeSession(session)
		return nil, errors.New("the desktop did not grant control of the pointer")
	}
	if v, ok := res["restore_token"].Value().(string); ok && v != "" && tokenFile != "" {
		if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err == nil {
			_ = os.WriteFile(tokenFile, []byte(v), 0o600)
		}
	}
	inj := &portalInjector{p: p, session: session}
	inj.watch, err = p.onSignal(sessionIface, "Closed", session, func(*dbus.Signal) { inj.closed.Store(true) })
	if err != nil {
		p.closeSession(session)
		return nil, fmt.Errorf("watching the remote desktop session: %w", err)
	}
	return inj, nil
}

func evdevButton(b Button) int32 {
	switch b {
	case Right:
		return 0x111
	case Middle:
		return 0x112
	}
	return 0x110
}

func (i *portalInjector) button(b Button, state uint32) error {
	if i.closed.Load() {
		return errRemoteEnded
	}
	return i.p.obj.Call(remoteIface+".NotifyPointerButton", 0, i.session, map[string]dbus.Variant{}, evdevButton(b), state).Err
}

func (i *portalInjector) Press(b Button) error   { return i.button(b, 1) }
func (i *portalInjector) Release(b Button) error { return i.button(b, 0) }

func (i *portalInjector) Close() error {
	i.p.offSignal(i.watch)
	if !i.closed.Load() {
		i.p.closeSession(i.session)
	}
	return nil
}

type portalHotkeys struct {
	p       *portal
	session dbus.ObjectPath
	watch   int
}

type shortcut struct {
	ID      string
	Options map[string]dbus.Variant
}

func newPortalHotkeys(onStart, onStop func()) (Hotkeys, error) {
	p, err := getPortal()
	if err != nil {
		return nil, err
	}
	session, err := p.createSession(shortcutsIface)
	if err != nil {
		return nil, fmt.Errorf("global shortcuts session: %w", err)
	}
	h := &portalHotkeys{p: p, session: session}
	h.watch, err = p.onSignal(shortcutsIface, "Activated", "", func(s *dbus.Signal) {
		if len(s.Body) < 2 {
			return
		}
		if path, _ := s.Body[0].(dbus.ObjectPath); path != session {
			return
		}
		switch id, _ := s.Body[1].(string); id {
		case "start":
			onStart()
		case "stop":
			onStop()
		}
	})
	if err != nil {
		p.closeSession(session)
		return nil, err
	}
	return h, nil
}

func (h *portalHotkeys) Bind(start, stop Hotkey) (string, error) {
	list := []shortcut{{ID: "start", Options: map[string]dbus.Variant{
		"description":       dbus.MakeVariant("Start clicking"),
		"preferred_trigger": dbus.MakeVariant(start.portalTrigger()),
	}}}
	if stop != start {
		list = append(list, shortcut{ID: "stop", Options: map[string]dbus.Variant{
			"description":       dbus.MakeVariant("Stop clicking"),
			"preferred_trigger": dbus.MakeVariant(stop.portalTrigger()),
		}})
	}
	res, err := h.p.request(shortcutsIface+".BindShortcuts", dialogTimeout, func(handle string) []any {
		return []any{h.session, list, "", map[string]dbus.Variant{"handle_token": dbus.MakeVariant(handle)}}
	})
	if err != nil {
		return "", err
	}
	return describeBound(res["shortcuts"].Value(), start == stop)
}

func describeBound(value any, toggle bool) (string, error) {
	triggers := map[string]string{}
	if list, ok := value.([][]any); ok {
		for _, item := range list {
			if len(item) < 2 {
				continue
			}
			id, _ := item[0].(string)
			opts, _ := item[1].(map[string]dbus.Variant)
			if d, ok := opts["trigger_description"].Value().(string); ok && d != "" {
				triggers[id] = d
			}
		}
	}
	if triggers["start"] == "" || (!toggle && triggers["stop"] == "") {
		return "", errors.New("the desktop did not assign the keys; set them in its keyboard shortcut settings")
	}
	if toggle {
		return triggers["start"] + " starts and stops clicking", nil
	}
	return triggers["start"] + " starts, " + triggers["stop"] + " stops", nil
}

func (h *portalHotkeys) Close() error {
	h.p.offSignal(h.watch)
	h.p.closeSession(h.session)
	return nil
}
