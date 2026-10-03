package scan

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemBrowsers asks Windows where browsers registered themselves ("App Paths"): this finds installs in custom folders
// and other drives that the standard locations miss.
func systemBrowsers() []string {
	var r []string
	for _, exe := range []string{"chrome.exe", "msedge.exe", "brave.exe", "vivaldi.exe", "opera.exe"} {
		for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
			k, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+exe, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			if v, _, err := k.GetStringValue(""); err == nil {
				r = append(r, strings.Trim(v, `"`))
			}
			k.Close()
		}
	}
	return r
}
