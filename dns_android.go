//go:build android

package main

import (
	"context"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Android has no /etc/resolv.conf, and a CGO-free Go binary then falls back to 127.0.0.1:53, where nothing listens.
// Point the pure-Go resolver at public DNS servers instead (override with GCC_DNS="ip:port,ip:port").
// Each lookup attempt rotates to the next server, so one blocked server does not stall name resolution.
func init() {
	servers := []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"}
	if v := strings.TrimSpace(os.Getenv("GCC_DNS")); v != "" {
		servers = strings.Split(v, ",")
	}
	var n atomic.Uint32
	d := &net.Dialer{Timeout: 4 * time.Second}
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			s := strings.TrimSpace(servers[int(n.Add(1)-1)%len(servers)])
			return d.DialContext(ctx, network, s)
		},
	}
}
