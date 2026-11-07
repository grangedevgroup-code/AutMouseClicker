//go:build linux

package main

import (
	"errors"
	"os"
)

func waylandSession() bool {
	switch os.Getenv("AMC_BACKEND") {
	case "x11":
		return false
	case "portal":
		return true
	}
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
}

func backendName() string {
	if waylandSession() {
		return "desktop portal"
	}
	return "XTEST"
}

func newInjector() (Injector, error) {
	if waylandSession() {
		return newPortalInjector()
	}
	return newX11Injector()
}

func newHotkeys(onStart, onStop func()) (Hotkeys, error) {
	if os.Getenv("AMC_BACKEND") == "" {
		h, err := newKDEHotkeys(kdeComponent, onStart, onStop)
		if err == nil {
			return h, nil
		}
		if !errors.Is(err, errNoKDE) {
			return nil, err
		}
	}
	if waylandSession() {
		return newPortalHotkeys(onStart, onStop)
	}
	return newX11Hotkeys(onStart, onStop)
}
