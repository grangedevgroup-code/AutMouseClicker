package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const appID = "io.github.grangedevgroup_code.AutMouseClicker"

var version = "dev"

const usageText = `AutMouseClicker %s: clicks the mouse for you, where the pointer is.

Usage:
  autmouseclicker                 open the window
  autmouseclicker click [flags]   click from the command line until Ctrl+C
  autmouseclicker install         add the app to the menu and the desktop
  autmouseclicker uninstall       remove it from the menu and the desktop
  autmouseclicker version         print the version

Flags for click:
  --button left|right|middle      which button (default left)
  --double                        double-click each time
  --interval 1s                   time between clicks, at least 10ms (default 1s)
  --count N                       stop after N rounds (default 0: until stopped)
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("AutMouseClicker", version)
			return
		case "help", "--help", "-h":
			fmt.Printf(usageText, version)
			return
		case "click":
			os.Exit(runClick(os.Args[2:]))
		case "install":
			os.Exit(runInstall())
		case "uninstall":
			os.Exit(runUninstall())
		default:
			fmt.Fprintf(os.Stderr, usageText, version)
			os.Exit(2)
		}
	}
	inst, primary := claimInstance()
	if !primary {
		return
	}
	defer inst.Close()
	runGUI(inst)
}

func runClick(args []string) int {
	flags := flag.NewFlagSet("click", flag.ContinueOnError)
	buttonName := flags.String("button", "left", "left, right or middle")
	double := flags.Bool("double", false, "double-click each time")
	interval := flags.Duration("interval", time.Second, "time between clicks")
	count := flags.Int("count", 0, "stop after this many rounds")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	button, err := ParseButton(*buttonName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *count < 0 {
		fmt.Fprintln(os.Stderr, "--count cannot be negative")
		return 2
	}
	clicker := NewClicker(newInjector)
	defer clicker.Close()
	finished := make(chan error, 1)
	clicker.OnStop = func(err error) { finished <- err }
	if err := clicker.Start(Settings{Interval: *interval, Button: button, Double: *double, Rounds: *count}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
		clicker.Stop()
	case err := <-finished:
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Printf("%d clicks\n", clicker.Clicks())
	return 0
}
