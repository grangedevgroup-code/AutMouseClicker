//go:build linux

package main

import "os"

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
		return "Wayland (desktop portal)"
	}
	return "X11 (XTEST)"
}

func newInjector() (Injector, error) {
	if waylandSession() {
		return newPortalInjector()
	}
	return newX11Injector()
}

func newHotkeys(onStart, onStop func()) (Hotkeys, error) {
	if waylandSession() {
		return newPortalHotkeys(onStart, onStop)
	}
	return newX11Hotkeys(onStart, onStop)
}
