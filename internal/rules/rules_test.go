package rules

import (
	"os"
	"path/filepath"
	"testing"

	"xmlmerge/internal/xmltree"
)

func TestInheritanceAndKeys(t *testing.T) {
	d, err := xmltree.Parse([]byte(`<root id="r"><child id="c"><leaf id="l"/></child></root>`))
	if err != nil {
		t.Fatal(err)
	}
	p := Profile{Root: "root", Rules: []Rule{{Path: Path(d.Root), Mode: "attribute", Order: "insignificant", Fields: []Field{{Name: "id"}}}}}
	leaf := d.Root.Children()[0].Children()[0]
	r := p.Effective(leaf)
	if r != &p.Rules[0] {
		t.Fatal("transitive inheritance did not resolve to the original rule")
	}
	key, err := r.Key(leaf)
	if err != nil || key != `["{}leaf","l"]` {
		t.Fatalf("%s: %v", key, err)
	}
	if len(p.Rules) != 1 {
		t.Fatal("inheritance materialized rules")
	}
}

func TestProfileDefaultIsSharedFallbackAtEveryDepth(t *testing.T) {
	d, err := xmltree.Parse([]byte(`<r><parent id="p"><leaf>A</leaf><item id="x"/></parent><leaf>B</leaf></r>`))
	if err != nil {
		t.Fatal(err)
	}
	p := Profile{Root: "r", Rules: []Rule{
		{Selector: "*", Mode: "text", Order: "insignificant"},
		{Selector: "parent", Mode: "attribute", Order: "significant", Fields: []Field{{Name: "id"}}},
		{Selector: "item", Mode: "attribute", Order: "significant", Fields: []Field{{Name: "id"}}},
	}}
	for _, n := range []*xmltree.Node{d.Root.Children()[0].Children()[0], d.Root.Children()[1]} {
		if r := p.Effective(n); r != &p.Rules[0] {
			t.Fatal("parent rule replaced profile default", r)
		}
	}
	if r := p.Effective(d.Root.Children()[0].Children()[1]); r != &p.Rules[2] {
		t.Fatal("own rule did not override default")
	}
	p.Rules[0].Order = "significant"
	if p.Effective(d.Root.Children()[1]).Order != "significant" || len(p.Rules) != 3 {
		t.Fatal("default was copied into elements")
	}
}

func TestInventorySurvivesCloneOverlayAndStore(t *testing.T) {
	db := New()
	db.Version = 2
	db.Profiles = []Profile{{Root: "r", Rules: []Rule{{Selector: "*", Mode: "text", Order: "significant"}}, KnownElements: []KnownElement{{Name: "item", Namespace: "urn:x", Attributes: []Field{{Name: "id"}}, Elements: []Field{{Name: "name"}}}}}}
	cloned := Clone(db)
	cloned.Profiles[0].KnownElements[0].Attributes[0].Name = "changed"
	if db.Profiles[0].KnownElements[0].Attributes[0].Name != "id" {
		t.Fatal("clone shares catalog fields")
	}
	out := Overlay(db, New())
	if len(out.Profiles[0].KnownElements) != 1 {
		t.Fatal("overlay lost catalog")
	}
	path := filepath.Join(t.TempDir(), "rules.xml")
	if e := Update(path, func(current *Database) error { *current = *out; return nil }); e != nil {
		t.Fatal(e)
	}
	loaded, e := Load(path)
	if e != nil || len(loaded.Profiles[0].KnownElements[0].Elements) != 1 {
		t.Fatal("store lost catalog", e)
	}
}

func TestOwnKeyCanUseLiveDefaultOrder(t *testing.T) {
	d, e := xmltree.Parse([]byte(`<r><i id="1"/></r>`))
	if e != nil {
		t.Fatal(e)
	}
	p := Profile{Root: "r", Rules: []Rule{{Selector: "*", Mode: "text", Order: "significant"}, {Selector: "i", Mode: "attribute", Order: "default", Fields: []Field{{Name: "id"}}}}}
	db := New()
	db.Version = 2
	db.Profiles = []Profile{p}
	if e = db.Validate(); e != nil {
		t.Fatal(e)
	}
	n := d.Root.Children()[0]
	if r := p.Effective(n); r.Order != "significant" || r.Mode != "attribute" || r.ConfiguredOrder != "default" {
		t.Fatal(r)
	}
	p.Rules[0].Order = "insignificant"
	if p.Effective(n).Order != "insignificant" || p.Rules[1].Order != "default" {
		t.Fatal("order was flattened")
	}
}

func TestCompositeAndMissing(t *testing.T) {
	d, err := xmltree.Parse([]byte(`<r><code>A|B</code><language>C</language></r>`))
	if err != nil {
		t.Fatal(err)
	}
	r := Rule{Mode: "element", Fields: []Field{{Name: "code"}, {Name: "language"}}}
	if key, err := r.Key(d.Root); err != nil || key != `["{}r","A|B","C"]` {
		t.Fatalf("%s %v", key, err)
	}
	r.Fields = append(r.Fields, Field{Name: "missing"})
	if _, err := r.Key(d.Root); err == nil {
		t.Fatal("missing field accepted")
	}
}

func TestStoreBackupAndLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.xml")
	change := func(d *Database) error { d.Profiles = append(d.Profiles, Profile{Root: "r"}); return nil }
	if err := Update(path, change); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Update(path, func(d *Database) error { return nil }); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(first) != string(backup) {
		t.Fatal("invalid backup", err)
	}
	if err := os.WriteFile(path+".lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Update(path, change); err == nil {
		t.Fatal("lock ignored")
	}
	d, err := Load(path)
	if err != nil || len(d.Profiles) != 1 {
		t.Fatal("database changed while locked", err)
	}
}
