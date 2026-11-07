package main

import (
	"fmt"
	"strconv"
	"strings"
)

type Modifier int

const (
	NoModifier Modifier = iota
	Ctrl
	Alt
	Shift
)

var modifierNames = []string{"None", "Ctrl", "Alt", "Shift"}

func (m Modifier) String() string {
	if m < NoModifier || m > Shift {
		return fmt.Sprintf("Modifier(%d)", int(m))
	}
	return modifierNames[m]
}

func ParseModifier(name string) (Modifier, error) {
	for i, n := range modifierNames {
		if equalFold(n, name) {
			return Modifier(i), nil
		}
	}
	return NoModifier, fmt.Errorf("unknown modifier %q: use None, Ctrl, Alt or Shift", name)
}

type Hotkey struct {
	Mod Modifier
	Key int
}

func (h Hotkey) Valid() bool {
	return h.Key >= 1 && h.Key <= 12 && h.Mod >= NoModifier && h.Mod <= Shift
}

func (h Hotkey) KeyName() string { return "F" + strconv.Itoa(h.Key) }

func (h Hotkey) String() string {
	if h.Mod == NoModifier {
		return h.KeyName()
	}
	return h.Mod.String() + "+" + h.KeyName()
}

func ParseHotkey(s string) (Hotkey, error) {
	mod, key := NoModifier, strings.TrimSpace(s)
	if before, after, found := strings.Cut(key, "+"); found {
		m, err := ParseModifier(strings.TrimSpace(before))
		if err != nil {
			return Hotkey{}, err
		}
		mod, key = m, strings.TrimSpace(after)
	}
	n := functionKeyNumber(key)
	if n == 0 {
		return Hotkey{}, fmt.Errorf("unsupported hotkey %q: use F1 to F12, optionally with Ctrl, Alt or Shift", s)
	}
	return Hotkey{Mod: mod, Key: n}, nil
}

func (h Hotkey) qtCode() int32 {
	code := int32(0x01000030 + h.Key - 1)
	switch h.Mod {
	case Shift:
		code |= 0x02000000
	case Ctrl:
		code |= 0x04000000
	case Alt:
		code |= 0x08000000
	}
	return code
}

func (h Hotkey) portalTrigger() string {
	switch h.Mod {
	case Ctrl:
		return "CTRL+" + h.KeyName()
	case Alt:
		return "ALT+" + h.KeyName()
	case Shift:
		return "SHIFT+" + h.KeyName()
	}
	return h.KeyName()
}

func describeHotkeys(start, stop Hotkey) string {
	if start == stop {
		return start.String() + " starts and stops clicking"
	}
	return start.String() + " starts, " + stop.String() + " stops"
}

type Hotkeys interface {
	Bind(start, stop Hotkey) (string, error)
	Close() error
}

func FunctionKeys() []string {
	keys := make([]string, 12)
	for i := range keys {
		keys[i] = fmt.Sprintf("F%d", i+1)
	}
	return keys
}

func functionKeyNumber(key string) int {
	if len(key) < 2 || (key[0] != 'F' && key[0] != 'f') {
		return 0
	}
	n, err := strconv.Atoi(key[1:])
	if err != nil || n < 1 || n > 12 {
		return 0
	}
	return n
}
