package scan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Request struct {
	Base        string  `json:"base"`
	Source      string  `json:"source"`
	Custom      string  `json:"custom"`
	Pasted      string  `json:"pasted"`
	Need        string  `json:"need"`        // both | studio | gemini: what "healthy" requires
	Level       string  `json:"level"`       // advanced (default): free configs chained behind Base | simple: each config tested directly
	Mode        string  `json:"mode"`        // region (signed-in browser check, default) | reach (connectivity only, no browser)
	Limit       int     `json:"limit"`       // max configs to test
	Concurrency int     `json:"concurrency"` // chains tested at once (connectivity stage)
	BrowserConc int     `json:"browserConc"` // browser region checks at once (heavy: ~200 MB each)
	Timeout     int     `json:"timeout"`     // seconds, connectivity stage
	Profile     Profile `json:"profile"`     // anti-DPI settings for the base config's entry hop
}

// Status is the JSON snapshot the UI polls. The base link and the Google session are never part of it.
type Status struct {
	State   string   `json:"state"` // idle | running | done | stopped | failed
	Msg     string   `json:"msg"`
	Auth    string   `json:"auth"` // none | waiting | ok | failed
	AuthMsg string   `json:"authMsg"`
	Level   string   `json:"level"`   // simple | advanced, as of the current/last scan
	Mode    string   `json:"mode"`    // the mode of the current/last scan
	Browser bool     `json:"browser"` // a Chromium browser exists on this machine (the region check needs one)
	Base    *Result  `json:"base"`
	Total   int      `json:"total"`
	Done    int      `json:"done"`
	Skipped int      `json:"skipped"`
	Results []Result `json:"results"`
}

// One scan at a time per process.
var (
	mu     sync.Mutex
	cancel context.CancelFunc
	st     = Status{State: "idle", Results: []Result{}}
)

func set(f func()) { mu.Lock(); f(); mu.Unlock() }

func Snapshot() Status {
	mu.Lock()
	s := st
	s.Results = append([]Result{}, st.Results...)
	mu.Unlock()
	s.Auth, s.AuthMsg = authSnapshot()
	s.Browser = hasBrowser()
	return s
}

func Stop() {
	set(func() {
		if cancel != nil {
			cancel()
		}
	})
}

// Clear drops the last results. It does nothing while a scan is running.
func Clear() {
	set(func() {
		if st.State != "running" {
			st = Status{State: "idle", Results: []Result{}}
		}
	})
}

func Start(rq Request) error {
	if rq.Level != "simple" {
		rq.Level = "advanced"
		if strings.TrimSpace(rq.Base) == "" {
			return errors.New("کانفیگ پایه را وارد کن")
		}
	}
	if rq.Mode != "reach" {
		rq.Mode = "region"
		if _, err := findBrowser(); err != nil {
			return errors.New(err.Error() + "؛ یا حالت «فقط اتصال» را انتخاب کن")
		}
		if !LoggedIn() {
			return errors.New("اول با حساب Google وارد شو")
		}
	}
	if rq.Limit <= 0 {
		rq.Limit = 300
	}
	if rq.Concurrency <= 0 || rq.Concurrency > 100 {
		rq.Concurrency = 20
	}
	if rq.BrowserConc <= 0 || rq.BrowserConc > 12 {
		rq.BrowserConc = 4
	}
	if rq.Timeout <= 0 {
		rq.Timeout = 10
	}
	mu.Lock()
	defer mu.Unlock()
	if st.State == "running" {
		return errors.New("یک اسکن در حال اجراست")
	}
	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())
	st = Status{State: "running", Msg: "در حال شروع…", Level: rq.Level, Mode: rq.Mode, Results: []Result{}}
	go func() {
		state, msg := run(ctx, rq)
		set(func() { st.State, st.Msg = state, msg })
	}()
	return nil
}

// hasBrowser reports whether a Chromium browser the region check can drive is installed. Not cached: a browser
// installed after the app started is picked up at once.
func hasBrowser() bool {
	_, err := findBrowser()
	return err == nil
}

// run returns the final state and message. The browser is fully shut down before it returns, so a new scan can
// start immediately without fighting over the profile.
func run(ctx context.Context, rq Request) (state, msg string) {
	simple := rq.Level == "simple"
	var base Base // advanced only: the user's config every free config is chained behind
	var err error
	if !simple {
		parsed, perr := parseBase(rq.Base)
		if perr != nil {
			return "failed", "کانفیگ پایه نامعتبر است: " + perr.Error()
		}
		var stopFrag func()
		if base, stopFrag, err = prepareBase(parsed, rq.Profile); err != nil {
			return "failed", "تنظیمات ضد فیلتر: " + err.Error()
		}
		defer stopFrag()
	} else if _, err := parseFinalmaskText(rq.Profile); err != nil {
		return "failed", "تنظیمات ضد فیلتر: " + err.Error()
	}
	timeout := time.Duration(rq.Timeout) * time.Second

	var br *browser
	var cookies []Cookie
	if rq.Mode == "region" {
		profileMu.Lock() // released after the browser is fully closed (defers run last-in-first-out)
		defer profileMu.Unlock()
		set(func() { st.Msg = "در حال راه‌اندازی مرورگر…" })
		if br, err = launch(true, "about:blank"); err != nil {
			return "failed", err.Error()
		}
		defer br.kill()
		if cookies, err = br.cookies(); err != nil || !hasAuth(cookies) {
			authSet("failed", errSignedOut.Error())
			return "failed", errSignedOut.Error()
		}
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	var signedOut atomic.Bool

	// check runs both stages for one route: connectivity through xray, then the signed-in browser region check.
	// route is the base chain (advanced) or the config itself (simple); free is chained behind it, or nil.
	check := func(route Base, free obj, bsem chan struct{}) Result {
		if rq.Mode == "reach" { // no browser: only prove the route carries traffic to Google
			inst, err := newInstance(route, free)
			if err != nil {
				return Result{Status: "error", Err: err.Error()}
			}
			defer inst.Close()
			ms, err := reach(runCtx, inst, timeout)
			if err != nil {
				return Result{Status: "error", Err: err.Error()}
			}
			return Result{Status: "reachable", Ms: ms}
		}
		inst, port, err := startSocks(route, free)
		if err != nil {
			return Result{Status: "error", Err: err.Error()}
		}
		defer inst.Close()
		ms, err := reach(runCtx, inst, timeout)
		if err != nil {
			return Result{Status: "error", Err: err.Error()}
		}
		select {
		case bsem <- struct{}{}:
			defer func() { <-bsem }()
		case <-runCtx.Done():
			return Result{Status: "error", Err: "canceled"}
		}
		reg, err := br.region(runCtx, port, cookies)
		if err != nil {
			return Result{Status: "error", Ms: ms, Err: shorten(err.Error())}
		}
		if reg.Studio == "signin" || reg.Gemini == "signin" {
			signedOut.Store(true)
			stop()
		}
		return Result{Status: verdictOf(rq.Need, reg), Ms: ms, Studio: reg.Studio, Gemini: reg.Gemini}
	}

	if !simple {
		set(func() { st.Msg = "در حال تست کانفیگ پایه…" })
		baseRes := check(base, nil, make(chan struct{}, 1))
		if signedOut.Load() {
			authSet("failed", errSignedOut.Error())
			return "failed", errSignedOut.Error()
		}
		set(func() { st.Base = &baseRes })
		if baseRes.Status == "error" {
			return "failed", "کانفیگ پایه به گوگل وصل نمی‌شود (" + baseRes.Err + "). اول مطمئن شو در کلاینت خودت کار می‌کند."
		}
	}

	text := rq.Pasted
	// A single http(s) link pasted as the config text is a subscription URL: download it.
	if u := strings.TrimSpace(text); (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) && !strings.ContainsAny(u, " \t\r\n") {
		rq.Source, rq.Custom, text = "custom", u, ""
	}
	if strings.TrimSpace(text) == "" {
		urls, err := sourceURLs(rq.Source, rq.Custom)
		if err != nil {
			return "failed", err.Error()
		}
		set(func() { st.Msg = "در حال دانلود لیست کانفیگ‌ها…" })
		var via *Base // advanced: the base chain is the last-resort route to the list (GitHub blocked); simple has none
		if !simple {
			via = &base
		}
		if text, err = fetchList(runCtx, via, urls); err != nil {
			return "failed", err.Error()
		}
	}
	type item struct {
		name, link string
		out        obj
	}
	var items []item
	skipped := 0
	for _, l := range extractLinks(text) {
		if len(items) == rq.Limit {
			break
		}
		if name, out, err := parseLink(l); err == nil {
			items = append(items, item{name, l, out})
		} else {
			skipped++
		}
	}
	if len(items) == 0 {
		return "failed", "هیچ لینک قابل‌استفاده‌ای (vless / vmess / trojan / ss) پیدا نشد."
	}
	set(func() {
		st.Total, st.Skipped = len(items), skipped
		st.Msg = "در حال اسکن…"
		if rq.Mode == "region" {
			st.Msg = fmt.Sprintf("در حال اسکن (بررسی ریجن با مرورگر، %d همزمان)…", rq.BrowserConc)
		}
	})

	sem := make(chan struct{}, rq.Concurrency)
	bsem := make(chan struct{}, rq.BrowserConc)
	var wg sync.WaitGroup
	for _, it := range items {
		sem <- struct{}{}
		if runCtx.Err() != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var r Result
			if simple { // each config is its own route, so the anti-DPI profile applies to it directly
				own, stopOwn, perr := prepareBase(Base{Outs: []obj{tagged(it.out, "base")}, Exit: "base"}, rq.Profile)
				if perr != nil {
					r = Result{Status: "error", Err: shorten(perr.Error())}
				} else {
					r = check(own, nil, bsem)
					stopOwn()
				}
			} else {
				r = check(base, it.out, bsem)
			}
			r.Name, r.Link = it.name, it.link
			if runCtx.Err() == nil {
				set(func() { st.Results = append(st.Results, r); st.Done++ })
			}
		}()
	}
	wg.Wait()
	if signedOut.Load() {
		authSet("failed", errSignedOut.Error())
		return "failed", errSignedOut.Error()
	}
	if ctx.Err() != nil {
		return "stopped", ""
	}
	return "done", ""
}
