// Web mode: same UI as the desktop app, served over HTTP (servers, Termux, anywhere without a webview).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"gemini-config-checker/internal/buildinfo"
	"gemini-config-checker/internal/scan"
	"gemini-config-checker/internal/ui"
)

func guard(loopback bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block DNS-rebinding (foreign Host) and cross-site form posts (non-JSON POST).
		h, _, _ := net.SplitHostPort(r.Host)
		if loopback && h != "localhost" && h != "127.0.0.1" && h != "::1" && h != "[::1]" {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if r.Method == "POST" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "json only", http.StatusUnsupportedMediaType)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address (use 0.0.0.0:8787 on a server; there is no auth, so keep it private)")
	noOpen := flag.Bool("no-open", false, "don't open the browser")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(ui.FS)) // index.html + fonts; the /api/ routes below are more specific
	mux.HandleFunc("POST /api/chain", func(w http.ResponseWriter, r *http.Request) {
		var rq scan.ChainRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&rq); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		s, err := scan.ChainJSON(rq)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Write([]byte(s))
	})
	mux.HandleFunc("GET /api/profile/default", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scan.DefaultProfile())
	})
	mux.HandleFunc("POST /api/profile/extract", func(w http.ResponseWriter, r *http.Request) {
		var rq struct{ Base string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&rq); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		p, err := scan.ExtractProfile(rq.Base)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(buildinfo.Version)) })
	mux.HandleFunc("POST /api/clear", func(w http.ResponseWriter, r *http.Request) { scan.Clear() })
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		if err := scan.StartLogin(); err != nil {
			http.Error(w, err.Error(), 409)
		}
	})
	mux.HandleFunc("POST /api/login/cancel", func(w http.ResponseWriter, r *http.Request) { scan.CancelLogin() })
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) { scan.Logout() })
	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		var rq scan.Request
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&rq); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := scan.Start(rq); err != nil {
			http.Error(w, err.Error(), 409)
		}
	})
	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) { scan.Stop() })
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scan.Snapshot())
	})

	scan.RestoreAtStartup()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(*addr)
	loopback := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	url := fmt.Sprintf("http://%s:%s", host, port)
	fmt.Println("Gemini Config Checker →", url)
	if !*noOpen {
		openBrowser(url)
	}
	log.Fatal(http.Serve(ln, guard(loopback, mux)))
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	_ = c.Start() // headless/Termux: just use the printed URL
}
