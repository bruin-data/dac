package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"github.com/bruin-data/dac/pkg/theme"
)

// Config holds server configuration.
type Config struct {
	Host          string
	Port          int
	DashboardDir  string
	TemplateName  string
	ConfigFile    string
	Environment   string
	AdminPassword string
	Frontend      fs.FS // embedded frontend files, nil for dev mode
}

// Server is the dac HTTP server.
type Server struct {
	config  Config
	paths   dashboard.ProjectPaths
	backend query.Backend
	themes  *theme.Registry
	loader  *dashboardLoader
	watcher *Watcher
	mux     *http.ServeMux
}

type dashboardLoader struct {
	dir     string
	backend query.Backend
}

func (l *dashboardLoader) Load() ([]*dashboard.Dashboard, error) {
	return dashboard.LoadDir(l.dir, l.tsxOpts()...)
}

// LoadMeta loads dashboards without executing queries — used for listing
// where only metadata (name, description, filter/widget counts) is needed.
func (l *dashboardLoader) LoadMeta() ([]*dashboard.Dashboard, error) {
	return dashboard.LoadDir(l.dir)
}

// LoadOne loads a single dashboard by name. Much faster than Load() when only
// one dashboard is needed, because it skips all other files in the directory.
func (l *dashboardLoader) LoadOne(name string) (*dashboard.Dashboard, error) {
	return dashboard.LoadOneByName(l.dir, name, l.tsxOpts()...)
}

// LoadPath loads a single dashboard file by path, handling both YAML and TSX.
func (l *dashboardLoader) LoadPath(path string) (*dashboard.Dashboard, error) {
	if dashboard.IsTSXFile(filepath.Base(path)) {
		return dashboard.LoadTSXFile(path, l.tsxOpts()...)
	}
	return dashboard.LoadFile(path)
}

func (l *dashboardLoader) tsxOpts() []dashboard.TSXOption {
	if l.backend == nil {
		return nil
	}
	return []dashboard.TSXOption{
		dashboard.WithQueryFunc(func(connection, sql string) (map[string]interface{}, error) {
			result, err := l.backend.Execute(context.Background(), connection, sql)
			if err != nil {
				return nil, err
			}
			cols := make([]map[string]interface{}, len(result.Columns))
			for i, c := range result.Columns {
				cols[i] = map[string]interface{}{"name": c.Name}
				if c.Type != "" {
					cols[i]["type"] = c.Type
				}
			}
			return map[string]interface{}{
				"columns": cols,
				"rows":    result.Rows,
			}, nil
		}),
	}
}

// New creates a new server instance.
func New(cfg Config) (*Server, error) {
	backend := &query.BruinCLIBackend{
		ConfigFile:  cfg.ConfigFile,
		Environment: cfg.Environment,
	}
	cachedBackend := query.NewCachedBackend(backend, 5*60*1e9) // 5 min default TTL

	themes := theme.NewRegistry()
	paths := dashboard.ResolveProjectPaths(cfg.DashboardDir)

	// If --template points to a .yml/.yaml file, load it as a user theme.
	templateName := cfg.TemplateName
	if strings.HasSuffix(templateName, ".yml") || strings.HasSuffix(templateName, ".yaml") {
		t, err := theme.LoadFile(templateName)
		if err != nil {
			return nil, fmt.Errorf("loading template file: %w", err)
		}
		themes.Add(t)
		templateName = t.Name
		cfg.TemplateName = templateName
	}

	if paths.ThemesDir != "" {
		if err := themes.LoadUserThemes(paths.ThemesDir); err != nil {
			log.Printf("Warning: could not load user themes: %v", err)
		}
	}

	s := &Server{
		config:  cfg,
		paths:   paths,
		backend: cachedBackend,
		themes:  themes,
		loader:  &dashboardLoader{dir: paths.RootDir, backend: cachedBackend},
		mux:     http.NewServeMux(),
	}

	s.setupRoutes()
	dashboards, err := s.loader.LoadMeta()
	if err != nil {
		return nil, err
	}
	if err := dashboard.ValidateAll(dashboards); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) setupRoutes() {
	// API routes.
	s.mux.HandleFunc("GET /api/v1/dashboards", s.handleListDashboards)
	s.mux.HandleFunc("GET /api/v1/dashboards/{name}", s.handleGetDashboard)
	s.mux.HandleFunc("GET /api/v1/dashboards/{name}/raw", s.handleGetDashboardRaw)
	s.mux.HandleFunc("POST /api/v1/dashboards/{name}/data", s.handleBatchQuery)
	s.mux.HandleFunc("POST /api/v1/dashboards/{name}/stream", s.handleStreamQuery)
	s.mux.HandleFunc("POST /api/v1/dashboards/{name}/widgets/{widgetId}/query", s.handleWidgetQuery)
	s.mux.HandleFunc("POST /api/v1/query", s.handleSingleQuery)
	s.mux.HandleFunc("GET /api/v1/themes", s.handleListThemes)
	s.mux.HandleFunc("GET /api/v1/themes/{name}", s.handleGetTheme)
	s.mux.HandleFunc("GET /api/v1/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/v1/events", s.handleSSE)

	// Admin routes — only registered when a password is configured.
	if s.config.AdminPassword != "" {
		s.mux.HandleFunc("POST /api/v1/admin/login", s.handleAdminLogin)
		s.mux.HandleFunc("GET /api/v1/admin/connections", s.requireAdmin(s.handleAdminListConnections))
		s.mux.HandleFunc("POST /api/v1/admin/connections", s.requireAdmin(s.handleAdminCreateConnection))
		s.mux.HandleFunc("PUT /api/v1/admin/connections/{type}/{name}", s.requireAdmin(s.handleAdminUpdateConnection))
		s.mux.HandleFunc("DELETE /api/v1/admin/connections/{type}/{name}", s.requireAdmin(s.handleAdminDeleteConnection))
		s.mux.HandleFunc("POST /api/v1/admin/connections/{type}/{name}/test", s.requireAdmin(s.handleAdminTestConnection))
	}

	// Frontend static files with SPA fallback for client-side routing.
	if s.config.Frontend != nil {
		s.mux.Handle("/", spaHandler(s.config.Frontend))
	}
}

// spaHandler serves static files from the embedded FS, falling back to index.html
// for any path that doesn't match a file (SPA client-side routing).
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly.
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if file exists.
		f, err := fsys.Open(path[1:]) // strip leading /
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for SPA routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// corsMiddleware lets the VS Code webview embed (a different origin) reach the server.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Cache-Control, Accept")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Start begins listening and serving.
// It tries the configured port first, then increments until it finds an available one.
func (s *Server) Start() error {
	// Start file watcher.
	watcher, err := NewWatcher(s.paths.DashboardDir, s.paths.SemanticDir, s.paths.ThemesDir)
	if err != nil {
		log.Printf("Warning: file watcher disabled: %v", err)
	} else {
		// Clear query cache on file changes so dynamic dashboards get fresh data.
		if cb, ok := s.backend.(*query.CachedBackend); ok {
			watcher.invalidators = append(watcher.invalidators, cb)
		}
		s.watcher = watcher
		go s.watcher.Run()
	}

	handler := corsMiddleware(s.mux)

	port := s.config.Port
	maxAttempts := 50
	for i := 0; i < maxAttempts; i++ {
		addr := fmt.Sprintf("%s:%d", s.config.Host, port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("Port %d in use, trying %d...", port, port+1)
			port++
			continue
		}
		log.Printf("dac server listening on http://%s", addr)
		return http.Serve(ln, handler)
	}

	return fmt.Errorf("no available port found in range %d-%d", s.config.Port, s.config.Port+maxAttempts-1)
}
