package interactive

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

func TestStandaloneArguments(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	o, err := Parse(nil, io.Discard)
	if err != nil || !o.StandaloneRules() || o.Rules != filepath.Join(filepath.Dir(exe), "rules.xml") {
		t.Fatal(o, err)
	}
	for _, args := range [][]string{{"--rules", "папка с пробелами/правила.xml"}, {"rules", "--rules", "папка с пробелами/правила.xml"}} {
		o, err := Parse(args, io.Discard)
		if err != nil || !o.StandaloneRules() || !filepath.IsAbs(o.Rules) {
			t.Fatal(o, err)
		}
		roundtrip, err := Parse(o.Args(), io.Discard)
		if err != nil || roundtrip != o {
			t.Fatal(roundtrip, err)
		}
	}
	for _, args := range [][]string{{"--base", "base.xml"}, {"compare"}, {"merge"}, {"--rules", ""}, {"--output", "out.xml"}, {"--user-rules", "extra.xml"}} {
		if _, err := Parse(args, io.Discard); err == nil {
			t.Fatalf("accepted incomplete invocation %v", args)
		}
	}
}

func TestStandaloneCreateEditReopenAndConcurrentChange(t *testing.T) {
	o := Options{Mode: "rules", Rules: filepath.Join(t.TempDir(), "settings", "rules.xml")}
	s, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rules.Load(o.Rules); err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile(o.Rules)
	if err != nil {
		t.Fatal("missing file was not created", err)
	}
	st := s.State()
	if st.Phase != "catalog" || st.Database == nil || len(st.Database.Profiles) != 0 {
		t.Fatal(st)
	}
	// A detached snapshot may be edited without mutating the active session.
	st.Database.Put(xmltree.Name{Local: "Root"}, rules.Rule{Path: "/{}Root/{}Item", Mode: "attribute", Order: "insignificant", Fields: []rules.Field{{Name: "id"}}})
	if len(s.State().Database.Profiles) != 0 {
		t.Fatal("state leaked mutable database")
	}
	updated, err := s.SaveDatabase(st.Database, st.Revision)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(o.Rules + ".bak")
	if err != nil || !bytes.Equal(backup, initial) {
		t.Fatal("original database not backed up", err)
	}
	s.Close()
	before, _ := os.ReadFile(o.Rules)
	reopened, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, _ := os.ReadFile(o.Rules)
	if !bytes.Equal(before, after) || len(reopened.State().Database.Profiles) != 1 {
		t.Fatal("reopen changed or lost settings")
	}
	if _, err := reopened.Compare(); err == nil {
		t.Fatal("catalog allowed comparison without inputs")
	}
	if _, err := reopened.Tree(""); err == nil {
		t.Fatal("catalog exposed input trees")
	}
	if err := reopened.Save("local"); err == nil {
		t.Fatal("catalog allowed output save")
	}
	if err := rules.Update(o.Rules, func(db *rules.Database) error { db.Profiles[0].Rules[0].Order = "significant"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.SaveDatabase(updated.Database, updated.Revision); err == nil {
		t.Fatal("stale catalog overwrote concurrent edit")
	}
	if _, err := reopened.Reload(); err != nil {
		t.Fatal(err)
	}
	current := reopened.State()
	current.Database.Profiles[0].Rules[0].Mode = "invalid"
	if _, err := reopened.SaveDatabase(current.Database, current.Revision); err == nil {
		t.Fatal("invalid rule persisted")
	}
	current = reopened.State()
	current.Database.Profiles[0].Rules = nil
	if _, err := reopened.SaveDatabase(current.Database, current.Revision); err != nil {
		t.Fatal(err)
	}
	if reopened.Close() != Success {
		t.Fatal("normal editor close failed")
	}
	if _, err := reopened.SaveDatabase(current.Database, reopened.State().Revision); err == nil {
		t.Fatal("saved after close")
	}
}

func TestStandaloneDoesNotReplaceInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.xml")
	input := []byte("not an XML database")
	if err := os.WriteFile(path, input, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{Mode: "rules", Rules: path}); err == nil {
		t.Fatal("invalid settings accepted")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(input, got) {
		t.Fatal("invalid existing file replaced")
	}
}

func TestNameRulePreviewCoversAllDepths(t *testing.T) {
	x := `<r><i id="1"/><i id="2"/><wrap><i id="x"/><i id="x"/></wrap></r>`
	o := fixture(t, x, x, x)
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r := rules.Rule{Path: "/{}r/{}i", Selector: "{}i", Mode: "attribute", Order: "significant", Fields: []rules.Field{{Name: "id"}}}
	p, e := s.Preview(r)
	if e != nil || p.Valid || p.Total != 12 {
		t.Fatal(p, e)
	}
	if _, e = s.SaveRule(r, s.State().Revision); e == nil {
		t.Fatal("saved rule valid at only one depth")
	}
}
