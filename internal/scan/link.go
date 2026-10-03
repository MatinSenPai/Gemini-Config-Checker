package scan

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

type obj = map[string]any

// parseLink converts a vless/vmess/trojan/ss share link into an xray outbound.
// Only TCP-based protocols: they are the ones that can be chained through a WS/TCP base.
func parseLink(link string) (name string, out obj, err error) {
	link = strings.TrimSpace(link)
	scheme, rest, ok := strings.Cut(link, "://")
	if !ok {
		return "", nil, errors.New("not a share link")
	}
	switch strings.ToLower(scheme) {
	case "vmess":
		return parseVmess(rest)
	case "vless", "trojan":
		return parseURLStyle(strings.ToLower(scheme), link)
	case "ss":
		return parseSS(link)
	}
	return "", nil, errors.New("unsupported protocol " + scheme)
}

func parseURLStyle(proto, link string) (string, obj, error) {
	u, err := url.Parse(link)
	if err != nil {
		return "", nil, err
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || u.Hostname() == "" || u.User == nil {
		return "", nil, errors.New("bad host/port/user")
	}
	q := u.Query()
	stream, err := buildStream(q.Get("type"), q.Get("security"), q)
	if err != nil {
		return "", nil, err
	}
	var settings obj
	if proto == "vless" {
		settings = obj{"address": u.Hostname(), "port": port, "id": u.User.Username(),
			"encryption": firstNonEmpty(q.Get("encryption"), "none"), "flow": q.Get("flow")}
	} else {
		settings = obj{"address": u.Hostname(), "port": port, "password": u.User.Username()}
	}
	return u.Fragment, obj{"protocol": proto, "settings": settings, "streamSettings": stream}, nil
}

func parseVmess(b64 string) (string, obj, error) {
	raw, err := decodeB64(b64)
	if err != nil {
		return "", nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", nil, err
	}
	s := func(k string) string {
		switch x := v[k].(type) {
		case string:
			return x
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
		return ""
	}
	port, err := strconv.Atoi(s("port"))
	if err != nil || s("add") == "" || s("id") == "" {
		return "", nil, errors.New("bad vmess fields")
	}
	q := url.Values{"host": {s("host")}, "path": {s("path")}, "sni": {s("sni")}, "alpn": {s("alpn")},
		"fp": {s("fp")}, "serviceName": {s("path")}, "fm": {s("fm")}, "finalmask": {s("finalmask")},
		"ciphers": {firstNonEmpty(s("ciphers"), s("cipherSuites"))}}
	sec := ""
	if s("tls") == "tls" {
		sec = "tls"
	}
	stream, err := buildStream(s("net"), sec, q)
	if err != nil {
		return "", nil, err
	}
	settings := obj{"address": s("add"), "port": port, "id": s("id"), "security": firstNonEmpty(s("scy"), "auto")}
	return s("ps"), obj{"protocol": "vmess", "settings": settings, "streamSettings": stream}, nil
}

func parseSS(link string) (string, obj, error) {
	body := strings.TrimPrefix(link, "ss://")
	body, frag, _ := strings.Cut(body, "#")
	name, _ := url.PathUnescape(frag)
	body, query, _ := strings.Cut(body, "?")
	if query != "" {
		return "", nil, errors.New("ss plugin not supported")
	}
	if !strings.Contains(body, "@") { // legacy: base64(method:pass@host:port)
		raw, err := decodeB64(body)
		if err != nil {
			return "", nil, err
		}
		body = string(raw)
	}
	userinfo, hostport, ok := strings.Cut(body, "@")
	if !ok {
		return "", nil, errors.New("bad ss link")
	}
	if !strings.Contains(userinfo, ":") { // SIP002: base64(method:pass)@host:port
		raw, err := decodeB64(userinfo)
		if err != nil {
			return "", nil, err
		}
		userinfo = string(raw)
	} else if u, err := url.PathUnescape(userinfo); err == nil {
		userinfo = u
	}
	method, pass, ok := strings.Cut(userinfo, ":")
	hostport = strings.TrimSuffix(hostport, "/")
	i := strings.LastIndex(hostport, ":")
	if !ok || i < 0 {
		return "", nil, errors.New("bad ss link")
	}
	port, err := strconv.Atoi(hostport[i+1:])
	if err != nil {
		return "", nil, err
	}
	settings := obj{"address": strings.Trim(hostport[:i], "[]"), "port": port, "method": method, "password": pass}
	return name, obj{"protocol": "shadowsocks", "settings": settings,
		"streamSettings": obj{"network": "tcp", "security": "none"}}, nil
}

func buildStream(network, security string, q url.Values) (obj, error) {
	if network == "" {
		network = "tcp"
	}
	host, path := q.Get("host"), q.Get("path")
	s := obj{"network": network, "security": firstNonEmpty(security, "none")}
	switch network {
	case "tcp":
	case "ws":
		ws := obj{"path": path}
		if host != "" {
			ws["host"] = host
		}
		s["wsSettings"] = ws
	case "httpupgrade":
		s["httpupgradeSettings"] = obj{"path": path, "host": host}
	case "xhttp", "splithttp":
		s["network"] = "xhttp"
		s["xhttpSettings"] = obj{"path": path, "host": host, "mode": firstNonEmpty(q.Get("mode"), "auto")}
	case "grpc":
		s["grpcSettings"] = obj{"serviceName": firstNonEmpty(q.Get("serviceName"), path), "multiMode": q.Get("mode") == "multi"}
	default:
		return nil, errors.New("unsupported transport " + network)
	}
	if fm := qv(q, "fm", "finalmask"); fm != "" { // anti-DPI masks travel in the link as URL-encoded JSON
		var v any
		if err := json.Unmarshal([]byte(fm), &v); err != nil {
			return nil, errors.New("finalmask داخل لینک JSON معتبر نیست")
		}
		s["finalmask"] = v
	}
	fp := firstNonEmpty(qv(q, "fp", "fingerprint"), "chrome")
	switch security {
	case "tls":
		tls := obj{"serverName": firstNonEmpty(q.Get("sni"), host), "fingerprint": fp,
			"allowInsecure": q.Get("allowInsecure") == "1" || q.Get("insecure") == "1"}
		if a := qv(q, "alpn"); a != "" {
			tls["alpn"] = splitList(a)
		}
		if c := qv(q, "ciphers", "cipherSuites", "cipher_suites", "cs"); c != "" {
			tls["cipherSuites"] = c
		}
		s["tlsSettings"] = tls
	case "reality":
		s["realitySettings"] = obj{"serverName": q.Get("sni"), "fingerprint": fp, "publicKey": q.Get("pbk"),
			"shortId": q.Get("sid"), "spiderX": q.Get("spx")}
	case "", "none":
	default:
		return nil, errors.New("unsupported security " + security)
	}
	return s, nil
}

func decodeB64(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

// qv reads a query parameter under any of the given names, ignoring case (links in the wild spell them differently).
func qv(q url.Values, names ...string) string {
	for _, n := range names {
		for k, v := range q {
			if strings.EqualFold(k, n) && len(v) > 0 && strings.TrimSpace(v[0]) != "" {
				return strings.TrimSpace(v[0])
			}
		}
	}
	return ""
}
