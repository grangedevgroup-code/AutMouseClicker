package main

import (
	"fmt"
	"strconv"
	"strings"
)

type Hotkeys interface {
	Bind(start, stop string) (string, error)
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
	if !strings.HasPrefix(key, "F") {
		return 0
	}
	n, err := strconv.Atoi(key[1:])
	if err != nil || n < 1 || n > 12 {
		return 0
	}
	return n
}
