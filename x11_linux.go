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

type x11Key struct {
	code xproto.Keycode
	mods uint16
}

type x11Hotkeys struct {
	conn    *xgb.Conn
	root    xproto.Window
	onStart func()
	onStop  func()

	mu       sync.Mutex
	start    x11Key
	stop     x11Key
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

func x11Mods(m Modifier) uint16 {
	switch m {
	case Ctrl:
		return xproto.ModMaskControl
	case Alt:
		return xproto.ModMask1
	case Shift:
		return xproto.ModMaskShift
	}
	return 0
}

func (h *x11Hotkeys) key(k Hotkey) (x11Key, error) {
	if !k.Valid() {
		return x11Key{}, fmt.Errorf("unsupported hotkey %v", k)
	}
	code, ok := h.keycodes[xproto.Keysym(0xffbe+k.Key-1)]
	if !ok {
		return x11Key{}, fmt.Errorf("this keyboard has no %s key", k.KeyName())
	}
	return x11Key{code: code, mods: x11Mods(k.Mod)}, nil
}

func (h *x11Hotkeys) grab(k x11Key, on bool) error {
	for _, extra := range lockMasks {
		mods := k.mods | extra
		if !on {
			xproto.UngrabKey(h.conn, k.code, h.root, mods)
			continue
		}
		if err := xproto.GrabKeyChecked(h.conn, true, h.root, mods, k.code, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil {
			return fmt.Errorf("another program already uses that key: %w", err)
		}
	}
	return nil
}

func (h *x11Hotkeys) Bind(start, stop Hotkey) (string, error) {
	startKey, err := h.key(start)
	if err != nil {
		return "", err
	}
	stopKey, err := h.key(stop)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, old := range []x11Key{h.start, h.stop} {
		if old.code != 0 {
			_ = h.grab(old, false)
		}
	}
	h.start, h.stop = x11Key{}, x11Key{}
	if err := h.grab(startKey, true); err != nil {
		return "", fmt.Errorf("%v: %w", start, err)
	}
	if stopKey != startKey {
		if err := h.grab(stopKey, true); err != nil {
			_ = h.grab(startKey, false)
			return "", fmt.Errorf("%v: %w", stop, err)
		}
	}
	h.start, h.stop = startKey, stopKey
	return describeHotkeys(start, stop), nil
}

func (h *x11Hotkeys) loop() {
	for {
		ev, err := h.conn.WaitForEvent()
		if ev == nil && err == nil {
			return
		}
		press, ok := ev.(xproto.KeyPressEvent)
		if !ok {
			continue
		}
		pressed := x11Key{code: press.Detail, mods: press.State &^ (xproto.ModMaskLock | xproto.ModMask2)}
		h.mu.Lock()
		start, stop := h.start, h.stop
		h.mu.Unlock()
		switch pressed {
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
