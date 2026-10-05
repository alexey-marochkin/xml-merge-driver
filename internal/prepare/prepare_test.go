package prepare

import (
	"testing"

	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

func docs(t *testing.T, sources ...string) []*xmltree.Document {
	t.Helper()
	var out []*xmltree.Document
	for _, source := range sources {
		d, e := xmltree.Parse([]byte(source))
		if e != nil {
			t.Fatal(e)
		}
		out = append(out, d)
	}
	return out
}

func TestChecksUnmatchedBranchesBeforeComparison(t *testing.T) {
	r, e := Analyze(docs(t, `<r/>`, `<r/>`, `<r><new><x/><x/></new></r>`), rules.New(), true)
	if e != nil {
		t.Fatal(e)
	}
	if r.Ready || r.Missing != 1 {
		t.Fatalf("%+v", r)
	}
	if r.Groups[2].Path != "/{}r/{}new/{}x" {
		t.Fatal(r.Groups[2].Path)
	}
}

func TestInferenceValidatesAllParents(t *testing.T) {
	s := `<r><a id="1"><i id="1"/><i id="2"/></a><a id="2"><i id="1"/><i id="1"/></a></r>`
	r, e := Analyze(docs(t, s, s, s), rules.New(), true)
	if e != nil {
		t.Fatal(e)
	}
	if r.Ready {
		t.Fatal("accepted a duplicate key in a second parent")
	}
	if len(r.Learned) != 1 {
		t.Fatalf("expected parent id rule, got %+v", r.Learned)
	}
}

func TestManualRuleNotSilentlyReplaced(t *testing.T) {
	s := `<r><i id="same" code="1"/><i id="same" code="2"/></r>`
	db := rules.New()
	db.Put(xmltree.Name{Local: "r"}, rules.Rule{Path: "/{}r/{}i", Mode: "attribute", Order: "significant", Origin: "manual", Fields: []rules.Field{{Name: "id"}}})
	r, e := Analyze(docs(t, s, s, s), db, true)
	if e != nil {
		t.Fatal(e)
	}
	if r.Ready || len(r.Learned) != 0 {
		t.Fatal("manual rule overwritten")
	}
}

func TestPreviewFindsDuplicatesOutsideDisplayedRows(t *testing.T) {
	d := docs(t, `<r><i id="1"/><i id="1"/></r>`)[0]
	g := &Group{Sets: []Set{{Side: "local", Nodes: d.Root.Children()}}}
	p := PreviewRule(g, rules.Rule{Mode: "attribute", Fields: []rules.Field{{Name: "id"}}})
	if p.Valid || !p.Rows[0].Duplicate || !p.Rows[1].Duplicate {
		t.Fatal(p)
	}
}

func TestAutomaticNameRuleCoversDifferentDepths(t *testing.T) {
	x := `<r><i id="1"/><i id="2"/><wrap><i id="3"/><i id="4"/></wrap></r>`
	r, e := Analyze(docs(t, x, x, x), rules.New(), true)
	if e != nil || !r.Ready || len(r.Learned) != 1 || r.Learned[0].Selector != "{}i" || r.Learned[0].Path != "" {
		t.Fatal(r, e)
	}
	x = `<r><i id="1"/><i id="2"/><wrap><i id="3"/><i id="3"/></wrap></r>`
	r, e = Analyze(docs(t, x, x, x), rules.New(), true)
	if e != nil || r.Ready || len(r.Learned) != 0 {
		t.Fatal("inferred name key invalid at another depth", r, e)
	}
}
