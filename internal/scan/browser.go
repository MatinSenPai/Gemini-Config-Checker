package scan

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Finding a browser the region check can drive over DevTools. Any Chromium-based browser works; the lists below are
// ordered by how likely each is to be the one the user actually signs in with. The candidate lists are pure functions
// of the platform, so they are unit-tested on every OS; only the existence checks touch the machine.

var errNoBrowser = errors.New("مرورگر Chrome / Edge / Brave پیدا نشد؛ یکی از آن‌ها را نصب کن (یا مسیرش را در متغیر GCC_BROWSER بگذار)")

// findBrowser returns the path of a usable browser. GCC_BROWSER (a path), CHROME_BIN and CHROME_PATH win over the search.
func findBrowser() (string, error) {
	if runtime.GOOS == "android" {
		// Android apps cannot start or remote-control another app's browser, so the region check has no browser to use
		// even when Chrome is installed.
		return "", errors.New("روی اندروید مرورگر نصب‌شده قابل کنترل نیست؛ حالت «فقط اتصال» را بزن یا بررسی ریجن را روی کامپیوتر انجام بده")
	}
	home, _ := os.UserHomeDir()
	for _, p := range browserCandidates(runtime.GOOS, os.Getenv, home, exec.LookPath) {
		if isFile(p) {
			return p, nil
		}
	}
	for _, p := range systemBrowsers() { // registry (Windows) / Spotlight (macOS): installs in non-standard places
		if isFile(p) {
			return p, nil
		}
	}
	return "", errNoBrowser
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// browserCandidates lists every path a browser may live at, most likely first. env and look are injected for tests.
func browserCandidates(goos string, env func(string) string, home string, look func(string) (string, error)) []string {
	var c []string
	for _, k := range []string{"GCC_BROWSER", "CHROME_BIN", "CHROME_PATH"} {
		if p := strings.TrimSpace(env(k)); p != "" {
			c = append(c, p)
		}
	}
	switch goos {
	case "windows":
		for _, k := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			if root := env(k); root != "" {
				c = append(c, windowsBrowsers(root)...)
			}
		}
	case "darwin":
		c = append(c, macBrowsers([]string{"/Applications", filepath.Join(home, "Applications")})...)
	default: // linux, freebsd, ...
		c = append(c, linuxBrowsers(home, look)...)
	}
	return c
}

// windowsBrowsers lists where the Chromium browsers install under one root (Program Files, Program Files (x86) or
// %LOCALAPPDATA%). Built from path elements, so it does not depend on the platform it is compiled on.
func windowsBrowsers(root string) []string {
	var r []string
	for _, p := range [][]string{
		{"Google", "Chrome", "Application", "chrome.exe"},
		{"Microsoft", "Edge", "Application", "msedge.exe"},
		{"BraveSoftware", "Brave-Browser", "Application", "brave.exe"},
		{"Chromium", "Application", "chrome.exe"},
		{"Vivaldi", "Application", "vivaldi.exe"},
		{"Opera", "opera.exe"},
		{"Programs", "Opera", "opera.exe"},
		{"Yandex", "YandexBrowser", "Application", "browser.exe"},
	} {
		r = append(r, filepath.Join(append([]string{root}, p...)...))
	}
	return r
}

// macApps: app bundle name -> executable inside Contents/MacOS (they are the same for every browser we support).
var macApps = []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium", "Vivaldi", "Arc", "Google Chrome Beta", "Google Chrome Canary", "Opera"}

// macBundleIDs: for Spotlight lookups of installs outside /Applications.
var macBundleIDs = map[string]string{
	"com.google.Chrome":          "Google Chrome",
	"com.microsoft.edgemac":      "Microsoft Edge",
	"com.brave.Browser":          "Brave Browser",
	"org.chromium.Chromium":      "Chromium",
	"com.vivaldi.Vivaldi":        "Vivaldi",
	"company.thebrowser.Browser": "Arc",
}

func macBrowsers(dirs []string) []string {
	var r []string
	for _, d := range dirs {
		for _, app := range macApps {
			r = append(r, filepath.Join(d, app+".app", "Contents", "MacOS", app))
		}
	}
	return r
}

// macExecutable turns an app bundle path found by Spotlight into its executable.
func macExecutable(appPath, execName string) string {
	return filepath.Join(appPath, "Contents", "MacOS", execName)
}

func linuxBrowsers(home string, look func(string) (string, error)) []string {
	names := []string{"google-chrome", "google-chrome-stable", "google-chrome-beta", "chromium", "chromium-browser", "microsoft-edge",
		"microsoft-edge-stable", "brave-browser", "brave-browser-stable", "brave", "vivaldi", "vivaldi-stable", "opera", "chrome", "msedge"}
	var c []string
	for _, n := range names {
		if p, err := look(n); err == nil {
			c = append(c, p)
		}
	}
	// A desktop launcher often starts with a short PATH, so also look in the usual directories directly.
	for _, d := range []string{"/usr/bin", "/usr/local/bin", "/snap/bin", filepath.Join(home, ".local", "bin")} {
		for _, n := range names {
			c = append(c, filepath.Join(d, n))
		}
	}
	// Vendor packages put the real binary here (the /usr/bin names are symlinks or wrapper scripts to these).
	c = append(c,
		"/opt/google/chrome/chrome", "/opt/google/chrome/google-chrome",
		"/opt/brave.com/brave/brave", "/opt/brave.com/brave/brave-browser",
		"/opt/microsoft/msedge/msedge", "/opt/microsoft/msedge/microsoft-edge",
		"/opt/vivaldi/vivaldi", "/opt/opera/opera",
		"/usr/lib/chromium/chromium", "/usr/lib/chromium-browser/chromium-browser", "/usr/lib64/chromium-browser/chromium-browser",
	)
	return c
}

// snapBrowser reports whether the browser is a snap, and the directory snaps may write to. Snap confinement blocks
// hidden directories under $HOME (like ~/.config), so a profile there makes the browser exit immediately.
func snapProfileBase(exe, home string) (string, bool) {
	if !strings.HasPrefix(exe, "/snap/") {
		return "", false
	}
	return filepath.Join(home, "snap", filepath.Base(exe), "common"), true
}

// profileDir is where the isolated browser profile lives.
func profileDir() string {
	if exe, err := findBrowser(); err == nil {
		if home, herr := os.UserHomeDir(); herr == nil {
			if base, ok := snapProfileBase(exe, home); ok {
				return filepath.Join(base, "GeminiConfigChecker", "browser-profile")
			}
		}
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "GeminiConfigChecker", "browser-profile")
}
