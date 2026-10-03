package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/main/distro/all"
)

// reachURL is only a connectivity probe: any answer from Google's API host (even "API key not valid") proves the
// chain carries traffic. It says nothing about the region; that is judged by the signed-in browser check (region.go).
const reachURL = "https://generativelanguage.googleapis.com/v1beta/models?pageSize=1&key=x"

type Result struct {
	Name   string `json:"name"`
	Link   string `json:"link"`
	Status string `json:"status"` // ok | blocked | error
	Ms     int64  `json:"ms"`     // connectivity latency through the chain
	Studio string `json:"studio"` // AI Studio verdict: ok | blocked | error
	Gemini string `json:"gemini"` // Gemini app verdict: ok | blocked | error
	Err    string `json:"err,omitempty"`
}

// ChainRequest asks for a standalone client config of the chain base -> free.
type ChainRequest struct {
	Base    string  `json:"base"`
	Link    string  `json:"link"`
	Profile Profile `json:"profile"`
	Flavor  string  `json:"flavor"` // "patt": keep the fork's finalmask (PattN / PattNG); "std": stock xray, finalmask dropped
}

// Spelled out so the export does not depend on a geoip.dat being installed next to the client.
var privateCIDRs = []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "::1/128", "fc00::/7", "fe80::/10"}

var inboundSniffing = obj{"enabled": true, "destOverride": []string{"http", "tls"}, "routeOnly": false}

func mixedInbound(tag string, port int) obj {
	return obj{"tag": tag, "port": port, "listen": "127.0.0.1", "protocol": "mixed", "sniffing": inboundSniffing,
		"settings": obj{"auth": "noauth", "udp": true, "allowTransparent": false}}
}

// ChainJSON builds a standalone xray config (local mixed proxy on 10808 and 10809) for the chain base -> free. It follows
// the layout GUI clients such as v2rayN / PattN export themselves: classic vnext/servers settings, a "proxy" exit that
// dials through the base via sockopt.dialerProxy, a pinned-DNS block, and a UDP/443 block rule.
func ChainJSON(r ChainRequest) (string, error) {
	_, free, err := parseLink(r.Link)
	if err != nil {
		return "", err
	}
	chained := strings.TrimSpace(r.Base) != ""
	var outs []obj
	if !chained { // simple mode: the config on its own, no chain
		free["tag"] = "proxy"
		outs = []obj{free}
	} else {
		b, err := parseBase(r.Base)
		if err != nil {
			return "", errors.New("کانفیگ پایه: " + err.Error())
		}
		outs = chainOutbounds(b, free)
	}
	var entries []obj
	for _, o := range outs {
		if dialerOf(o) == "" && (!chained || o["tag"] != "proxy") { // chained: the free hop runs inside the tunnel
			entries = append(entries, o)
		}
	}
	if err := applyProfile(entries, r.Profile); err != nil {
		return "", err
	}
	for _, o := range outs {
		toClassic(o)
		o["mux"] = obj{"enabled": false, "concurrency": -1}
		if ss := sub(o, "streamSettings"); ss != nil && r.Flavor == "std" {
			delete(ss, "finalmask")
		}
	}
	outs = append(outs, obj{"tag": "direct", "protocol": "freedom"}, obj{"tag": "block", "protocol": "blackhole"})
	cfg := obj{
		"log": obj{"loglevel": "warning"},
		"dns": obj{
			"hosts": obj{
				"dns.google":         []string{"8.8.8.8", "8.8.4.4"},
				"cloudflare-dns.com": []string{"104.16.249.249", "104.16.248.249"},
			},
			"servers": []string{"https://cloudflare-dns.com/dns-query"},
			"tag":     "dns-module",
		},
		"inbounds":  []obj{mixedInbound("socks", 10808), mixedInbound("socks2", 10809)},
		"outbounds": outs,
		"routing": obj{"domainStrategy": "AsIs", "rules": []obj{
			{"type": "field", "port": "443", "network": "udp", "outboundTag": "block"},
			{"type": "field", "outboundTag": "direct", "ip": privateCIDRs},
			{"type": "field", "inboundTag": []string{"dns-module"}, "outboundTag": "proxy"},
		}},
	}
	j, err := json.MarshalIndent(cfg, "", "  ")
	return string(j), err
}

// startSocks boots xray with a local SOCKS5 inbound on a free port, so a real browser can be pointed at the chain.
func startSocks(b Base, free obj) (inst *core.Instance, port int, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	port = ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	raw, err := json.Marshal(obj{"log": obj{"loglevel": "none"},
		"inbounds":  []obj{{"tag": "in", "listen": "127.0.0.1", "port": port, "protocol": "socks", "settings": obj{"auth": "noauth"}}},
		"outbounds": chainOutbounds(b, free)})
	if err != nil {
		return nil, 0, err
	}
	cfg, err := core.LoadConfig("json", bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	if inst, err = core.New(cfg); err != nil {
		return nil, 0, err
	}
	return inst, port, inst.Start()
}

// newInstance boots an in-process xray with no inbounds (we dial through it directly).
func newInstance(b Base, free obj) (*core.Instance, error) {
	raw, err := json.Marshal(obj{"log": obj{"loglevel": "none"}, "outbounds": chainOutbounds(b, free)})
	if err != nil {
		return nil, err
	}
	cfg, err := core.LoadConfig("json", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	inst, err := core.New(cfg)
	if err != nil {
		return nil, err
	}
	return inst, inst.Start()
}

func clientFor(inst *core.Instance, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			host, ps, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			p, err := strconv.Atoi(ps)
			if err != nil {
				return nil, err
			}
			return core.Dial(ctx, inst, xnet.TCPDestination(xnet.ParseAddress(host), xnet.Port(p)))
		},
	}}
}

// reach times one request to Google's API host through a running instance. A 400 ("API key not valid") is the
// healthy answer; a 403 page means Google refused the exit IP outright.
func reach(ctx context.Context, inst *core.Instance, timeout time.Duration) (int64, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", reachURL, nil)
	t0 := time.Now()
	resp, err := clientFor(inst, timeout).Do(req)
	if err != nil {
		return 0, errors.New(shorten(err.Error()))
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return time.Since(t0).Milliseconds(), nil
}

func shorten(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

const repo = "0xRadikal/Free-v2ray-Configs"

func sourceURLs(source, custom string) ([]string, error) {
	if source == "custom" {
		if !strings.HasPrefix(custom, "http") {
			return nil, errors.New("لینک دلخواه باید با http شروع شود")
		}
		return []string{custom}, nil
	}
	path := source + "/configs.txt"
	if source == "top100" {
		path = "top100.txt"
	}
	return []string{
		"https://raw.githubusercontent.com/" + repo + "/main/" + path,
		"https://cdn.jsdelivr.net/gh/" + repo + "@main/" + path, // mirror, may lag
	}, nil
}

// fetchList tries each URL directly, then again through the base proxy (for networks where GitHub is blocked).
func fetchList(ctx context.Context, via *Base, urls []string) (string, error) {
	get := func(c *http.Client, u string) (string, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := c.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		return string(b), err
	}
	direct := &http.Client{Timeout: 20 * time.Second}
	for _, u := range urls {
		if s, err := get(direct, u); err == nil {
			return s, nil
		}
	}
	if via != nil {
		if inst, err := newInstance(*via, nil); err == nil {
			defer inst.Close()
			c := clientFor(inst, 30*time.Second)
			for _, u := range urls {
				if s, err := get(c, u); err == nil {
					return s, nil
				}
			}
		}
		return "", errors.New("دانلود لیست ممکن نشد (نه مستقیم، نه از طریق کانفیگ پایه)")
	}
	return "", errors.New("دانلود لیست ممکن نشد؛ اتصال اینترنت یا آدرس لیست را بررسی کن")
}

// extractLinks accepts plain or base64 subscription text and returns deduplicated, shuffled links.
func extractLinks(text string) []string {
	if !strings.Contains(text, "://") {
		if b, err := decodeB64(strings.Join(strings.Fields(text), "")); err == nil {
			text = string(b)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range strings.Fields(text) {
		key, _, _ := strings.Cut(l, "#")
		if !seen[key] && (strings.HasPrefix(l, "vless://") || strings.HasPrefix(l, "vmess://") ||
			strings.HasPrefix(l, "trojan://") || strings.HasPrefix(l, "ss://")) {
			seen[key] = true
			out = append(out, l)
		}
	}
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
