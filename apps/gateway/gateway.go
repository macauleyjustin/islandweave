package gateway

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/islandweave/islandweave/core/objects"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/hosting"
)

// Server is the local HTTP gateway for browsing IslandWeave static websites.
// It is strictly isolated from the privileged islandd Control API.
type Server struct {
	store     *storage.Store
	objStore  *objects.Store
	fetchSite func(ctx context.Context, nameOrID string) error

	ln      net.Listener
	httpSrv *http.Server
}

// New creates a local browser gateway server.
func New(store *storage.Store, objStore *objects.Store, fetchSite func(ctx context.Context, nameOrID string) error) *Server {
	return &Server{
		store:     store,
		objStore:  objStore,
		fetchSite: fetchSite,
	}
}

// Start binds and serves the gateway on listenAddr (e.g. "127.0.0.1:47475").
func (s *Server) Start(listenAddr string) error {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	s.ln = ln

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/site/", s.handleSite)

	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = s.httpSrv.Serve(ln)
	}()
	return nil
}

// Addr returns the bound gateway address.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Stop shuts down the gateway server.
func (s *Server) Stop() {
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.httpSrv.Shutdown(ctx)
		cancel()
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	svcs, err := s.store.ListServices()
	if err != nil {
		http.Error(w, "failed to list services", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	fmt.Fprint(w, "<!doctype html><html><head><title>IslandWeave Gateway</title><style>body{font-family:system-ui,sans-serif;max-width:720px;margin:2rem auto;padding:0 1rem}li{margin:.5rem 0}</style></head><body><h1>IslandWeave Local Gateway</h1><ul>")
	for _, svc := range svcs {
		badge := "cached-replica"
		if svc.IsLocal {
			badge = "local-host"
		}
		fmt.Fprintf(w, `<li><a href="/site/%s/">%s</a> (<code>%s</code>) [%s, v%d]</li>`,
			html.EscapeString(svc.Name), html.EscapeString(svc.Name), html.EscapeString(svc.ServiceID), badge, svc.SiteVersion)
	}
	fmt.Fprint(w, "</ul></body></html>")
}

func (s *Server) handleSite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/site/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	nameOrID := parts[0]
	relPath := "index.html"
	if len(parts) == 2 && parts[1] != "" {
		relPath = parts[1]
	}

	var fetchErr error
	svc, err := s.store.GetServiceByNameOrID(nameOrID)
	if err != nil || !s.objStore.HasChunk(svc.ManifestHash) {
		if s.fetchSite != nil {
			fetchErr = s.fetchSite(r.Context(), nameOrID)
			if fetchErr == nil {
				svc, err = s.store.GetServiceByNameOrID(nameOrID)
			}
		}
	}
	if err != nil {
		msg := "IslandWeave service not found"
		if fetchErr != nil {
			msg += ": " + fetchErr.Error()
		}
		http.Error(w, msg, http.StatusNotFound)
		return
	}

	manifest, err := hosting.LoadSiteManifest(s.objStore, svc.ManifestHash)
	if err != nil {
		http.Error(w, "site manifest unavailable locally", http.StatusBadGateway)
		return
	}
	source := "cached-replica"
	if svc.IsLocal {
		source = "local"
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-IslandWeave-Service-ID", svc.ServiceID)
	w.Header().Set("X-IslandWeave-Publisher", svc.PublisherID)
	w.Header().Set("X-IslandWeave-Source", source)
	// Strictly isolate hosted websites from privileged localhost APIs.
	w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' data:; connect-src 'self'; object-src 'none'")

	entry, data, err := hosting.ReadSiteFile(s.objStore, manifest, relPath)
	if err != nil {
		if os.IsNotExist(err) && relPath == "index.html" {
			// Serve an HTML file index for shared file libraries or sites without index.html.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>%s (%s)</title><style>body{font-family:system-ui,sans-serif;max-width:780px;margin:2rem auto;padding:0 1rem;background:#0f172a;color:#e2e8f0}a{color:#38bdf8}li{margin:.4rem 0}</style></head><body><h1>%s</h1><p>Type: <code>%s</code> | Publisher: <code>%s</code> | Version: v%d</p><ul>",
				html.EscapeString(svc.Name), html.EscapeString(svc.Type),
				html.EscapeString(svc.Name), html.EscapeString(svc.Type), html.EscapeString(svc.PublisherID), svc.SiteVersion)
			for _, f := range manifest.Files {
				fmt.Fprintf(w, `<li><a href="/site/%s/%s">%s</a> (%d bytes, %s)</li>`,
					html.EscapeString(svc.Name), html.EscapeString(f.Path), html.EscapeString(f.Path), f.Size, html.EscapeString(f.ContentType))
			}
			fmt.Fprint(w, "</ul></body></html>")
			return
		}
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to load site resource", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", entry.ContentType)
	_, _ = w.Write(data)
}
