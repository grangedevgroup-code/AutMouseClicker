//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAddsTheMenuEntryDesktopIconAndBinaryThenUninstallRemovesThem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("PATH", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	exe := filepath.Join(t.TempDir(), "autmouseclicker")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := install(home, exe)
	if err != nil {
		t.Fatal(err)
	}
	want := installPaths{
		bin:      filepath.Join(home, ".local/bin/autmouseclicker"),
		icon:     filepath.Join(home, ".local/share/icons/hicolor/scalable/apps", appID+".svg"),
		entry:    filepath.Join(home, ".local/share/applications", appID+".desktop"),
		shortcut: filepath.Join(home, "Desktop", appID+".desktop"),
	}
	if p != want {
		t.Fatalf("paths %+v, want %+v", p, want)
	}
	if data, _ := os.ReadFile(p.bin); string(data) != "binary" {
		t.Fatalf("installed binary holds %q", data)
	}
	entry, _ := os.ReadFile(p.entry)
	for _, line := range []string{"Exec=" + p.bin, "Icon=" + appID, "Terminal=false", "StartupWMClass=" + appID} {
		if !strings.Contains(string(entry), line+"\n") {
			t.Errorf("menu entry lacks %q:\n%s", line, entry)
		}
	}
	if info, err := os.Stat(p.shortcut); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("desktop icon missing or not executable: %v %v", info, err)
	}
	if _, err := install(home, p.bin); err != nil {
		t.Fatalf("installing from the installed copy failed: %v", err)
	}
	if err := uninstall(home); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.bin, p.icon, p.entry, p.shortcut} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s still exists after uninstall", path)
		}
	}
}

func TestDesktopExecQuotesPathsThatNeedIt(t *testing.T) {
	cases := map[string]string{
		"/home/l/.local/bin/autmouseclicker": "/home/l/.local/bin/autmouseclicker",
		"/home/a b/bin/amc":                  `"/home/a b/bin/amc"`,
		"/srv/$x/amc":                        `"/srv/\\$x/amc"`,
		"/srv/100%/amc":                      "/srv/100%%/amc",
		`/srv/"q"/amc`:                       `"/srv/\\"q\\"/amc"`,
	}
	for path, want := range cases {
		if got := desktopExec(path); got != want {
			t.Errorf("desktopExec(%q) = %q, want %q", path, got, want)
		}
	}
}
