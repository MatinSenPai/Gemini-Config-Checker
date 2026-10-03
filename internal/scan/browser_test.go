package scan

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) string           { return "" }
func noLook(string) (string, error) { return "", errors.New("not found") }
func slash(p string) string         { return filepath.ToSlash(p) }
func has(list []string, suffix string) bool {
	for _, p := range list {
		if strings.HasSuffix(slash(p), suffix) {
			return true
		}
	}
	return false
}

func TestWindowsBrowserCandidates(t *testing.T) {
	env := func(k string) string {
		return map[string]string{"ProgramFiles": "C:/PF", "ProgramFiles(x86)": "C:/PF86", "LOCALAPPDATA": "C:/Users/u/AppData/Local"}[k]
	}
	c := browserCandidates("windows", env, "C:/Users/u", noLook)
	for _, want := range []string{
		"PF/Google/Chrome/Application/chrome.exe",
		"PF86/Microsoft/Edge/Application/msedge.exe",
		"Local/BraveSoftware/Brave-Browser/Application/brave.exe",
		"Local/Google/Chrome/Application/chrome.exe",
		"Local/Programs/Opera/opera.exe",
	} {
		if !has(c, want) {
			t.Errorf("windows: missing %s in %v", want, c)
		}
	}
	for _, p := range c { // the bug this guards against: separators lost in the path strings
		if strings.Contains(p, "GoogleChrome") || strings.Contains(p, "MicrosoftEdge") {
			t.Fatalf("mangled path %q", p)
		}
	}
}

func TestMacBrowserCandidates(t *testing.T) {
	c := browserCandidates("darwin", noEnv, "/Users/u", noLook)
	for _, want := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"/Users/u/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Arc.app/Contents/MacOS/Arc",
	} {
		if !has(c, want) {
			t.Errorf("mac: missing %s", want)
		}
	}
	if got := slash(macExecutable("/Volumes/X/Brave Browser.app", "Brave Browser")); got != "/Volumes/X/Brave Browser.app/Contents/MacOS/Brave Browser" {
		t.Errorf("spotlight executable: %s", got)
	}
}

func TestLinuxBrowserCandidates(t *testing.T) {
	look := func(n string) (string, error) {
		if n == "chromium" {
			return "/custom/bin/chromium", nil
		}
		return "", errors.New("no")
	}
	c := browserCandidates("linux", noEnv, "/home/u", look)
	if c[0] != "/custom/bin/chromium" {
		t.Errorf("PATH hit must come first, got %s", c[0])
	}
	for _, want := range []string{"/usr/bin/google-chrome", "/snap/bin/chromium", "/opt/google/chrome/chrome", "/opt/brave.com/brave/brave", "/opt/microsoft/msedge/msedge", "/home/u/.local/bin/brave"} {
		if !has(c, want) {
			t.Errorf("linux: missing %s", want)
		}
	}
}

func TestBrowserEnvOverrides(t *testing.T) {
	env := func(k string) string {
		return map[string]string{"GCC_BROWSER": "/x/mine", "CHROME_BIN": "/x/puppeteer"}[k]
	}
	for _, goos := range []string{"windows", "darwin", "linux"} {
		c := browserCandidates(goos, env, "/h", noLook)
		if c[0] != "/x/mine" || c[1] != "/x/puppeteer" {
			t.Errorf("%s: overrides must lead, got %v", goos, c[:2])
		}
	}
}

func TestSnapProfile(t *testing.T) {
	if base, ok := snapProfileBase("/snap/bin/chromium", "/home/u"); !ok || slash(base) != "/home/u/snap/chromium/common" {
		t.Errorf("snap: %q %v", base, ok)
	}
	if _, ok := snapProfileBase("/usr/bin/google-chrome", "/home/u"); ok {
		t.Error("a normal browser must keep the regular profile dir")
	}
}
