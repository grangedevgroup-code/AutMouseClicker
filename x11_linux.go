//go:build linux

package main

import (
	"fmt"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

type x11Injector struct {
	conn *xgb.Conn
	root xproto.Window
}

func newX11Injector() (Injector, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("cannot reach the X server: %w", err)
	}
	if err := xtest.Init(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("the X server has no XTEST extension: %w", err)
	}
	return &x11Injector{conn: conn, root: xproto.Setup(conn).DefaultScreen(conn).Root}, nil
}

func x11Button(b Button) byte {
	switch b {
	case Right:
		return 3
	case Middle:
		return 2
	}
	return 1
}

func (x *x11Injector) fake(kind byte, b Button) error {
	return xtest.FakeInputChecked(x.conn, kind, x11Button(b), 0, x.root, 0, 0, 0).Check()
}

func (x *x11Injector) Press(b Button) error   { return x.fake(xproto.ButtonPress, b) }
func (x *x11Injector) Release(b Button) error { return x.fake(xproto.ButtonRelease, b) }

func (x *x11Injector) Close() error {
	x.conn.Close()
	return nil
}

type x11Hotkeys struct {
	conn    *xgb.Conn
	root    xproto.Window
	onStart func()
	onStop  func()

	mu       sync.Mutex
	start    xproto.Keycode
	stop     xproto.Keycode
	keycodes map[xproto.Keysym]xproto.Keycode
}

var lockMasks = []uint16{0, xproto.ModMaskLock, xproto.ModMask2, xproto.ModMaskLock | xproto.ModMask2}

func newX11Hotkeys(onStart, onStop func()) (Hotkeys, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("cannot reach the X server: %w", err)
	}
	setup := xproto.Setup(conn)
	h := &x11Hotkeys{conn: conn, root: setup.DefaultScreen(conn).Root, onStart: onStart, onStop: onStop, keycodes: map[xproto.Keysym]xproto.Keycode{}}
	count := byte(setup.MaxKeycode - setup.MinKeycode + 1)
	mapping, err := xproto.GetKeyboardMapping(conn, setup.MinKeycode, count).Reply()
	if err != nil {
		conn.Close()
		return nil, err
	}
	per := int(mapping.KeysymsPerKeycode)
	for i := 0; i < int(count) && per > 0; i++ {
		for j := range per {
			sym := mapping.Keysyms[i*per+j]
			if _, seen := h.keycodes[sym]; !seen && sym != 0 {
				h.keycodes[sym] = xproto.Keycode(int(setup.MinKeycode) + i)
			}
		}
	}
	go h.loop()
	return h, nil
}

func (h *x11Hotkeys) keycode(key string) (xproto.Keycode, error) {
	n := functionKeyNumber(key)
	if n == 0 {
		return 0, fmt.Errorf("unsupported hotkey %q", key)
	}
	code, ok := h.keycodes[xproto.Keysym(0xffbe+n-1)]
	if !ok {
		return 0, fmt.Errorf("this keyboard has no %s key", key)
	}
	return code, nil
}

func (h *x11Hotkeys) grab(code xproto.Keycode, on bool) error {
	for _, extra := range lockMasks {
		mods := xproto.ModMaskControl | extra
		if !on {
			xproto.UngrabKey(h.conn, code, h.root, mods)
			continue
		}
		if err := xproto.GrabKeyChecked(h.conn, true, h.root, mods, code, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil {
			return fmt.Errorf("another program already uses that key: %w", err)
		}
	}
	return nil
}

func (h *x11Hotkeys) Bind(start, stop string) (string, error) {
	startCode, err := h.keycode(start)
	if err != nil {
		return "", err
	}
	stopCode, err := h.keycode(stop)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, old := range []xproto.Keycode{h.start, h.stop} {
		if old != 0 {
			_ = h.grab(old, false)
		}
	}
	h.start, h.stop = 0, 0
	if err := h.grab(startCode, true); err != nil {
		return "", err
	}
	if stopCode != startCode {
		if err := h.grab(stopCode, true); err != nil {
			_ = h.grab(startCode, false)
			return "", err
		}
	}
	h.start, h.stop = startCode, stopCode
	return "start Ctrl+" + start + ", stop Ctrl+" + stop, nil
}

func (h *x11Hotkeys) loop() {
	for {
		ev, err := h.conn.WaitForEvent()
		if ev == nil && err == nil {
			return
		}
		press, ok := ev.(xproto.KeyPressEvent)
		if !ok || press.State&^(xproto.ModMaskLock|xproto.ModMask2) != xproto.ModMaskControl {
			continue
		}
		h.mu.Lock()
		start, stop := h.start, h.stop
		h.mu.Unlock()
		switch press.Detail {
		case start:
			h.onStart()
		case stop:
			h.onStop()
		}
	}
}

func (h *x11Hotkeys) Close() error {
	h.conn.Close()
	return nil
}
