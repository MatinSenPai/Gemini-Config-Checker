package scan

import (
	"os/exec"
	"strings"
)

// systemBrowsers asks Spotlight for browsers installed outside /Applications (an external disk, another folder).
func systemBrowsers() []string {
	var r []string
	for id, execName := range macBundleIDs {
		out, err := exec.Command("mdfind", "kMDItemCFBundleIdentifier == '"+id+"'").Output()
		if err != nil {
			continue
		}
		for _, app := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.HasSuffix(app, ".app") {
				r = append(r, macExecutable(app, execName))
			}
		}
	}
	return r
}
