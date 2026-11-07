//go:build !linux

package main

import (
	"fmt"
	"runtime"
)

func backendName() string { return runtime.GOOS + " (not supported yet)" }

func newInjector() (Injector, error) {
	return nil, fmt.Errorf("clicking on %s is not supported yet; Linux is supported today", runtime.GOOS)
}

func newHotkeys(onStart, onStop func()) (Hotkeys, error) {
	return nil, fmt.Errorf("global hotkeys on %s are not supported yet", runtime.GOOS)
}

func forgetKDEHotkeys() {}
