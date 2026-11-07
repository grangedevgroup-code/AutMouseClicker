//go:build !linux

package main

import (
	"fmt"
	"os"
	"runtime"
)

type instance struct{}

func claimInstance() (*instance, bool) { return &instance{}, true }

func (i *instance) OnActivate(func()) {}

func (i *instance) Close() {}

func runInstall() int {
	fmt.Fprintf(os.Stderr, "install is not supported on %s yet\n", runtime.GOOS)
	return 1
}

func runUninstall() int {
	fmt.Fprintf(os.Stderr, "uninstall is not supported on %s yet\n", runtime.GOOS)
	return 1
}
