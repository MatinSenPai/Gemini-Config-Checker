// Package buildinfo carries values stamped at release time.
package buildinfo

// Version is set by the release workflow:
//
//	go build -ldflags "-X gemini-config-checker/internal/buildinfo.Version=v1.2.3"
var Version = "dev"
