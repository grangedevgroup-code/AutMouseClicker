package main

import "testing"

func TestHotkeysRoundTripThroughText(t *testing.T) {
	cases := map[string]Hotkey{
		"F6":       {Key: 6},
		"Ctrl+F2":  {Mod: Ctrl, Key: 2},
		"Alt+F12":  {Mod: Alt, Key: 12},
		"Shift+F1": {Mod: Shift, Key: 1},
	}
	for text, want := range cases {
		got, err := ParseHotkey(text)
		if err != nil || got != want {
			t.Errorf("ParseHotkey(%q) = %v, %v; want %v", text, got, err, want)
		}
		if got.String() != text {
			t.Errorf("%v.String() = %q, want %q", got, got.String(), text)
		}
	}
	for _, bad := range []string{"", "F0", "F13", "Meta+F1", "Ctrl+", "A", "Ctrl+Q"} {
		if h, err := ParseHotkey(bad); err == nil {
			t.Errorf("ParseHotkey(%q) accepted %v", bad, h)
		}
	}
}

func TestHotkeysMapToQtCodesAndPortalTriggers(t *testing.T) {
	cases := []struct {
		key     Hotkey
		qt      int32
		trigger string
	}{
		{Hotkey{Key: 8}, 0x01000037, "F8"},
		{Hotkey{Mod: Ctrl, Key: 2}, 0x05000031, "CTRL+F2"},
		{Hotkey{Mod: Alt, Key: 1}, 0x09000030, "ALT+F1"},
		{Hotkey{Mod: Shift, Key: 12}, 0x0300003b, "SHIFT+F12"},
	}
	for _, c := range cases {
		if got := c.key.qtCode(); got != c.qt {
			t.Errorf("%v.qtCode() = %#x, want %#x", c.key, got, c.qt)
		}
		if got := c.key.portalTrigger(); got != c.trigger {
			t.Errorf("%v.portalTrigger() = %q, want %q", c.key, got, c.trigger)
		}
	}
}

func TestDescribeHotkeysSaysWhenOneKeyToggles(t *testing.T) {
	if got := describeHotkeys(Hotkey{Key: 6}, Hotkey{Key: 7}); got != "F6 starts, F7 stops" {
		t.Errorf("got %q", got)
	}
	if got := describeHotkeys(Hotkey{Key: 6}, Hotkey{Key: 6}); got != "F6 starts and stops clicking" {
		t.Errorf("got %q", got)
	}
}
