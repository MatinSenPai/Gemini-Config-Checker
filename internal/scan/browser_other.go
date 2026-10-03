//go:build !windows && !darwin

package scan

// systemBrowsers: nothing beyond the standard locations on this platform.
func systemBrowsers() []string { return nil }
