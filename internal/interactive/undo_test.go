package interactive

import (
	"strings"
	"testing"
	"xmlmerge/internal/rules"
)

func TestUndoRestoresResultConflictsAndEarlierDecisions(t *testing.T) {
	s, err := New(fixture(t, `<r><a>0</a><b>0</b></r>`, `<r><a>L</a><b>L</b></r>`, `<r><a>R</a><b>0</b></r>`))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.Compare()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Node("root")
	a, b := reviewID(t, s, "a"), reviewID(t, s, "b")
	first, err := s.Resolve(NodeDecision{ID: a, Choice: "take-remote"})
	if err != nil || !first.CanUndo || !first.CanSave {
		t.Fatal(first, err)
	}
	afterFirst, _ := s.Node("root")
	if _, err = s.Resolve(NodeDecision{ID: b, Choice: "xml", XML: "<b>manual</b>"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Resolve(NodeDecision{ID: b, Choice: "xml", XML: "<broken>"}); err == nil {
		t.Fatal("invalid edit accepted")
	}
	undone, err := s.Undo()
	got, _ := s.Node("root")
	if err != nil || got.ResultXML != afterFirst.ResultXML || undone.Decisions != 1 || !undone.CanUndo {
		t.Fatal(undone, got.ResultXML, err)
	}
	undone, err = s.Undo()
	got, _ = s.Node("root")
	if err != nil || got.ResultXML != before.ResultXML || undone.CanUndo || undone.Decisions != 0 || len(undone.Conflicts) != len(initial.Conflicts) || undone.CanSave {
		t.Fatal(undone, got.ResultXML, err)
	}
	if _, err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	// A new action after undo must not mutate an older snapshot.
	if _, err = s.Resolve(NodeDecision{ID: a, Choice: "take-local"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Resolve(NodeDecision{ID: a, Choice: "reset"}); err != nil {
		t.Fatal(err)
	}
	undone, err = s.Undo()
	got, _ = s.Node(a)
	if err != nil || got.ResultXML != "<a>L</a>" || !undone.CanSave {
		t.Fatal(undone, got, err)
	}
}

func TestRestoredPathKeepsNestedIdentityFields(t *testing.T) {
	base := `<r><group><Properties><Name>g</Name><Unused/></Properties><box><wanted>one</wanted><other/></box></group></r>`
	for _, explicit := range []bool{false, true} {
		local := `<r/>`
		if explicit {
			local = base
		}
		s, err := New(fixture(t, base, local, base))
		if err != nil {
			t.Fatal(err)
		}
		s.db.Put(s.docs[0].Root.Name, rules.Rule{Selector: "group", Mode: "element", Order: "significant", Fields: []rules.Field{{Name: "Properties/Name"}}})
		if _, err = s.Compare(); err != nil {
			t.Fatal(err)
		}
		if explicit {
			for id, n := range s.nodes {
				if n.nodes[0] != nil && n.nodes[0].Name.Local == "group" {
					if _, err = s.Resolve(NodeDecision{ID: id, Choice: "xml"}); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, "wanted"), Choice: "take-remote"}); err != nil {
			t.Fatal(err)
		}
		got := string(s.result)
		if !strings.Contains(got, "<Name>g</Name>") || !strings.Contains(got, "<wanted>one</wanted>") || strings.Contains(got, "Unused") || strings.Contains(got, "other") {
			t.Fatal(explicit, got)
		}
	}
}

func TestUndoRestoredAncestorChain(t *testing.T) {
	base := `<r><group><a>one</a><box><b>two</b><other/></box></group></r>`
	s, err := New(fixture(t, base, base, base))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compare(); err != nil {
		t.Fatal(err)
	}
	group := reviewID(t, s, "group")
	for _, d := range []NodeDecision{{ID: group, Choice: "xml"}, {ID: reviewID(t, s, "a"), Choice: "take-remote"}, {ID: reviewID(t, s, "b"), Choice: "take-remote"}} {
		if _, err = s.Resolve(d); err != nil {
			t.Fatal(err)
		}
	}
	if s.center[reviewID(t, s, "other")] != nil {
		t.Fatal("unrelated sibling restored")
	}
	if _, err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.center[reviewID(t, s, "a")] == nil || s.center[reviewID(t, s, "box")] != nil {
		t.Fatal("undo must remove only the last restored chain")
	}
	if _, err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.center[group] != nil {
		t.Fatal("deleted group restored too early")
	}
	if _, err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	if string(s.result) != base {
		t.Fatal(string(s.result))
	}
}
