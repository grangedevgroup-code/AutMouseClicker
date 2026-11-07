//go:build linux

package main

import (
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestKDEHotkeysDeliverPressesAndReleaseTheirKeys(t *testing.T) {
	if os.Getenv("AMC_KDE_TEST") != "1" {
		t.Skip("set AMC_KDE_TEST=1 in a KDE Plasma session to talk to the real kglobalaccel")
	}
	const component = "autmouseclicker-test"
	pressed := make(chan string, 4)
	h, err := newKDEHotkeys(component, func() { pressed <- "start" }, func() { pressed <- "stop" })
	if err != nil {
		t.Fatal(err)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer func() {
		for _, a := range kdeActions {
			conn.Object(kgaService, kgaPath).Call(kgaIface+".unregister", 0, component, a[0])
		}
	}()
	if taken, err := h.bind([2][]int32{{}, {}}); err != nil || taken != [2]bool{} {
		t.Fatalf("bind: %v %v", taken, err)
	}
	componentObj := conn.Object(kgaService, h.path)
	for _, action := range []string{"start", "stop"} {
		if call := componentObj.Call(kgaComponentIface+".invokeShortcut", 0, action); call.Err != nil {
			t.Fatal(call.Err)
		}
		select {
		case got := <-pressed:
			if got != action {
				t.Fatalf("invoking %s called %s", action, got)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("pressing %s never arrived", action)
		}
	}
	var active bool
	if err := componentObj.Call(kgaComponentIface+".isActive", 0).Store(&active); err != nil || !active {
		t.Fatalf("component active %v, %v; want active while bound", active, err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if err := componentObj.Call(kgaComponentIface+".isActive", 0).Store(&active); err != nil || active {
		t.Fatalf("component active %v, %v after Close; want inactive", active, err)
	}
}
