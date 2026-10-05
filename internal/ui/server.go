package ui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"xmlmerge/internal/interactive"
	"xmlmerge/internal/rules"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	Session     *interactive.Session
	Token       string
	URL         string
	Done        chan struct{}
	listener    net.Listener
	http        *http.Server
	once        sync.Once
	preferences *preferenceStore
}

func Start(session *interactive.Session) (*Server, error) {
	path, err := preferencesPath()
	if err != nil {
		return nil, err
	}
	return StartWithSettings(session, path)
}

// StartWithSettings lets previews and tests keep settings separate from the user.
func StartWithSettings(session *interactive.Session, settingsPath string) (*Server, error) {
	preferences := &preferenceStore{path: settingsPath}
	if _, err := preferences.load(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		if _, err = preferences.update(PreferencesPatch{}); err != nil {
			return nil, err
		}
	}
	// Binding port 0 lets Windows allocate and reserve a free port atomically.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		listener.Close()
		return nil, err
	}
	s := &Server{Session: session, listener: listener, Token: hex.EncodeToString(secret), URL: "http://" + listener.Addr().String(), Done: make(chan struct{})}
	s.preferences = preferences
	s.http = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		err := s.http.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			s.finish()
		}
	}()
	return s, nil
}

func (s *Server) finish() { s.once.Do(func() { close(s.Done) }) }
func (s *Server) Close() int {
	// Close the session first: an active save finishes under the session lock,
	// and no request can begin another write after cancellation.
	code := s.Session.Close()
	_ = s.http.Close()
	s.finish()
	return code
}
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

func (s *Server) handler() http.Handler {
	static, _ := fs.Sub(assets, "assets")
	files := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if r.Host != s.listener.Addr().String() {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			if r.URL.Path == "/" && s.Session.Options.StandaloneRules() {
				r.URL.Path = "/catalog.html"
			}
			if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/catalog.html" {
				name := "index.html"
				if r.URL.Path == "/catalog.html" {
					name = "catalog.html"
				}
				p, err := s.preferences.load()
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				data, err := assets.ReadFile("assets/" + name)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				// Set the theme before the first paint, avoiding a bright flash.
				_, _ = io.WriteString(w, strings.Replace(string(data), "<html lang=\"ru\">", fmt.Sprintf("<html lang=\"ru\" data-theme=\"%s\">", p.Theme), 1))
				return
			}
			files.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != s.URL {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/api/settings":
				p, err := s.preferences.load()
				s.reply(w, p, err)
			case "/api/state":
				data, err := s.Session.MarshaledState()
				if err != nil {
					s.err(w, err)
					return
				}
				w.Write(data)
			case "/api/samples":
				data, err := s.Session.Samples(r.URL.Query().Get("path"))
				s.reply(w, data, err)
			case "/api/tree":
				data, err := s.Session.Tree(r.URL.Query().Get("parent"))
				s.reply(w, data, err)
			case "/api/navigation":
				data, err := s.Session.Navigation()
				s.reply(w, data, err)
			case "/api/node":
				data, err := s.Session.Node(r.URL.Query().Get("id"))
				s.reply(w, data, err)
			default:
				http.NotFound(w, r)
			}
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit := int64(128 << 10)
		if r.URL.Path == "/api/database" || r.URL.Path == "/api/resolve" {
			limit = 16 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		switch r.URL.Path {
		case "/api/undo":
			data, err := s.Session.Undo()
			s.reply(w, data, err)
		case "/api/resolve":
			var req interactive.NodeDecision
			if err := decodeJSON(r, &req); err != nil {
				s.err(w, err)
				return
			}
			data, err := s.Session.Resolve(req)
			s.reply(w, data, err)
		case "/api/settings":
			var patch PreferencesPatch
			if err := decodeJSON(r, &patch); err != nil {
				s.err(w, err)
				return
			}
			p, err := s.preferences.update(patch)
			s.reply(w, p, err)
		case "/api/database":
			var req struct {
				Database *rules.Database
				Revision string
			}
			if err := decodeJSON(r, &req); err != nil {
				s.err(w, err)
				return
			}
			data, err := s.Session.SaveDatabase(req.Database, req.Revision)
			s.reply(w, data, err)
		case "/api/preview":
			var rule rules.Rule
			if err := decodeJSON(r, &rule); err != nil {
				s.err(w, err)
				return
			}
			data, err := s.Session.Preview(rule)
			s.reply(w, data, err)
		case "/api/rule":
			var req struct {
				Rule     rules.Rule
				Revision string
			}
			if err := decodeJSON(r, &req); err != nil {
				s.err(w, err)
				return
			}
			data, err := s.Session.SaveRule(req.Rule, req.Revision)
			s.reply(w, data, err)
		case "/api/reload":
			data, err := s.Session.Reload()
			s.reply(w, data, err)
		case "/api/compare":
			data, err := s.Session.Compare()
			s.reply(w, data, err)
		case "/api/save":
			var req struct{ Choice string }
			if err := decodeJSON(r, &req); err != nil {
				s.err(w, err)
				return
			}
			if err := s.Session.Save(req.Choice); err != nil {
				s.err(w, err)
				return
			}
			s.reply(w, map[string]bool{"Saved": true}, nil)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			s.finish()
		case "/api/cancel":
			s.Session.Close()
			s.reply(w, map[string]bool{"Cancelled": true}, nil)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			s.finish()
		default:
			http.NotFound(w, r)
		}
	})
}

func decodeJSON(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected a single JSON value")
	}
	return nil
}
func (s *Server) err(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"Error": err.Error()})
}
func (s *Server) reply(w http.ResponseWriter, value any, err error) {
	if err != nil {
		s.err(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
