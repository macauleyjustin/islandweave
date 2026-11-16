package main

import (
	"embed"
	"encoding/json"
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

// ResolvedURI describes how an island:// URI maps to either a native view or an isolated gateway URL.
type ResolvedURI struct {
	Raw        string `json:"raw"`
	Kind       string `json:"kind"` // "native" | "site" | "https" | "invalid"
	View       string `json:"view,omitempty"`
	Service    string `json:"service,omitempty"`
	Path       string `json:"path,omitempty"`
	GatewayURL string `json:"gateway_url,omitempty"`
	Source     string `json:"source,omitempty"`
	ServiceID  string `json:"service_id,omitempty"`
	Publisher  string `json:"publisher,omitempty"`
}

// ResolveIslandURI parses an island:// address according to BROWSER_SPECIFICATION.md.
func ResolveIslandURI(raw, gatewayAddr string) ResolvedURI {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return ResolvedURI{Raw: raw, Kind: "https"}
	}
	if !strings.HasPrefix(raw, "island://") {
		raw = "island://" + raw
	}
	trimmed := strings.TrimPrefix(raw, "island://")
	parts := strings.SplitN(trimmed, "/", 2)
	auth := strings.TrimSpace(parts[0])
	path := ""
	if len(parts) == 2 {
		path = parts[1]
	}
	switch auth {
	case "", "home", "mail", "news", "files", "peers", "services", "downloads":
		if auth == "" {
			auth = "home"
		}
		return ResolvedURI{Raw: raw, Kind: "native", View: auth}
	default:
		gwURL := fmt.Sprintf("http://%s/site/%s/%s", gatewayAddr, auth, path)
		return ResolvedURI{Raw: raw, Kind: "site", Service: auth, Path: path, GatewayURL: gwURL}
	}
}

// NewHandler creates the IslandWeave Navigator HTTP handler.
func NewHandler(dataDir string) (http.Handler, error) {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/nav/resolve", func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Query().Get("uri")
		gwBytes, err := os.ReadFile(filepath.Join(dataDir, "gateway.addr"))
		gwAddr := "127.0.0.1:47475"
		if err == nil {
			gwAddr = strings.TrimSpace(string(gwBytes))
		}
		res := ResolveIslandURI(uri, gwAddr)
		if res.Kind == "site" {
			// Trigger fetch/verify metadata from islandd if running
			if svcsRaw, code, err := callDaemon(dataDir, http.MethodGet, "/v1/services", nil); err == nil && code == 200 {
				var svcs []struct {
					ServiceID   string `json:"service_id"`
					Name        string `json:"name"`
					PublisherID string `json:"publisher_id"`
					IsLocal     bool   `json:"is_local"`
				}
				if json.Unmarshal(svcsRaw, &svcs) == nil {
					for _, s := range svcs {
						if s.Name == res.Service || s.ServiceID == res.Service {
							res.ServiceID = s.ServiceID
							res.Publisher = s.PublisherID
							if s.IsLocal {
								res.Source = "Live Local Host"
							} else {
								res.Source = "Cached Offline Replica"
							}
							break
						}
					}
				}
			}
			if res.Source == "" {
				// Attempt on-demand peer fetch
				body, _ := json.Marshal(map[string]string{"name_or_id": res.Service})
				_, code, err := callDaemon(dataDir, http.MethodPost, "/v1/services/fetch", body)
				if err == nil && code == 200 {
					res.Source = "Cached Offline Replica"
				} else {
					res.Source = "Unavailable / Checking Gateway"
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})
	mux.HandleFunc("/nav/api/", func(w http.ResponseWriter, r *http.Request) {
		targetPath := strings.TrimPrefix(r.URL.Path, "/nav/api")
		if r.URL.RawQuery != "" {
			targetPath += "?" + r.URL.RawQuery
		}
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		out, code, err := callDaemon(dataDir, r.Method, targetPath, body)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(out)
	})
	return mux, nil
}

func callDaemon(dataDir, method, path string, body []byte) ([]byte, int, error) {
	addrBytes, err := os.ReadFile(filepath.Join(dataDir, "control.addr"))
	if err != nil {
		return nil, 0, err
	}
	tokBytes, err := os.ReadFile(filepath.Join(dataDir, "control.token"))
	if err != nil {
		return nil, 0, err
	}
	var r io.Reader
	if len(body) > 0 {
		r = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, "http://"+strings.TrimSpace(string(addrBytes))+path, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokBytes)))
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return out, resp.StatusCode, err
}

func main() {
	dataDir := flag.String("data-dir", defaultDataDir(), "IslandWeave data directory")
	listen := flag.String("listen", "127.0.0.1:47485", "Navigator listen address")
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
	fmt.Printf("IslandWeave Navigator running at %s\n", url)
	if !*noOpen {
		launch.AppWindow(url)
	}
	log.Fatal(http.Serve(ln, h))
}
