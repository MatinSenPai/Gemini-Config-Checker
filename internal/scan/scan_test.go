package scan

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xtls/xray-core/core"
)

func baseOf(t *testing.T, link string) Base {
	t.Helper()
	b, err := parseBase(link)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseLink(t *testing.T) {
	vmess := "vmess://" + base64.StdEncoding.EncodeToString([]byte(
		`{"ps":"vm","add":"h.example","port":"443","id":"u","net":"ws","host":"h","path":"/p","tls":"tls","sni":"s"}`))
	ssUserinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pw"))
	cases := []struct {
		link, name, proto, addr string
	}{
		{"vless://00000000-0000-4000-8000-000000000001@203.0.113.10:2096?encryption=none&security=tls&sni=a.workers.dev&fp=chrome&type=ws&host=a.workers.dev&path=%2Fp#cf-wd", "cf-wd", "vless", "203.0.113.10"},
		{"trojan://pass%24word@t.example:443?security=tls&sni=t.example&type=grpc&serviceName=g#tr", "tr", "trojan", "t.example"},
		{vmess, "vm", "vmess", "h.example"},
		{"ss://" + ssUserinfo + "@1.2.3.4:8388#ss1", "ss1", "shadowsocks", "1.2.3.4"},
		{"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pw@5.6.7.8:1")) + "#old", "old", "shadowsocks", "5.6.7.8"},
	}
	for _, c := range cases {
		name, out, err := parseLink(c.link)
		if err != nil {
			t.Fatalf("%s: %v", c.link, err)
		}
		if name != c.name || out["protocol"] != c.proto || out["settings"].(obj)["address"] != c.addr {
			t.Errorf("%s: got name=%q out=%v", c.link, name, out)
		}
		// every outbound must be accepted by xray itself
		out["tag"] = "base"
		if inst, err := newInstance(Base{Outs: []obj{out}, Exit: "base"}, nil); err != nil {
			t.Errorf("%s: xray rejected config: %v", c.proto, err)
		} else {
			inst.Close()
		}
	}
	for _, bad := range []string{"hysteria2://x@h:1", "vless://u@h:1?type=kcp", "ss://abc@h:1?plugin=x"} {
		if _, _, err := parseLink(bad); err == nil {
			t.Errorf("%s should be rejected", bad)
		}
	}
}

// Fixtures are the real page states observed through blocked and allowed exits while signed in.
func TestVerdicts(t *testing.T) {
	const studioApp = "Skip to main content EXPLORE chat_spark Playground history History BUILD add New app"
	const geminiBlocked = "Gemini Supercharge your creativity Gemini isn’t currently supported in your country. Stay tuned!"
	for _, c := range []struct {
		name, href, text string
		studio           string
		studioLoaded     bool
	}{
		{"app loaded", "https://aistudio.google.com/prompts/new_chat", studioApp, "", true},
		{"still loading", "https://aistudio.google.com/prompts/new_chat", "", "", false},
		{"blocked redirect", "https://aistudio.google.com/docs/available-regions", "error Failed to list models: permission denied", "blocked", true},
		{"signed out", "https://aistudio.google.com/welcome", "", "signin", true},
	} {
		if v, l := studioVerdict(c.href, c.text); v != c.studio || l != c.studioLoaded {
			t.Errorf("studio %s: got (%q,%v)", c.name, v, l)
		}
	}
	if v, _ := geminiVerdict("https://gemini.google.com/", geminiBlocked); v != "blocked" {
		t.Errorf("gemini blocked: got %q", v)
	}
	if v, l := geminiVerdict("https://gemini.google.com/app", "Gemini Flash Conversation with Gemini Hi"); v != "" || !l {
		t.Errorf("gemini ok page: got (%q,%v)", v, l)
	}
	if v, l := geminiVerdict("https://gemini.google.com/app", ""); v != "" || l {
		t.Errorf("gemini still loading: got (%q,%v)", v, l)
	}
}

func TestVerdictOf(t *testing.T) {
	for _, c := range []struct {
		need string
		r    Region
		want string
	}{
		{"both", Region{"ok", "ok"}, "ok"},
		{"both", Region{"ok", "blocked"}, "blocked"}, // HK-style: Gemini ok, AI Studio blocked (and the reverse)
		{"both", Region{"blocked", "ok"}, "blocked"},
		{"both", Region{"ok", "error"}, "error"},
		{"studio", Region{"ok", "blocked"}, "ok"},
		{"gemini", Region{"blocked", "ok"}, "ok"},
		{"", Region{"ok", "ok"}, "ok"}, // default requires both
	} {
		if got := verdictOf(c.need, c.r); got != c.want {
			t.Errorf("verdictOf(%q,%v) = %s, want %s", c.need, c.r, got, c.want)
		}
	}
}

func TestCookieParams(t *testing.T) {
	in := []Cookie{
		{"name": "SID", "value": "x", "domain": ".google.com", "expires": float64(1900000000), "size": 5, "session": false, "httpOnly": true},
		{"name": "S", "value": "y", "domain": ".google.com", "expires": float64(-1), "session": true},
		{"name": "CHIPS", "value": "z", "partitionKey": map[string]any{"topLevelSite": "https://a"}},
	}
	out := cookieParams(in)
	if len(out) != 2 {
		t.Fatalf("partitioned cookie should be dropped: %v", out)
	}
	if _, ok := out[0]["size"]; ok {
		t.Error("size must be stripped")
	}
	if _, ok := out[1]["expires"]; ok {
		t.Error("session cookie must not carry expires")
	}
}

// A trimmed copy of a real chain export: a Reality exit that dials through a Cloudflare-worker hop.
const pastedChain = `{
 "outbounds": [
  {"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"203.0.113.20","port":443,"users":[{"id":"00000000-0000-4000-8000-000000000002","encryption":"none"}]}]},
   "streamSettings":{"network":"raw","security":"reality","realitySettings":{"serverName":"android.clients.google.com","fingerprint":"chrome","publicKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","shortId":"00000000","spiderX":"/x"},
    "sockopt":{"dialerProxy":"chain-proxy-1-cf"}}},
  {"tag":"chain-proxy-1-cf","protocol":"vless","settings":{"vnext":[{"address":"203.0.113.10","port":2096,"users":[{"id":"00000000-0000-4000-8000-000000000001","encryption":"none"}]}]},
   "streamSettings":{"network":"ws","security":"tls","tlsSettings":{"serverName":"w.example.workers.dev","fingerprint":"chrome"},"wsSettings":{"path":"/p","host":"w.example.workers.dev"}}},
  {"tag":"direct","protocol":"freedom"},{"tag":"block","protocol":"blackhole"}]}`

const freeTrojan = "trojan://pw@t.example:443?security=tls&sni=t.example#free"

func TestParseBaseChain(t *testing.T) {
	b := baseOf(t, pastedChain)
	if b.Exit != "proxy" || len(b.Outs) != 2 {
		t.Fatalf("want exit=proxy and 2 proxy outbounds (freedom/blackhole dropped), got exit=%q n=%d", b.Exit, len(b.Outs))
	}
	if e := b.entryHops(); len(e) != 1 || e[0]["tag"] != "chain-proxy-1-cf" {
		t.Fatalf("the Cloudflare hop is the only entry hop: %v", e)
	}
	_, free, _ := parseLink(freeTrojan)
	outs := chainOutbounds(b, free)
	if len(outs) != 3 || outs[0]["tag"] != "proxy" || dialerOf(outs[0]) != "chain-proxy" || outs[1]["tag"] != "chain-proxy" || dialerOf(outs[1]) != "chain-proxy-1-cf" {
		t.Fatalf("chain wiring wrong: %v", outs)
	}
	inst, err := newInstance(b, free)
	if err != nil {
		t.Fatalf("xray rejected the pasted chain: %v", err)
	}
	inst.Close()
	if _, err := parseBase(`{"outbounds":[{"tag":"d","protocol":"freedom"}]}`); err == nil {
		t.Error("a config without proxy outbounds must be rejected")
	}
}

func TestChainJSON(t *testing.T) {
	base := "vless://u@104.18.1.1:443?security=tls&sni=a.workers.dev&type=ws&host=a.workers.dev&path=%2Fp#base"
	for _, flavor := range []string{"patt", "std"} {
		s, err := ChainJSON(ChainRequest{Base: base, Link: freeTrojan, Profile: DefaultProfile(), Flavor: flavor})
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Inbounds  []struct{ Protocol string }
			Outbounds []struct {
				Tag      string
				Protocol string
				Settings map[string]any
				Stream   map[string]any `json:"streamSettings"`
			}
		}
		if err := json.Unmarshal([]byte(s), &cfg); err != nil {
			t.Fatal(err)
		}
		o := cfg.Outbounds
		if len(o) != 4 || o[0].Tag != "proxy" || o[1].Tag != "chain-base" || o[2].Tag != "direct" || o[3].Tag != "block" {
			t.Fatalf("%s: outbounds wrong: %s", flavor, s)
		}
		if so := o[0].Stream["sockopt"].(map[string]any); so["dialerProxy"] != "chain-base" {
			t.Errorf("%s: free must dial through the base: %v", flavor, so)
		}
		if o[0].Settings["servers"] == nil || o[1].Settings["vnext"] == nil {
			t.Errorf("%s: settings must use the classic servers/vnext layout: %v / %v", flavor, o[0].Settings, o[1].Settings)
		}
		if cfg.Inbounds[0].Protocol != "mixed" {
			t.Errorf("%s: inbound should be mixed", flavor)
		}
		entry := o[1].Stream
		if _, has := entry["finalmask"]; has != (flavor == "patt") {
			t.Errorf("%s: finalmask present=%v", flavor, has)
		}
		if _, has := o[0].Stream["finalmask"]; has {
			t.Errorf("%s: the free hop runs inside the tunnel and must not get the profile", flavor)
		}
		tl := entry["tlsSettings"].(map[string]any)
		if tl["fingerprint"] != "unsafe" || !strings.HasPrefix(tl["cipherSuites"].(string), "TLS_AES_256_GCM_SHA384:") || tl["alpn"].([]any)[0] != "http/1.1" {
			t.Errorf("%s: profile not applied to the entry hop: %v", flavor, tl)
		}
		if flavor == "std" { // stock xray must accept the exported file as is
			if _, err := core.LoadConfig("json", strings.NewReader(s)); err != nil {
				t.Errorf("stock xray rejected the std export: %v", err)
			}
		}
	}
}

func TestExtractAndCleanIP(t *testing.T) {
	p, err := ExtractProfile(`{"outbounds":[{"tag":"x","protocol":"vless","settings":{"address":"h.example","port":443,"id":"u"},
	  "streamSettings":{"network":"ws","security":"tls","tlsSettings":{"fingerprint":"unsafe","alpn":["http/1.1"],"cipherSuites":"A:B"},
	  "finalmask":{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["1"],"delays":["0"],"maxSplit":"0"}}]}}}]}`)
	if err != nil || p.Fingerprint != "unsafe" || p.ALPN != "http/1.1" || p.CipherSuites != "A:B" || !strings.Contains(p.Finalmask, "tlshello") {
		t.Fatalf("extract: %+v err=%v", p, err)
	}
	if _, err := ExtractProfile("vless://u@h.example:443?security=tls&type=ws#plain"); err == nil {
		t.Error("a config without anti-DPI values should say so")
	}

	b := baseOf(t, "vless://u@w.example.workers.dev:443?security=tls&type=ws&path=%2F#w")
	pr := DefaultProfile()
	pr.CleanIP = "188.114.97.6"
	if err := applyProfile(b.entryHops(), pr); err != nil {
		t.Fatal(err)
	}
	o := b.Outs[0]
	addr, _ := addrPort(o)
	tl := sub(sub(o, "streamSettings"), "tlsSettings")
	if addr != "188.114.97.6" || tl["serverName"] != "w.example.workers.dev" || sub(sub(o, "streamSettings"), "wsSettings")["host"] != "w.example.workers.dev" {
		t.Errorf("clean IP must keep the old domain as SNI and WS host: addr=%s tls=%v", addr, tl)
	}
}

// ---- fragmenting: the post's values, byte for byte ----

type recorder struct{ writes [][]byte }

func (r *recorder) Write(p []byte) (int, error) {
	r.writes = append(r.writes, append([]byte(nil), p...))
	return len(p), nil
}

func fakeHello(n int) []byte {
	p := make([]byte, 5+n)
	p[0], p[1], p[2], p[3], p[4] = 22, 3, 1, byte(n>>8), byte(n)
	for i := 5; i < len(p); i++ {
		p[i] = byte(i)
	}
	return p
}

func postMask(t *testing.T) []fragCfg {
	t.Helper()
	m, err := jsonAny(DefaultProfile().Finalmask)
	if err != nil {
		t.Fatal(err)
	}
	cfgs, err := parseFinalmask(m)
	if err != nil || len(cfgs) != 2 {
		t.Fatalf("parse: %v (%d masks)", err, len(cfgs))
	}
	return cfgs
}

func TestFragmentPostValues(t *testing.T) {
	hello := fakeHello(300)
	rec := &recorder{}
	if _, err := chainWriters(postMask(t), rec).Write(hello); err != nil {
		t.Fatal(err)
	}
	// mask 1 turns the hello into TLS records [0, 104, 1, 1, ...] sent as ONE write; mask 2 then cuts that write
	// into 114 bytes + single bytes, at most 11 writes.
	if len(rec.writes) != 11 || len(rec.writes[0]) != 114 || len(rec.writes[1]) != 1 || len(rec.writes[9]) != 1 {
		var sizes []int
		for _, w := range rec.writes {
			sizes = append(sizes, len(w))
		}
		t.Fatalf("TCP pieces wrong: %v", sizes)
	}
	var wire []byte
	for _, w := range rec.writes {
		wire = append(wire, w...)
	}
	var lens []int
	var payload []byte
	for len(wire) > 0 {
		l := int(wire[3])<<8 | int(wire[4])
		if wire[0] != 22 || wire[1] != 3 || wire[2] != 1 || len(wire) < 5+l {
			t.Fatalf("not a clean record stream at %d bytes left", len(wire))
		}
		lens, payload, wire = append(lens, l), append(payload, wire[5:5+l]...), wire[5+l:]
	}
	if lens[0] != 0 || lens[1] != 104 || lens[2] != 1 || len(lens) != 2+196 {
		t.Errorf("record sizes wrong: first=%v count=%d", lens[:3], len(lens))
	}
	if string(payload) != string(hello[5:]) {
		t.Error("the reassembled handshake differs from the original")
	}
}

func TestFragmentLeavesLaterTrafficAlone(t *testing.T) {
	rec := &recorder{}
	w := chainWriters(postMask(t), rec)
	w.Write(fakeHello(300))
	before := len(rec.writes)
	w.Write([]byte("application data that is long enough to be split if the mask were still active"))
	if len(rec.writes) != before+1 {
		t.Errorf("only the first write may be fragmented, got %d extra writes", len(rec.writes)-before)
	}
}

// A real TLS server must still complete the handshake through the forwarder.
func TestFragmentProxyHandshake(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	target := strings.TrimPrefix(srv.URL, "https://")
	// Go's strict TLS server rejects the post's zero-length first record (Cloudflare accepts it), so this test uses the same
	// recipe with a 1-byte first record: it proves the forwarder frames a ClientHello a real stack can complete.
	m, _ := jsonAny(strings.Replace(DefaultProfile().Finalmask, `["0", "104", "1"]`, `["1", "104", "1"]`, 1))
	strict, err := parseFinalmask(m)
	if err != nil {
		t.Fatal(err)
	}
	for name, cfgs := range map[string][]fragCfg{"post recipe, 1-byte first record": strict, "no masks": nil} {
		fp, err := startFragProxy(target, cfgs)
		if err != nil {
			t.Fatal(err)
		}
		c, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(fp.port())), &tls.Config{InsecureSkipVerify: true, ServerName: "example.com"})
		if err != nil {
			t.Errorf("%s: handshake through the forwarder failed: %v", name, err)
		} else {
			c.Close()
		}
		fp.Close()
	}
}

func TestExtractLinks(t *testing.T) {
	plain := "vless://a@h:1#x\nvless://a@h:1#dup\nfoo\nss://b@h:2"
	if n := len(extractLinks(plain)); n != 2 {
		t.Errorf("plain: got %d links", n)
	}
	if n := len(extractLinks(base64.StdEncoding.EncodeToString([]byte(plain)))); n != 2 {
		t.Errorf("base64: got %d links", n)
	}
}

func TestLinkCarriesAntiDPIValues(t *testing.T) {
	fm := `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","lengths":["1"],"delays":["0"],"maxSplit":"0"}}]}`
	link := "vless://u@h.example:443?security=tls&type=ws&path=%2F&fp=unsafe&alpn=http%2F1.1&ciphers=TLS_AES_128_GCM_SHA256" +
		"&fm=" + url.QueryEscape(fm) + "#x"
	p, err := ExtractProfile(link)
	if err != nil || p.Fingerprint != "unsafe" || p.ALPN != "http/1.1" || p.CipherSuites != "TLS_AES_128_GCM_SHA256" || !strings.Contains(p.Finalmask, "tlshello") {
		t.Fatalf("link values not read: %+v err=%v", p, err)
	}
	if _, err := ExtractProfile("vless://u@h.example:443?security=tls&type=ws&fm=%7Bnot-json#x"); err == nil {
		t.Error("an invalid finalmask in a link must be reported")
	}
}

// Simple mode has no base config: "copy full config" is the single config plus the anti-DPI profile.
func TestChainJSONWithoutBase(t *testing.T) {
	link := "vless://u@104.18.1.1:443?security=tls&sni=a.workers.dev&type=ws&host=a.workers.dev&path=%2Fp#one"
	s, err := ChainJSON(ChainRequest{Base: "", Link: link, Profile: DefaultProfile(), Flavor: "patt"})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Outbounds []struct {
			Tag    string
			Stream map[string]any `json:"streamSettings"`
		}
	}
	if err := json.Unmarshal([]byte(s), &cfg); err != nil {
		t.Fatal(err)
	}
	o := cfg.Outbounds
	if len(o) != 3 || o[0].Tag != "proxy" {
		t.Fatalf("want proxy + direct + block, got %s", s)
	}
	if _, chained := o[0].Stream["sockopt"]; chained {
		t.Error("simple mode must not chain through anything")
	}
	if _, ok := o[0].Stream["finalmask"]; !ok {
		t.Error("the config connects from the user's machine, so it must carry the anti-DPI profile")
	}
}

func TestStartValidation(t *testing.T) {
	if err := Start(Request{Level: "advanced", Mode: "reach"}); err == nil {
		t.Error("advanced mode without a base config must be rejected")
	}
	if err := Start(Request{Mode: "reach"}); err == nil {
		t.Error("the default level is advanced, so a missing base config must be rejected")
	}
}

// A sign-in redirect that only flashes by (AI Studio passes through /welcome while loading) must not count.
func TestPersist(t *testing.T) {
	var p persist
	d := 60 * time.Millisecond
	if p.holds(true, d) {
		t.Fatal("must not hold immediately")
	}
	time.Sleep(80 * time.Millisecond)
	if !p.holds(true, d) {
		t.Fatal("must hold after the settle time")
	}
	p.holds(false, d) // the page moved on: the clock resets
	if p.holds(true, d) {
		t.Fatal("the timer must restart after a break")
	}
}

func TestExtractProfileSpellings(t *testing.T) {
	const suites = "TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384"
	for name, in := range map[string]string{
		"array":   `{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"h.example","port":443,"users":[{"id":"u"}]}]},"streamSettings":{"network":"ws","security":"tls","tlsSettings":{"alpn":"http/1.1","cipherSuites":["TLS_AES_128_GCM_SHA256","TLS_AES_256_GCM_SHA384"]}}}]}`,
		"ciphers": `{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"h.example","port":443,"users":[{"id":"u"}]}]},"streamSettings":{"network":"ws","security":"tls","tlsSettings":{"Ciphers":"` + suites + `"}}}]}`,
		"link":    "vless://u@h.example:443?security=tls&type=ws&path=%2F&CipherSuites=" + suites,
		"vmess":   "vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"v":"2","add":"h.example","port":"443","id":"u","net":"ws","tls":"tls","path":"/","ciphers":"`+suites+`"}`)),
	} {
		p, err := ExtractProfile(in)
		if err != nil || p.CipherSuites != suites {
			t.Errorf("%s: got %q, %v", name, p.CipherSuites, err)
		}
	}
}
