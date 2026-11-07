//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type installPaths struct {
	bin, icon, entry, shortcut string
}

func dataHome(home string) string {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(home, ".local", "share")
}

func desktopDir(home string) string {
	if out, err := exec.Command("xdg-user-dir", "DESKTOP").Output(); err == nil {
		if dir := strings.TrimSpace(string(out)); filepath.IsAbs(dir) && dir != home {
			return dir
		}
	}
	return filepath.Join(home, "Desktop")
}

func pathsFor(home string) installPaths {
	data := dataHome(home)
	return installPaths{
		bin:      filepath.Join(home, ".local", "bin", "autmouseclicker"),
		icon:     filepath.Join(data, "icons", "hicolor", "scalable", "apps", appID+".svg"),
		entry:    filepath.Join(data, "applications", appID+".desktop"),
		shortcut: filepath.Join(desktopDir(home), appID+".desktop"),
	}
}

func desktopExec(path string) string {
	path = strings.ReplaceAll(path, "%", "%%")
	if !strings.ContainsAny(path, " \t\n\"'\\><~|&;$*?#()`") {
		return path
	}
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`)
	return `"` + r.Replace(path) + `"`
}

func desktopEntry(bin string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=AutMouseClicker",
		"GenericName=Auto clicker",
		"Comment=Clicks the mouse for you at a set interval",
		"Exec=" + desktopExec(bin),
		"Icon=" + appID,
		"Terminal=false",
		"Categories=Utility;",
		"Keywords=auto;clicker;mouse;click;",
		"StartupWMClass=" + appID,
		"SingleMainWindow=true",
	}, "\n") + "\n"
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	staged := path + ".new"
	if err := os.WriteFile(staged, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(staged, mode); err != nil {
		os.Remove(staged)
		return err
	}
	return os.Rename(staged, path)
}

func refreshMenus(apps string) {
	for _, cmd := range [][]string{{"update-desktop-database", apps}, {"kbuildsycoca6"}, {"kbuildsycoca5"}} {
		if _, err := exec.LookPath(cmd[0]); err == nil {
			_ = exec.Command(cmd[0], cmd[1:]...).Run()
		}
	}
}

func install(home, exe string) (installPaths, error) {
	p := pathsFor(home)
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if current, _ := filepath.EvalSymlinks(p.bin); current != exe {
		data, err := os.ReadFile(exe)
		if err != nil {
			return p, fmt.Errorf("reading %s: %w", exe, err)
		}
		if err := writeAtomic(p.bin, data, 0o755); err != nil {
			return p, fmt.Errorf("installing %s: %w", p.bin, err)
		}
	}
	if err := writeAtomic(p.icon, iconSVG, 0o644); err != nil {
		return p, fmt.Errorf("installing the icon: %w", err)
	}
	entry := []byte(desktopEntry(p.bin))
	if err := writeAtomic(p.entry, entry, 0o644); err != nil {
		return p, fmt.Errorf("adding the menu entry: %w", err)
	}
	if err := writeAtomic(p.shortcut, entry, 0o755); err != nil {
		return p, fmt.Errorf("adding the desktop icon: %w", err)
	}
	refreshMenus(filepath.Dir(p.entry))
	return p, nil
}

func uninstall(home string) error {
	p := pathsFor(home)
	forgetKDEHotkeys()
	var errs []error
	for _, path := range []string{p.shortcut, p.entry, p.icon, p.bin} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	refreshMenus(filepath.Dir(p.entry))
	return errors.Join(errs...)
}

func runInstall() int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	p, err := install(home, exe)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Installed %s.\nDouble-click AutMouseClicker on the desktop or open it from the app menu.\n", p.bin)
	return 0
}

func runUninstall() int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := uninstall(home); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Removed AutMouseClicker, its icons and its hotkeys. Settings stay in ~/.config/fyne.")
	return 0
}
