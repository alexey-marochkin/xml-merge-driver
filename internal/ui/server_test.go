package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xmlmerge/internal/interactive"
)

func TestStandaloneEditorRouteAndSave(t *testing.T) {
	session, err := interactive.New(interactive.Options{Mode: "rules", Rules: filepath.Join(t.TempDir(), "rules.xml")})
	if err != nil {
		t.Fatal(err)
	}
	s, err := StartWithSettings(session, filepath.Join(t.TempDir(), "settings.xml"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	res, err := http.Get(s.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(html), "catalog.js") {
		t.Fatal("wrong standalone entry page", res.StatusCode)
	}
	st := session.State()
	data, _ := json.Marshal(map[string]any{"Database": st.Database, "Revision": st.Revision})
	req, _ := http.NewRequest("POST", s.URL+"/api/database", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+s.Token)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("catalog save route failed", res.StatusCode)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	path := func(s string) string { return filepath.Join(dir, s) }
	for _, n := range []string{"base", "local", "remote"} {
		if e := os.WriteFile(path(n), []byte(`<r><i/><i/></r>`), 0600); e != nil {
			t.Fatal(e)
		}
	}
	s, e := interactive.New(interactive.Options{Mode: "merge", Rules: path("rules"), Base: path("base"), Local: path("local"), Remote: path("remote"), Output: path("result")})
	if e != nil {
		t.Fatal(e)
	}
	server, e := StartWithSettings(s, filepath.Join(t.TempDir(), "settings.xml"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { server.Close() })
	return server
}

func TestSettingsAPIAndInitialTheme(t *testing.T) {
	s := newTestServer(t)
	for _, tc := range []struct {
		token string
		want  int
	}{{"", 401}, {s.Token, 200}} {
		req, _ := http.NewRequest("POST", s.URL+"/api/settings", strings.NewReader(`{"Theme":"dark","SidebarWidth":496}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Fatalf("settings status: %d, want %d", res.StatusCode, tc.want)
		}
	}
	p, err := (&preferenceStore{path: s.preferences.path}).load()
	if err != nil || p.Theme != "dark" || p.SidebarWidth != 496 {
		t.Fatalf("settings not persisted: %+v, %v", p, err)
	}
	for _, path := range []string{"/", "/catalog.html"} {
		res, err := http.Get(s.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || !strings.Contains(string(body), `data-theme="dark"`) {
			t.Fatalf("initial theme missing on %s: %v", path, err)
		}
	}
}

func TestAutomaticPortsAndIsolation(t *testing.T) {
	a, b := newTestServer(t), newTestServer(t)
	if a.URL == b.URL || a.Token == b.Token {
		t.Fatal("instances share port or secret")
	}
	client := &http.Client{Timeout: time.Second}
	for _, tc := range []struct {
		token, origin string
		want          int
	}{{"", "", 401}, {b.Token, "", 401}, {a.Token, "https://other.example", 403}, {a.Token, a.URL, 200}} {
		r, _ := http.NewRequest("GET", a.URL+"/api/state", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Fatal(res.StatusCode, tc.want)
		}
	}
	addr := a.listener.Addr().String()
	a.Close()
	conn, e := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if e == nil {
		conn.Close()
		t.Fatal("port still listening after close")
	}
	res, e := client.Get(b.URL)
	if e != nil {
		t.Fatal("closing one instance stopped the other", e)
	}
	res.Body.Close()
}

func TestHTTPGateAndCancellation(t *testing.T) {
	s := newTestServer(t)
	request := func(path string, body any) *http.Response {
		t.Helper()
		data, _ := json.Marshal(body)
		r, _ := http.NewRequest("POST", s.URL+path, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+s.Token)
		res, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	res := request("/api/compare", map[string]string{})
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("comparison gate bypassed")
	}
	res = request("/api/save", map[string]string{"Choice": "local"})
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("save gate bypassed")
	}
	res = request("/api/cancel", map[string]string{})
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	select {
	case <-s.Done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not signal window")
	}
	if s.Close() != interactive.Cancelled {
		t.Fatal("wrong cancellation exit code")
	}
	if _, e := os.Stat(s.Session.Options.Output); !os.IsNotExist(e) {
		t.Fatal("cancel wrote output")
	}
}
