package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/islandweave/islandweave/platform/launch"
)

//go:embed ui/*
var uiFS embed.FS

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".islandweave"
	}
	return filepath.Join(home, ".islandweave")
}

// NewHandler builds the Desktop Control Center HTTP handler proxying /api/* to islandd.
func NewHandler(dataDir string) (http.Handler, error) {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		addrBytes, err := os.ReadFile(filepath.Join(dataDir, "control.addr"))
		if err != nil {
			http.Error(w, `{"error":"islandd not running"}`, http.StatusServiceUnavailable)
			return
		}
		tokBytes, err := os.ReadFile(filepath.Join(dataDir, "control.token"))
		if err != nil {
			http.Error(w, `{"error":"missing control token"}`, http.StatusServiceUnavailable)
			return
		}
		targetPath := strings.TrimPrefix(r.URL.Path, "/api")
		if r.URL.RawQuery != "" {
			targetPath += "?" + r.URL.RawQuery
		}
		targetURL := "http://" + strings.TrimSpace(string(addrBytes)) + targetPath
		req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokBytes)))
		if ct := r.Header.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})
	return mux, nil
}

func main() {
	dataDir := flag.String("data-dir", defaultDataDir(), "IslandWeave data directory")
	listen := flag.String("listen", "127.0.0.1:47480", "Desktop Control Center listen address")
	noOpen := flag.Bool("no-open", false, "Do not automatically open browser window")
	flag.Parse()

	h, err := NewHandler(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + ln.Addr().String()
	fmt.Printf("IslandWeave Desktop Control Center running at %s\n", url)
	if !*noOpen {
		launch.AppWindow(url)
	}
	log.Fatal(http.Serve(ln, h))
}
