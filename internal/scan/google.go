package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Local Google sign-in. Google only judges the region for a signed-in account, so the real check opens AI Studio
// and Gemini in a signed-in browser. The user signs in inside their own Chromium browser (Chrome/Edge/Brave) running
// with an isolated profile, exactly like a normal login, so the app never sees a password. The profile stays on this
// machine; session cookies are kept in memory only.

// Cookie is a raw CDP cookie object, passed through untouched.
type Cookie = map[string]any

var (
	sessMu  sync.Mutex
	session []Cookie
)

func setSession(c []Cookie) { sessMu.Lock(); session = c; sessMu.Unlock() }

func LoggedIn() bool { sessMu.Lock(); defer sessMu.Unlock(); return hasAuth(session) }

func hasAuth(c []Cookie) bool {
	var sapisid, sid bool
	for _, k := range c {
		n, _ := k["name"].(string)
		sapisid = sapisid || n == "SAPISID" || n == "__Secure-3PAPISID"
		sid = sid || n == "SID" || n == "__Secure-3PSID"
	}
	return sapisid && sid
}

// Sign-in state shown in the UI: none | waiting (browser window open) | ok | failed.
var (
	authMu     sync.Mutex
	authState  = "none"
	authMsg    string
	authCancel context.CancelFunc
)

func authSet(state, msg string) { authMu.Lock(); authState, authMsg = state, msg; authMu.Unlock() }
func authSnapshot() (string, string) {
	authMu.Lock()
	defer authMu.Unlock()
	return authState, authMsg
}

// StartLogin opens the browser window in the background; poll Snapshot().Auth for the outcome.
func StartLogin() error {
	mu.Lock()
	scanning := st.State == "running"
	mu.Unlock()
	if scanning {
		return errors.New("یک اسکن در حال اجراست؛ بعد از پایان یا توقف آن وارد شو")
	}
	authMu.Lock()
	defer authMu.Unlock()
	if authState == "waiting" {
		return errors.New("پنجره‌ی ورود از قبل باز است")
	}
	var ctx context.Context
	ctx, authCancel = context.WithCancel(context.Background())
	authState, authMsg = "waiting", "در حال باز کردن مرورگر…"
	go func() {
		if err := Login(ctx); err != nil {
			authSet("failed", err.Error())
		} else {
			authSet("ok", "")
		}
	}()
	return nil
}

func CancelLogin() {
	authMu.Lock()
	if authCancel != nil {
		authCancel()
	}
	authMu.Unlock()
}

// RestoreAtStartup quietly reuses a previous sign-in, if the isolated profile still has one. The state is
// "checking" meanwhile, so the UI keeps polling instead of showing a stale "not signed in".
func RestoreAtStartup() {
	if _, err := os.Stat(profileDir()); err != nil {
		return
	}
	authSet("checking", "")
	go func() {
		ok := RestoreSession() == nil
		authMu.Lock()
		defer authMu.Unlock()
		if authState != "checking" { // the user started a sign-in meanwhile; that outcome wins
			return
		}
		if ok {
			authState = "ok"
		} else {
			authState = "none"
		}
	}()
}

// Logout forgets the session and deletes the isolated browser profile, so nothing of the sign-in stays on disk.
// It never touches the user's own browser profile.
func Logout() error {
	setSession(nil)
	authSet("none", "")
	return os.RemoveAll(profileDir())
}

// profileMu: only one browser may use the isolated profile at a time (restore, sign-in, scan).
var profileMu sync.Mutex

// Every browser we start is recorded here (pid is informational; the port is what we use to talk to it), so a
// browser left over from a crash, a killed app or an interrupted build step can be found and closed gracefully.
func recordPath() string { return filepath.Join(filepath.Dir(profileDir()), "browser.json") }

func writeRecord(pid, port int) {
	b, _ := json.Marshal(map[string]int{"pid": pid, "port": port})
	os.WriteFile(recordPath(), b, 0o600)
}

// closeStale asks a recorded leftover browser to close itself (graceful, so cookies are flushed to disk).
// A record of a browser that is already gone is harmless: nothing answers on its port.
func closeStale() {
	data, err := os.ReadFile(recordPath())
	if err != nil {
		return
	}
	var r struct{ Port int }
	if json.Unmarshal(data, &r) != nil || r.Port == 0 {
		return
	}
	base := fmt.Sprintf("http://127.0.0.1:%d/json/version", r.Port)
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(base)
	if err != nil {
		return
	}
	var v struct{ WebSocketDebuggerUrl string }
	err = json.NewDecoder(resp.Body).Decode(&v)
	resp.Body.Close()
	if err != nil || v.WebSocketDebuggerUrl == "" {
		return
	}
	conn, _, err := websocket.DefaultDialer.Dial(v.WebSocketDebuggerUrl, nil)
	if err != nil {
		return
	}
	conn.WriteJSON(map[string]any{"id": 1, "method": "Browser.close"})
	conn.Close()
	for i := 0; i < 80; i++ { // wait until it is really gone
		time.Sleep(250 * time.Millisecond)
		if _, err := c.Get(base); err != nil {
			time.Sleep(time.Second) // let it release the profile lock
			return
		}
	}
}

// browser is one Chromium process driven over its local debugging port.
type browser struct {
	cmd  *exec.Cmd
	port int
	done chan struct{} // closed when the process exits
	mu   sync.Mutex    // serialises browser-level calls
	conn *websocket.Conn
	id   int
}

func launch(headless bool, url string, extra ...string) (*browser, error) {
	exe, err := findBrowser()
	if err != nil {
		return nil, err
	}
	if !headless && runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, errors.New("این سیستم صفحه‌ی گرافیکی ندارد (DISPLAY خالی است)؛ ورود به Google را روی یک دسکتاپ انجام بده")
	}
	closeStale()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	args := []string{"--user-data-dir=" + profileDir(), fmt.Sprintf("--remote-debugging-port=%d", port), "--remote-allow-origins=*",
		"--no-first-run", "--no-default-browser-check", "--disable-sync", "--mute-audio"}
	if headless {
		args = append(args, "--headless=new")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		args = append(args, "--no-sandbox") // Chromium refuses to start as root otherwise (containers, servers)
	}
	args = append(args, extra...)
	cmd := exec.Command(exe, append(args, url)...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	writeRecord(cmd.Process.Pid, port)
	b := &browser{cmd: cmd, port: port, done: make(chan struct{})}
	go func() { cmd.Wait(); close(b.done) }()
	for i := 0; i < 80; i++ { // wait for the debugging endpoint
		time.Sleep(250 * time.Millisecond)
		select {
		case <-b.done: // a normal exit this early means the request was handed to a browser already using the profile
			return nil, errors.New("مرورگر بلافاصله بسته شد؛ احتمالاً مرورگر دیگری همین پروفایل را باز کرده است. پنجره‌های Chrome مربوط به برنامه را ببند و دوباره امتحان کن")
		default:
		}
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
		if err != nil {
			continue
		}
		var v struct{ WebSocketDebuggerUrl string }
		err = json.NewDecoder(resp.Body).Decode(&v)
		resp.Body.Close()
		if err != nil || v.WebSocketDebuggerUrl == "" {
			continue
		}
		if b.conn, _, err = websocket.DefaultDialer.Dial(v.WebSocketDebuggerUrl, nil); err == nil {
			return b, nil
		}
	}
	b.kill()
	return nil, errors.New("مرورگر پاسخ نداد (ممکن است پنجره‌ی ورود هنوز باز باشد)")
}

func (b *browser) call(method string, params any) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.id++
	if err := b.conn.WriteJSON(map[string]any{"id": b.id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		var m struct {
			ID     int
			Result json.RawMessage
			Error  *struct{ Message string }
		}
		if err := b.conn.ReadJSON(&m); err != nil {
			return nil, err
		}
		if m.ID == b.id {
			if m.Error != nil {
				return nil, errors.New(m.Error.Message)
			}
			return m.Result, nil
		}
	}
}

func (b *browser) cookies() ([]Cookie, error) {
	r, err := b.call("Storage.getCookies", map[string]any{})
	if err != nil {
		return nil, err
	}
	var v struct{ Cookies []Cookie }
	err = json.Unmarshal(r, &v)
	return v.Cookies, err
}

func (b *browser) kill() {
	if b.conn != nil {
		b.call("Browser.close", map[string]any{})
		b.conn.Close()
	}
	select {
	case <-b.done:
	case <-time.After(30 * time.Second): // Chrome flushes cookies to disk on a graceful exit; killing early loses the sign-in
		b.cmd.Process.Kill()
		<-b.done
	}
	os.Remove(recordPath())
}

// Login opens the browser for the user to sign in and returns once a Google session exists (or ctx ends).
func Login(ctx context.Context) error {
	profileMu.Lock()
	defer profileMu.Unlock()
	if err := os.MkdirAll(profileDir(), 0o700); err != nil {
		return err
	}
	b, err := launch(false, "https://accounts.google.com/ServiceLogin?continue=https%3A%2F%2Faistudio.google.com%2F")
	if err != nil {
		return err
	}
	defer b.kill()
	authSet("waiting", "پنجره‌ی مرورگر باز شد؛ با حساب Google خودت وارد شو.")
	deadline := time.After(10 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.done:
			return errors.New("پنجره‌ی مرورگر بسته شد، پیش از آنکه ورود کامل شود")
		case <-deadline:
			return errors.New("زمان ورود تمام شد")
		case <-time.After(2 * time.Second):
		}
		c, err := b.cookies()
		if err != nil || !hasAuth(c) {
			continue
		}
		time.Sleep(2 * time.Second) // let the remaining cookies settle
		if c2, err := b.cookies(); err == nil {
			c = c2
		}
		setSession(c)
		return nil
	}
}

// RestoreSession silently reloads a previous sign-in from the isolated profile (headless, no window).
func RestoreSession() error {
	profileMu.Lock()
	defer profileMu.Unlock()
	if _, err := os.Stat(profileDir()); err != nil {
		return errors.New("هنوز وارد نشده‌ای")
	}
	b, err := launch(true, "about:blank")
	if err != nil {
		return err
	}
	defer b.kill()
	c, err := b.cookies()
	if err != nil {
		return err
	}
	if !hasAuth(c) {
		return errors.New("ورود قبلی منقضی شده؛ دوباره وارد شو")
	}
	setSession(c)
	return nil
}
