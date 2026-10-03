package scan

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
)

// Profile is the anti-DPI recipe for the connection that leaves the user's machine (the entry hop of the base
// chain). Where it applies: inside Iran the Cloudflare configs mostly only connect with these settings. The values
// change from time to time, so every field is user-editable; the defaults are the published ones (t.me/MatinSenPaii/5469).
type Profile struct {
	Enabled      bool   `json:"enabled"`
	Finalmask    string `json:"finalmask"`    // xray "finalmask" JSON (fragment masks)
	Fingerprint  string `json:"fingerprint"`  // e.g. "unsafe"; empty keeps the config's own
	ALPN         string `json:"alpn"`         // comma separated, e.g. "http/1.1"
	CipherSuites string `json:"cipherSuites"` // colon separated TLS suite names
	CleanIP      string `json:"cleanIP"`      // replaces the entry hop's address; the old domain stays as SNI / host
}

func DefaultProfile() Profile {
	return Profile{
		Enabled:     true,
		Finalmask:   `{"tcp": [{"type": "fragment", "settings": {"packets": "tlshello", "lengths": ["0", "104", "1"], "delays": ["0"], "maxSplit": "0"}},{"type": "fragment", "settings": {"packets": "1-1", "lengths": ["114", "1"], "delays": ["1"], "maxSplit": "11"}}]}`,
		Fingerprint: "unsafe",
		ALPN:        "http/1.1",
		CipherSuites: "TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_128_GCM_SHA256:TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384:" +
			"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256:TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:" +
			"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256:TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256:TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:" +
			"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256:TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
	}
}

func splitList(s string) []string {
	var r []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			r = append(r, x)
		}
	}
	return r
}

// applyProfile writes the profile into the given entry hops (maps are edited in place). Fields the profile leaves
// empty keep whatever the config already had; non-empty fields win.
func applyProfile(entries []obj, p Profile) error {
	if !p.Enabled {
		return nil
	}
	var mask any
	if strings.TrimSpace(p.Finalmask) != "" {
		var err error
		if mask, err = jsonAny(p.Finalmask); err != nil {
			return err
		}
	}
	for _, o := range entries {
		ss := sub(o, "streamSettings")
		if ss == nil {
			ss = obj{}
			o["streamSettings"] = ss
		}
		if ip := strings.TrimSpace(p.CleanIP); ip != "" {
			setAddr(o, ip, 0)
		}
		if ss["security"] == "tls" {
			t := sub(ss, "tlsSettings")
			if t == nil {
				t = obj{}
				ss["tlsSettings"] = t
			}
			if p.Fingerprint != "" {
				t["fingerprint"] = p.Fingerprint
			}
			if a := splitList(p.ALPN); len(a) > 0 {
				t["alpn"] = a
			}
			if p.CipherSuites != "" {
				t["cipherSuites"] = strings.TrimSpace(p.CipherSuites)
			}
		}
		if mask != nil {
			ss["finalmask"] = mask
		}
	}
	return nil
}

// parseFinalmaskText validates the profile's finalmask early (simple mode builds a forwarder per config later).
func parseFinalmaskText(p Profile) ([]fragCfg, error) {
	if !p.Enabled || strings.TrimSpace(p.Finalmask) == "" {
		return nil, nil
	}
	v, err := jsonAny(p.Finalmask)
	if err != nil {
		return nil, err
	}
	return parseFinalmask(v)
}

// prepareBase returns a copy of the base ready for the embedded engine: the profile is applied and every
// finalmask is replaced by a local fragmenting forwarder in front of its hop. The returned func stops the forwarders.
func prepareBase(b Base, p Profile) (Base, func(), error) {
	nb := Base{Exit: b.Exit}
	for _, o := range b.Outs {
		nb.Outs = append(nb.Outs, clone(o))
	}
	var proxies []*fragProxy
	closeAll := func() {
		for _, x := range proxies {
			x.Close()
		}
	}
	entries := nb.entryHops()
	if err := applyProfile(entries, p); err != nil {
		return b, func() {}, err
	}
	for _, o := range entries {
		ss := sub(o, "streamSettings")
		if ss == nil || ss["finalmask"] == nil {
			continue
		}
		cfgs, err := parseFinalmask(ss["finalmask"])
		if err != nil {
			closeAll()
			return b, func() {}, err
		}
		delete(ss, "finalmask") // the stock engine cannot parse the fork's schema; the forwarder does the work
		if len(cfgs) == 0 {
			continue
		}
		addr, port := addrPort(o)
		if addr == "" || port == 0 {
			continue
		}
		fp, err := startFragProxy(net.JoinHostPort(addr, strconv.Itoa(port)), cfgs)
		if err != nil {
			closeAll()
			return b, func() {}, err
		}
		proxies = append(proxies, fp)
		setAddr(o, "127.0.0.1", fp.port())
	}
	return nb, closeAll, nil
}

// ExtractProfile reads the anti-DPI values out of a pasted config (link or xray JSON), so they can be shown in,
// and edited from, the settings form.
func ExtractProfile(text string) (Profile, error) {
	b, err := parseBase(text)
	if err != nil {
		return Profile{}, err
	}
	p := Profile{Enabled: true}
	found := false
	for _, o := range b.entryHops() {
		ss := sub(o, "streamSettings")
		if ss == nil {
			continue
		}
		if fm := ss["finalmask"]; fm != nil {
			if j, err := json.Marshal(fm); err == nil {
				p.Finalmask, found = string(j), true
			}
		}
		if t := sub(ss, "tlsSettings"); t != nil {
			if s, _ := t["fingerprint"].(string); s != "" {
				p.Fingerprint = s
				found = found || s == "unsafe" // any other fingerprint (chrome is the parser's default) is not an anti-DPI value by itself
			}
			switch a := t["alpn"].(type) {
			case []any:
				var l []string
				for _, v := range a {
					l = append(l, v.(string))
				}
				p.ALPN, found = strings.Join(l, ","), found || len(l) > 0
			case []string:
				p.ALPN, found = strings.Join(a, ","), found || len(a) > 0
			}
			if s, _ := t["cipherSuites"].(string); s != "" {
				p.CipherSuites, found = s, true
			}
		}
	}
	if !found {
		return Profile{}, errors.New("در این کانفیگ مقدار ضد فیلتری (finalmask / cipherSuites / alpn / fingerprint) پیدا نشد")
	}
	return p, nil
}
