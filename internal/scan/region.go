package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Region check = what a person sees. Google decides the region for a signed-in account, and only *after* the page
// has loaded (the app calls an internal RPC and then redirects), so we open AI Studio and Gemini in an isolated
// browser context, routed through the candidate chain, with the user's session cookies, and read the outcome:
//
//	AI Studio blocked: redirected to /docs/available-regions
//	Gemini blocked:    page says "Gemini isn't currently supported in your country"
const (
	studioURL = "https://aistudio.google.com/prompts/new_chat"
	geminiURL = "https://gemini.google.com/app"

	settleAfterLoad = 10 * time.Second // the block redirect arrives 5-10 s after the page first renders
	maxWait         = 50 * time.Second
)

type Region struct{ Studio, Gemini string } // each: ok | blocked | error | signin

func studioVerdict(href, text string) (verdict string, loaded bool) {
	switch {
	case strings.Contains(href, "available-region"):
		return "blocked", true
	case strings.Contains(href, "accounts.google.com"), strings.Contains(href, "/welcome"):
		return "signin", true
	}
	return "", strings.Contains(text, "Playground")
}

func geminiVerdict(href, text string) (verdict string, loaded bool) {
	switch {
	case strings.Contains(strings.ToLower(text), "currently supported in your country"):
		return "blocked", true
	case strings.Contains(href, "accounts.google.com"):
		return "signin", true
	}
	return "", strings.Contains(href, "gemini.google.com/app") && strings.Contains(text, "Gemini")
}

type page struct {
	conn *websocket.Conn
	id   int
}

func (b *browser) open(ctxID, url string) (*page, error) {
	params := obj{"url": url}
	if ctxID != "" { // empty = the browser's default context
		params["browserContextId"] = ctxID
	}
	r, err := b.call("Target.createTarget", params)
	if err != nil {
		return nil, err
	}
	var v struct{ TargetId string }
	if err := json.Unmarshal(r, &v); err != nil {
		return nil, err
	}
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/devtools/page/%s", b.port, v.TargetId), nil)
	if err != nil {
		return nil, err
	}
	return &page{conn: conn}, nil
}

func (p *page) read() (href, text string) {
	p.id++
	p.conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	p.conn.SetWriteDeadline(time.Now().Add(8 * time.Second))
	if p.conn.WriteJSON(obj{"id": p.id, "method": "Runtime.evaluate", "params": obj{"returnByValue": true,
		"expression": "JSON.stringify([location.href,(document.body?document.body.innerText:'').slice(0,800)])"}}) != nil {
		return "", ""
	}
	for {
		var m struct {
			ID     int
			Result struct{ Result struct{ Value string } }
		}
		if p.conn.ReadJSON(&m) != nil {
			return "", ""
		}
		if m.ID == p.id {
			var a []string
			if json.Unmarshal([]byte(m.Result.Result.Value), &a) == nil && len(a) == 2 {
				return a[0], a[1]
			}
			return "", ""
		}
	}
}

// cookieParams turns raw cookies into Storage.setCookies params (drops fields the setter rejects, and partitioned cookies).
func cookieParams(in []Cookie) []Cookie {
	keep := map[string]bool{"name": true, "value": true, "domain": true, "path": true, "secure": true, "httpOnly": true,
		"sameSite": true, "expires": true, "priority": true, "sourceScheme": true, "sourcePort": true}
	var out []Cookie
	for _, c := range in {
		if _, part := c["partitionKey"]; part {
			continue
		}
		n := Cookie{}
		for k, v := range c {
			if keep[k] {
				n[k] = v
			}
		}
		if e, _ := n["expires"].(float64); e <= 0 {
			delete(n, "expires") // session cookie
		}
		out = append(out, n)
	}
	return out
}

// region opens both products through the SOCKS proxy at socksPort and waits for Google's verdict.
func (b *browser) region(ctx context.Context, socksPort int, cookies []Cookie) (Region, error) {
	r, err := b.call("Target.createBrowserContext", obj{"proxyServer": fmt.Sprintf("socks5://127.0.0.1:%d", socksPort), "disposeOnDetach": true})
	if err != nil {
		return Region{}, err
	}
	var cx struct{ BrowserContextId string }
	if err := json.Unmarshal(r, &cx); err != nil {
		return Region{}, err
	}
	defer b.call("Target.disposeBrowserContext", obj{"browserContextId": cx.BrowserContextId})
	if _, err := b.call("Storage.setCookies", obj{"cookies": cookieParams(cookies), "browserContextId": cx.BrowserContextId}); err != nil {
		return Region{}, err
	}
	ps, err := b.open(cx.BrowserContextId, studioURL)
	if err != nil {
		return Region{}, err
	}
	defer ps.conn.Close()
	pg, err := b.open(cx.BrowserContextId, geminiURL)
	if err != nil {
		return Region{}, err
	}
	defer pg.conn.Close()

	var res Region
	var studioLoaded, geminiLoaded time.Time
	var studioSign, geminiSign persist
	for start := time.Now(); time.Since(start) < maxWait && (res.Studio == "" || res.Gemini == ""); {
		select {
		case <-ctx.Done():
			return Region{}, ctx.Err()
		case <-time.After(time.Second):
		}
		if res.Studio == "" {
			href, text := ps.read()
			v, loaded := studioVerdict(href, text)
			if !studioSign.holds(v == "signin", signinSettle) && v == "signin" {
				v = "" // AI Studio passes through /welcome for a moment while it loads; only a sign-in page that stays is real
			}
			if loaded && studioLoaded.IsZero() {
				studioLoaded = time.Now()
			}
			if v == "" && !studioLoaded.IsZero() && time.Since(studioLoaded) >= settleAfterLoad {
				v = "ok"
			}
			res.Studio = v
		}
		if res.Gemini == "" {
			href, text := pg.read()
			v, loaded := geminiVerdict(href, text)
			if !geminiSign.holds(v == "signin", signinSettle) && v == "signin" {
				v = ""
			}
			if loaded && geminiLoaded.IsZero() {
				geminiLoaded = time.Now()
			}
			if v == "" && !geminiLoaded.IsZero() && time.Since(geminiLoaded) >= settleAfterLoad {
				v = "ok"
			}
			res.Gemini = v
		}
	}
	if res.Studio == "" {
		res.Studio = "error"
	}
	if res.Gemini == "" {
		res.Gemini = "error"
	}
	return res, nil
}

// signinSettle: a sign-in redirect must persist this long before it counts as "the session is gone".
const signinSettle = 8 * time.Second

// persist reports whether a condition has held continuously for at least d.
type persist struct{ since time.Time }

func (p *persist) holds(cond bool, d time.Duration) bool {
	if !cond {
		p.since = time.Time{}
		return false
	}
	if p.since.IsZero() {
		p.since = time.Now()
	}
	return time.Since(p.since) >= d
}

var errSignedOut = errors.New("نشست Google منقضی شده؛ دوباره وارد شو")

// verdictOf combines the two products according to what the user requires ("both" | "studio" | "gemini").
func verdictOf(need string, r Region) string {
	var v []string
	switch need {
	case "studio":
		v = []string{r.Studio}
	case "gemini":
		v = []string{r.Gemini}
	default:
		v = []string{r.Studio, r.Gemini}
	}
	allOK := true
	for _, s := range v {
		if s == "blocked" {
			return "blocked"
		}
		allOK = allOK && s == "ok"
	}
	if allOK {
		return "ok"
	}
	return "error"
}
