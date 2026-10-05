package rules_test

import (
	"os"
	"path/filepath"
	"testing"

	"xmlmerge/internal/merge"
	"xmlmerge/internal/prepare"
	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

func imported(t *testing.T) *rules.Database {
	t.Helper()
	// Keep this historical import behavior independent of the user's mutable
	// rules.xml and of the private source formats used to create it.
	const sample = `<xmlmerge version="2">
  <profile root="ConfigDumpInfo" namespace="*">
    <rule selector="*" mode="text" order="insignificant" origin="imported" trim-space="true" />
    <rule selector="Metadata" mode="attribute" order="default" origin="imported" no-inherit="true">
      <field name="name" lexical="true" /><field name="id" lexical="true" />
    </rule>
  </profile>
  <profile root="DataCompositionSchema" namespace="*">
    <rule selector="*" mode="text" order="significant" origin="imported" trim-space="true" />
    <rule selector="dataSet" mode="element" order="default" origin="imported" no-inherit="true">
      <field name="name" lexical="true" trim-space="true" />
    </rule>
    <rule selector="dataSet/name" mode="text" order="default" origin="imported" trim-space="true" no-inherit="true" />
    <rule selector="field" mode="element" order="default" origin="imported" no-inherit="true">
      <field name="field" lexical="true" trim-space="true" />
    </rule>
    <rule selector="field/field" mode="text" order="default" origin="imported" trim-space="true" no-inherit="true" />
  </profile>
</xmlmerge>`
	path := filepath.Join(t.TempDir(), "imported-rules.xml")
	if err := os.WriteFile(path, []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := rules.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func doc(t *testing.T, s string) *xmltree.Document {
	t.Helper()
	d, e := xmltree.Parse([]byte(s))
	if e != nil {
		t.Fatal(e)
	}
	return d
}

func TestImportedKeysAtDifferentDepths(t *testing.T) {
	db := imported(t)
	d := doc(t, `<ConfigDumpInfo xmlns="urn:test"><Metadata name="A" id="1"/><wrap><Metadata name="B" id="2"/><Metadata name="B" id="3"/></wrap></ConfigDumpInfo>`)
	p := db.Profile(d.Root.Name)
	for _, n := range []*xmltree.Node{d.Root.Children()[0], d.Root.Children()[1].Children()[0]} {
		r := p.Effective(n)
		if r == nil || r.Selector != "Metadata" || len(r.Fields) != 2 {
			t.Fatal(r)
		}
		if _, e := r.Key(n); e != nil {
			t.Fatal(e)
		}
	}
	r, e := prepare.Analyze([]*xmltree.Document{d, d, d}, db, false)
	if e != nil || !r.Ready {
		t.Fatal(r, e)
	}
}

func TestImportedElementKeysAndNestedOverride(t *testing.T) {
	db := imported(t)
	d := doc(t, `<DataCompositionSchema xmlns="urn:test"><dataSet><name> A </name></dataSet><dataSet><name>B</name></dataSet><field><field>Value</field></field></DataCompositionSchema>`)
	p := db.Profile(d.Root.Name)
	key, e := p.Effective(d.Root.Children()[0]).Key(d.Root.Children()[0])
	if e != nil {
		t.Fatal(e)
	}
	n := doc(t, `<dataSet xmlns="urn:test"><name>A</name></dataSet>`).Root
	key2, e := p.Effective(n).Key(n)
	if e != nil || key != key2 {
		t.Fatal(key, key2, e)
	}
	leaf := d.Root.Children()[2].Children()[0]
	if p.Effective(leaf).Selector != "field/field" {
		t.Fatal("nested content override lost")
	}
	r, e := prepare.Analyze([]*xmltree.Document{d, d, d}, db, false)
	if e != nil || !r.Ready {
		t.Fatal(r, e)
	}
}

func TestImportedOrderAndSafeExplicitKeyFailure(t *testing.T) {
	db := imported(t)
	b := []byte(`<ConfigDumpInfo xmlns="urn:test"><Metadata name="A" id="1" v="0"/><Metadata name="B" id="2" v="0"/></ConfigDumpInfo>`)
	l := []byte(`<ConfigDumpInfo xmlns="urn:test"><Metadata name="B" id="2" v="0"/><Metadata name="A" id="1" v="local"/></ConfigDumpInfo>`)
	r := []byte(`<ConfigDumpInfo xmlns="urn:test"><Metadata name="A" id="1" v="0"/><Metadata name="B" id="2" v="remote"/></ConfigDumpInfo>`)
	result, e := merge.Merge(b, l, r, db)
	if e != nil || len(result.Conflicts) > 0 {
		t.Fatal(result, e)
	}
	d := doc(t, string(result.Data))
	children := d.Root.Children()
	if name, _ := children[0].Attribute(xmltree.Name{Local: "name"}); name != "B" {
		t.Fatal("local order lost")
	}
	if value, _ := children[0].Attribute(xmltree.Name{Local: "v"}); value != "remote" {
		t.Fatal("remote change lost")
	}
	if value, _ := children[1].Attribute(xmltree.Name{Local: "v"}); value != "local" {
		t.Fatal("local change lost")
	}
	bad := doc(t, `<ConfigDumpInfo><Metadata id="1"/><Metadata id="2"/></ConfigDumpInfo>`)
	report, e := prepare.Analyze([]*xmltree.Document{bad, bad, bad}, db, true)
	if e != nil || report.Ready {
		t.Fatal("explicit composite key silently replaced", e)
	}
}

func TestExpandedNameSelectorAndNamespaceIsolation(t *testing.T) {
	db := rules.New()
	name := xmltree.Name{URI: "https://example.test/ns", Local: "Root"}
	db.Put(name, rules.Rule{Selector: "{https://example.test/ns}Item", Mode: "attribute", Order: "significant", Origin: "manual", Fields: []rules.Field{{Name: "id"}}})
	if e := db.Validate(); e != nil {
		t.Fatal(e)
	}
	d := doc(t, `<a:Root xmlns:a="https://example.test/ns" xmlns:b="https://example.test/ns"><a:wrap><b:Item id="1"/></a:wrap></a:Root>`)
	p := db.Profile(name)
	n := d.Root.Children()[0].Children()[0]
	if p.Effective(n) == nil {
		t.Fatal("renamed prefix not matched by URI")
	}
	if p.Effective(doc(t, `<Item xmlns="urn:other" id="1"/>`).Root) != nil {
		t.Fatal("different URI matched")
	}
	path := filepath.Join(t.TempDir(), "rules.xml")
	if e := rules.Update(path, func(current *rules.Database) error { *current = *rules.Clone(db); return nil }); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); e != nil {
		t.Fatal(e)
	}
	loaded, e := rules.Load(path)
	if e != nil || loaded.Profile(name).Effective(n) == nil {
		t.Fatal("roundtrip lost name rule", e)
	}
}
