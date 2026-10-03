package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

// Base is the user's working config: one outbound, or a whole chain of them (e.g. a pasted xray JSON where an exit
// outbound dials through a Cloudflare-worker hop). Free configs are chained *behind* its Exit.
type Base struct {
	Outs []obj  // every proxy outbound of the base chain
	Exit string // tag of the outbound that reaches the internet; free configs dial through it
}

func clone(o obj) obj {
	b, _ := json.Marshal(o)
	var r obj
	json.Unmarshal(b, &r)
	return r
}

// tagged returns a copy of the outbound with the given tag.
func tagged(o obj, tag string) obj {
	c := clone(o)
	c["tag"] = tag
	return c
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case string:
		var n int
		fmt.Sscan(x, &n)
		return n
	}
	return 0
}

func sub(m obj, k string) obj {
	if v, ok := m[k].(obj); ok {
		return v
	}
	return nil
}

// parseBase accepts a share link, a full xray config (JSON), a bare outbound, or an array of outbounds.
func parseBase(text string) (Base, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Base{}, errors.New("کانفیگ پایه خالی است")
	}
	if text[0] != '{' && text[0] != '[' {
		_, out, err := parseLink(text)
		if err != nil {
			return Base{}, err
		}
		out["tag"] = "base"
		return Base{Outs: []obj{out}, Exit: "base"}, nil
	}
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return Base{}, errors.New("JSON نامعتبر: " + err.Error())
	}
	var list []any
	switch v := raw.(type) {
	case []any:
		list = v
	case obj:
		if o, ok := v["outbounds"].([]any); ok {
			list = o
		} else if _, ok := v["protocol"]; ok {
			list = []any{v}
		}
	}
	var b Base
	for i, x := range list {
		o, ok := x.(obj)
		if !ok {
			continue
		}
		switch o["protocol"] {
		case "freedom", "blackhole", "dns", "loopback", nil:
			continue
		}
		if t, _ := o["tag"].(string); t == "" {
			o["tag"] = fmt.Sprintf("out-%d", i)
		}
		b.Outs = append(b.Outs, o)
	}
	if len(b.Outs) == 0 {
		return Base{}, errors.New("در JSON هیچ outbound پروکسی‌ای پیدا نشد")
	}
	b.Exit, _ = b.Outs[0]["tag"].(string) // xray's default route is the first outbound
	return b, nil
}

func dialerOf(o obj) string {
	if ss := sub(o, "streamSettings"); ss != nil {
		if so := sub(ss, "sockopt"); so != nil {
			s, _ := so["dialerProxy"].(string)
			return s
		}
	}
	return ""
}

// entryHops are the outbounds that connect straight from the user's machine (they do not dial through another
// outbound). Those are the connections an anti-DPI profile has to protect.
func (b Base) entryHops() []obj {
	var r []obj
	for _, o := range b.Outs {
		if dialerOf(o) == "" {
			r = append(r, o)
		}
	}
	return r
}

// endpoint returns the settings map that holds address/port for any of the three xray layouts
// (flat, "vnext", "servers").
func endpoint(o obj) obj {
	s := sub(o, "settings")
	if s == nil {
		return nil
	}
	for _, k := range []string{"vnext", "servers"} {
		if l, ok := s[k].([]any); ok && len(l) > 0 {
			if m, ok := l[0].(obj); ok {
				return m
			}
		}
	}
	return s
}

func addrPort(o obj) (string, int) {
	e := endpoint(o)
	if e == nil {
		return "", 0
	}
	a, _ := e["address"].(string)
	return a, toInt(e["port"])
}

func isIP(s string) bool { return net.ParseIP(strings.Trim(s, "[]")) != nil }

// setAddr points the outbound at addr:port, first pinning the old domain as SNI / WS host so TLS and the
// worker still see the original name.
func setAddr(o obj, addr string, port int) {
	e := endpoint(o)
	if e == nil {
		return
	}
	if old, _ := e["address"].(string); old != "" && !isIP(old) && old != addr {
		if ss := sub(o, "streamSettings"); ss != nil {
			if t := sub(ss, "tlsSettings"); t != nil {
				if s, _ := t["serverName"].(string); s == "" {
					t["serverName"] = old
				}
			}
			for _, k := range []string{"wsSettings", "httpupgradeSettings", "xhttpSettings"} {
				if t := sub(ss, k); t != nil {
					if s, _ := t["host"].(string); s == "" {
						t["host"] = old
					}
				}
			}
		}
	}
	e["address"] = addr
	if port > 0 {
		e["port"] = port
	}
}

// chainOutbounds returns outbounds for the base alone (exit first = default route), or for the chain
// client -> base chain -> free -> internet. Base tags get a "chain-" prefix so they can never clash with ours.
func chainOutbounds(b Base, free obj) []obj {
	tag := func(t string) string { return "chain-" + strings.TrimPrefix(t, "chain-") }
	var outs []obj
	for _, o := range b.Outs {
		c := clone(o)
		c["tag"] = tag(o["tag"].(string))
		if d := dialerOf(c); d != "" {
			c["streamSettings"].(obj)["sockopt"].(obj)["dialerProxy"] = tag(d)
		}
		if c["tag"] == tag(b.Exit) { // exit first
			outs = append([]obj{c}, outs...)
		} else {
			outs = append(outs, c)
		}
	}
	if free == nil {
		return outs
	}
	f := clone(free)
	ss := sub(f, "streamSettings")
	so := sub(ss, "sockopt")
	if so == nil {
		so = obj{}
		ss["sockopt"] = so
	}
	so["dialerProxy"] = tag(b.Exit)
	f["tag"] = "proxy"
	return append([]obj{f}, outs...)
}

// toClassic rewrites flat settings into the classic layout ("vnext" / "servers") that every xray-based client and
// older cores understand. Already-classic outbounds are left alone.
func toClassic(o obj) obj {
	s := sub(o, "settings")
	if s == nil || s["vnext"] != nil || s["servers"] != nil {
		return o
	}
	addr, port := s["address"], s["port"]
	switch o["protocol"] {
	case "vless":
		u := obj{"id": s["id"], "encryption": firstNonEmpty(fmt.Sprint(s["encryption"]), "none"), "email": "t@t.tt"}
		if f, _ := s["flow"].(string); f != "" {
			u["flow"] = f
		}
		o["settings"] = obj{"vnext": []obj{{"address": addr, "port": port, "users": []obj{u}}}}
	case "vmess":
		o["settings"] = obj{"vnext": []obj{{"address": addr, "port": port, "users": []obj{
			{"id": s["id"], "alterId": 0, "security": firstNonEmpty(fmt.Sprint(s["security"]), "auto"), "email": "t@t.tt"}}}}}
	case "trojan":
		o["settings"] = obj{"servers": []obj{{"address": addr, "port": port, "password": s["password"]}}}
	case "shadowsocks":
		o["settings"] = obj{"servers": []obj{{"address": addr, "port": port, "method": s["method"], "password": s["password"]}}}
	}
	return o
}
