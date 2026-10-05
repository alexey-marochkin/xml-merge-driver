package interactive

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"xmlmerge/internal/xmltree"
)

func reviewID(t *testing.T, s *Session, name string) string {
	t.Helper()
	for id, n := range s.nodes {
		if n.row.Name == name {
			return id
		}
	}
	t.Fatal("missing node", name)
	return ""
}

func TestNodeDecisionsPreserveIndependentChangesAndUndo(t *testing.T) {
	o := fixture(t, `<r><a>0</a><b>0</b><c>0</c></r>`, `<r><a>L</a><b>L</b><c>0</c></r>`, `<r><a>R</a><b>0</b><c>R</c></r>`)
	s, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Compare()
	if err != nil || len(st.Conflicts) != 1 {
		t.Fatal(st, err)
	}
	id := reviewID(t, s, "a")
	center, err := s.Node("root")
	if err != nil || center.ResultXML != `<r><a>0</a><b>L</b><c>R</c></r>` {
		t.Fatal("center must retain BASE at conflict and combine independent changes", center.ResultXML, err)
	}
	if st.Conflicts[0].TargetID != id {
		t.Fatal("wrong conflict target", st.Conflicts)
	}
	st, err = s.Resolve(NodeDecision{ID: id, Choice: "remote"})
	if err != nil || !st.CanSave {
		t.Fatal(st, err)
	}
	if string(s.result) != `<r><a>R</a><b>L</b><c>R</c></r>` {
		t.Fatal(string(s.result))
	}
	center, err = s.Node("root")
	if err != nil || center.ResultXML != string(s.result) {
		t.Fatal("center differs from saved result", center.ResultXML, err)
	}
	if _, err = os.Stat(o.Output); !os.IsNotExist(err) {
		t.Fatal("decision wrote output")
	}
	st, err = s.Resolve(NodeDecision{ID: id, Choice: "reset"})
	if err != nil || st.CanSave || len(st.Conflicts) != 1 {
		t.Fatal(st, err)
	}
	st, err = s.Resolve(NodeDecision{ID: id, Choice: "xml", XML: `<a>Manual &amp; checked</a>`})
	if err != nil || !st.CanSave {
		t.Fatal(st, err)
	}
	before := string(s.result)
	if _, err = s.Resolve(NodeDecision{ID: id, Choice: "xml", XML: `<a>bad`}); err == nil {
		t.Fatal("accepted malformed XML")
	}
	if string(s.result) != before {
		t.Fatal("invalid edit changed result")
	}
	if err = s.Save("merged"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(o.Output)
	if !strings.Contains(string(data), `<b>L</b><c>R</c>`) {
		t.Fatal(string(data))
	}
}

func TestNodeDeletionAdditionNamespacesAndWhitespace(t *testing.T) {
	for _, choice := range []string{"local", "remote"} {
		t.Run(choice, func(t *testing.T) {
			s, err := New(fixture(t, `<r xmlns:p="urn:test"><p:a>0</p:a><z/></r>`, `<r xmlns:p="urn:test"><z/></r>`, `<r xmlns:p="urn:test"><p:a>R</p:a><z/></r>`))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Compare(); err != nil {
				t.Fatal(err)
			}
			st, err := s.Resolve(NodeDecision{ID: reviewID(t, s, "p:a"), Choice: choice})
			if err != nil || !st.CanSave {
				t.Fatal(st, err)
			}
			if strings.Contains(string(s.result), ">R</p:a>") != (choice == "remote") {
				t.Fatal(string(s.result))
			}
		})
	}
	s, err := New(fixture(t, "<r>\n\t<a/>\n\t<b/>\n</r>", "<r>\n\t<a/>\n\t<b/>\n</r>", "<r>\n\t<a/>\n\t<b/>\n</r>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compare(); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Tree("")
	if rows[0].Values[0].Text != "" {
		t.Fatal("formatting whitespace leaked into preview")
	}
}

func TestKeyedConflictTargetsAndParentDecisions(t *testing.T) {
	s, err := New(fixture(t, `<r><i id="1">0</i><i id="2">0</i></r>`, `<r><i id="1">L</i><i id="2">L</i></r>`, `<r><i id="1">R</i><i id="2">R</i></r>`))
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Compare()
	if err != nil || len(st.Conflicts) != 2 || st.Conflicts[0].TargetID == st.Conflicts[1].TargetID {
		t.Fatal(st, err)
	}
	id := st.Conflicts[0].TargetID
	st, err = s.Resolve(NodeDecision{ID: id, Choice: "local"})
	if err != nil || len(st.Conflicts) != 1 {
		t.Fatal(st, err)
	}
	if _, err = s.Resolve(NodeDecision{ID: "root", Choice: "remote"}); err == nil {
		t.Fatal("overwrote descendant decision")
	}
	if _, err = s.Resolve(NodeDecision{ID: id, Choice: "reset"}); err != nil {
		t.Fatal(err)
	}
	if st, err = s.Resolve(NodeDecision{ID: "root", Choice: "remote"}); err != nil || !st.CanSave {
		t.Fatal(st, err)
	}
	if _, err = s.Resolve(NodeDecision{ID: id, Choice: "local"}); err == nil {
		t.Fatal("edited masked descendant")
	}
}

func TestMoveEndpointDecisionDoesNotAcceptSameNamedNeighbour(t *testing.T) {
	base := `<r><Resource id="pack"><Properties><Name>Упаковка</Name><ChoiceParameters><item name="show"><value>false</value></item></ChoiceParameters></Properties></Resource><Dimension id="pack"><Properties><Name>Упаковка</Name><ChoiceParameters/></Properties></Dimension><other>0</other></r>`
	local := strings.Replace(base, "<other>0</other>", "<other>L</other>", 1)
	remote := strings.Replace(base, `<ChoiceParameters><item name="show"><value>false</value></item></ChoiceParameters>`, `<ChoiceParameters/>`, 1)
	at := strings.Index(remote, "<Dimension")
	remote = remote[:at] + strings.Replace(remote[at:], `<ChoiceParameters/>`, `<ChoiceParameters><item name="show"><value>false</value></item></ChoiceParameters>`, 1)
	for _, choice := range []string{"take-local", "take-remote", "xml"} {
		t.Run(choice, func(t *testing.T) {
			s, err := New(fixture(t, base, local, remote))
			if err != nil {
				t.Fatal(err)
			}
			st, err := s.Compare()
			if err != nil || len(st.Conflicts) == 0 {
				t.Fatal(st, err)
			}
			var source, destination string
			for id, n := range s.nodes {
				if n.row.Name != "item · show" {
					continue
				}
				for p := s.nodes[n.row.Parent]; p != nil; p = s.nodes[p.row.Parent] {
					if p.row.Name == "Resource · Упаковка" {
						source = id
					}
					if p.row.Name == "Dimension · Упаковка" {
						destination = id
					}
				}
			}
			if source == "" || destination == "" || source == destination {
				t.Fatal("missing distinct full-path endpoints")
			}
			before, _ := s.Node(destination)
			decision := NodeDecision{ID: source, Choice: choice, XML: `<item name="show"><value>edited</value></item>`}
			st, err = s.Resolve(decision)
			if err != nil || st.CanSave || len(st.Conflicts) == 0 {
				t.Fatal("other endpoint accepted implicitly", st, err)
			}
			after, _ := s.Node(destination)
			if before.ResultXML != after.ResultXML || before.ResultPresent != after.ResultPresent {
				t.Fatal("neighbour changed", before.ResultXML, after.ResultXML)
			}
			if len(st.Conflicts) != 1 || len(st.Conflicts[0].RelatedIDs) == 0 {
				t.Fatal("wrong pending path", st.Conflicts)
			}
			for _, id := range st.Conflicts[0].RelatedIDs {
				inside := false
				for n := s.nodes[id]; n != nil; n = s.nodes[n.row.Parent] {
					if n.row.ID == destination {
						inside = true
						break
					}
				}
				if !inside {
					t.Fatal("pending endpoint outside the destination", id)
				}
			}
			st, err = s.Resolve(NodeDecision{ID: destination, Choice: "take-remote"})
			if err != nil || !st.CanSave {
				t.Fatal(st, err)
			}
			if !strings.Contains(string(s.result), "<other>L</other>") {
				t.Fatal("lost independent change")
			}
			sourceResult, _ := s.Node(source)
			if choice == "take-remote" && sourceResult.ResultPresent {
				t.Fatal("source deletion undone")
			}
			if choice == "xml" && !strings.Contains(sourceResult.ResultXML, "edited") {
				t.Fatal("manual source edit lost")
			}
			st, err = s.Resolve(NodeDecision{ID: destination, Choice: "reset"})
			if err != nil || st.CanSave {
				t.Fatal("reset did not restore pending endpoint", st, err)
			}
			reset, _ := s.Node(destination)
			if reset.ResultXML != before.ResultXML {
				t.Fatal("reset changed original scope")
			}
		})
	}
}

func TestSequentialRightArrowsUnderAutomaticallyAddedParent(t *testing.T) {
	base := `<r xmlns="urn:test" xmlns:app="urn:app"><Resource><Properties><Name>Упаковка</Name><ChoiceParameters><item id="one">1</item><item id="two">2</item></ChoiceParameters></Properties></Resource><other>0</other></r>`
	remote := `<r xmlns="urn:test" xmlns:app="urn:app"><Resource><Properties><Name>Упаковка</Name><ChoiceParameters/></Properties></Resource><Dimension><Properties><Name>Упаковка</Name><ChoiceParameters><item id="one">1</item><item id="two">2</item></ChoiceParameters><Auto>yes</Auto></Properties></Dimension><other>0</other></r>`
	local := strings.Replace(base, "<other>0</other>", "<other>L</other>", 1)
	for _, mode := range []string{"take-remote", "xml"} {
		t.Run(mode, func(t *testing.T) {
			s, err := New(fixture(t, base, local, remote))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Compare(); err != nil {
				t.Fatal(err)
			}
			var source, destination []string
			for _, id := range s.nodeOrder {
				n := s.nodes[id]
				if n.row.Name != "item" {
					continue
				}
				for p := s.nodes[n.row.Parent]; p != nil; p = s.nodes[p.row.Parent] {
					if p.row.Name == "Resource · Упаковка" {
						source = append(source, id)
					}
					if p.row.Name == "Dimension · Упаковка" {
						destination = append(destination, id)
					}
				}
			}
			if len(source) != 2 || len(destination) != 2 {
				t.Fatal(source, destination)
			}
			for i, id := range destination {
				detail, _ := s.Node(id)
				decision := NodeDecision{ID: id, Choice: mode, XML: detail.XML[2]}
				if mode == "xml" {
					decision.XML = strings.Replace(decision.XML, ">2<", ">manual<", 1)
				}
				st, err := s.Resolve(decision)
				if err != nil || st.CanSave {
					t.Fatal(st, err)
				}
				for j, target := range destination {
					n, _ := s.Node(target)
					if n.ResultPresent != (j <= i) {
						t.Fatalf("arrow %d changed wrong destination %d", i, j)
					}
				}
				for _, target := range source {
					n, _ := s.Node(target)
					if !n.ResultPresent {
						t.Fatal("destination arrow removed source")
					}
				}
			}
			for _, id := range source {
				if _, err = s.Resolve(NodeDecision{ID: id, Choice: "take-remote"}); err != nil {
					t.Fatal(err)
				}
			}
			if !s.state().CanSave {
				t.Fatal(s.state())
			}
			if !strings.Contains(string(s.result), "<Auto>yes</Auto>") || !strings.Contains(string(s.result), "<other>L</other>") {
				t.Fatal("lost unrelated automatic changes", string(s.result))
			}
			if st, err := s.Resolve(NodeDecision{ID: destination[1], Choice: "reset"}); err != nil || st.CanSave {
				t.Fatal(st, err)
			}
			first, _ := s.Node(destination[0])
			second, _ := s.Node(destination[1])
			if !first.ResultPresent || second.ResultPresent {
				t.Fatal("reset changed the first accepted item")
			}
		})
	}
}

func TestLeafArrowRestoresOnlyMissingAncestorPath(t *testing.T) {
	base := `<r xmlns="urn:test"><group id="g"><box><wanted>one</wanted><neighbour>two</neighbour></box><unrelated/></group></r>`
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			local := `<r xmlns="urn:test"/>`
			if explicit {
				local = base
			}
			s, err := New(fixture(t, base, local, base))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Compare(); err != nil {
				t.Fatal(err)
			}
			if explicit {
				if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, "group"), Choice: "xml", XML: ""}); err != nil {
					t.Fatal(err)
				}
			}
			wanted := reviewID(t, s, "wanted")
			st, err := s.Resolve(NodeDecision{ID: wanted, Choice: "take-remote"})
			if err != nil || !st.CanSave {
				t.Fatal(st, err)
			}
			result := string(s.result)
			if !strings.Contains(result, `id="g"`) || !strings.Contains(result, "<box") || s.center[wanted] == nil || s.center[wanted].DirectText() != "one" || strings.Contains(result, "neighbour") || strings.Contains(result, "unrelated") {
				t.Fatal(result)
			}
			if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, "neighbour"), Choice: "take-remote"}); err != nil {
				t.Fatal(err)
			}
			if s.center[wanted] == nil || s.center[wanted].DirectText() != "one" || s.center[reviewID(t, s, "neighbour")] == nil || s.center[reviewID(t, s, "neighbour")].DirectText() != "two" || strings.Contains(string(s.result), "unrelated") {
				t.Fatal(string(s.result))
			}
		})
	}
}

func TestEditWholeResultKeepsAcceptedChildAndIndependentChanges(t *testing.T) {
	s, err := New(fixture(t, `<r><a>0</a><b>0</b><c>0</c></r>`, `<r><a>L</a><b>L</b><c>0</c></r>`, `<r><a>R</a><b>0</b><c>R</c></r>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compare(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, "a"), Choice: "remote"}); err != nil {
		t.Fatal(err)
	}
	center, err := s.Node("root")
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(center.ResultXML, `<b>L</b>`, `<b>Manual</b>`, 1)
	st, err := s.Resolve(NodeDecision{ID: "root", Choice: "xml", XML: edited})
	if err != nil || !st.CanSave || st.Decisions != 1 {
		t.Fatal(st, err)
	}
	if string(s.result) != `<r><a>R</a><b>Manual</b><c>R</c></r>` {
		t.Fatal(string(s.result))
	}
	center, err = s.Node("root")
	if err != nil || center.ResultXML != string(s.result) {
		t.Fatal(center.ResultXML, err)
	}
	if err = s.Save("merged"); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(s.Options.Output)
	if err != nil || string(saved) != center.ResultXML {
		t.Fatal(string(saved), err)
	}
}

func TestTreeArrowsComposeOverAcceptedParent(t *testing.T) {
	s, err := New(fixture(t, `<r><a>0</a><z/></r>`, `<r><a>L</a><left/><z/></r>`, `<r><a>R</a><right/><z/></r>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compare(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Resolve(NodeDecision{ID: "root", Choice: "base"}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct{ name, choice string }{{"left", "take-local"}, {"right", "take-remote"}, {"a", "take-remote"}} {
		if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, v.name), Choice: v.choice}); err != nil {
			t.Fatal(v, err)
		}
	}
	center, err := s.Node("root")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<left/>", "<right/>", "<a>R</a>", "<z/>"} {
		if !strings.Contains(center.ResultXML, fragment) {
			t.Fatal(center.ResultXML)
		}
	}
	if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, "left"), Choice: "xml", XML: "<left>edited</left>"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(s.result), "<right/>") || !strings.Contains(string(s.result), "<left>edited</left>") {
		t.Fatal(string(s.result))
	}
	if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, "right"), Choice: "take-local"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(s.result), "<right/>") {
		t.Fatal("deletion arrow failed", string(s.result))
	}
}

func TestTextConflictRegionsFollowSelectedNode(t *testing.T) {
	s, err := New(fixture(t, "<r>\n<a>0</a>\n<b>0</b>\n</r>", "<r>\n<a>L</a>\n<b>0</b>\n</r>", "<r>\n<a>R</a>\n<b>0</b>\n</r>"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compare(); err != nil {
		t.Fatal(err)
	}
	root, err := s.Node("root")
	if err != nil {
		t.Fatal(err)
	}
	for _, regions := range root.ConflictLines {
		if len(regions) != 1 || regions[0] != ([2]int{1, 2}) {
			t.Fatal(root.ConflictLines)
		}
	}
	b, err := s.Node(reviewID(t, s, "b"))
	if err != nil {
		t.Fatal(err)
	}
	for _, regions := range b.ConflictLines {
		if len(regions) != 0 {
			t.Fatal("unrelated node marked conflicted", b.ConflictLines)
		}
	}
	if _, err = s.Resolve(NodeDecision{ID: reviewID(t, s, "a"), Choice: "take-local"}); err != nil {
		t.Fatal(err)
	}
	root, err = s.Node("root")
	if err != nil {
		t.Fatal(err)
	}
	for _, regions := range root.ConflictLines {
		if len(regions) != 0 {
			t.Fatal(root.ConflictLines)
		}
	}
}

func TestResultOriginIncludesAutomaticChangesAndExplicitChoice(t *testing.T) {
	s, err := New(fixture(t, `<r><a>0</a><b>0</b><c>0</c><d>0</d></r>`, `<r><a>L</a><b>0</b><c>X</c><d>L</d></r>`, `<r><a>0</a><b>R</b><c>X</c><d>R</d></r>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Compare(); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		name string
		want [2]bool
	}{{"a", [2]bool{true, false}}, {"b", [2]bool{false, true}}, {"c", [2]bool{true, true}}, {"d", [2]bool{false, false}}} {
		if got := s.row(reviewID(t, s, v.name)).Origin; got != v.want {
			t.Fatal(v.name, got, v.want)
		}
	}
	id := reviewID(t, s, "d")
	if _, err = s.Resolve(NodeDecision{ID: id, Choice: "take-remote"}); err != nil {
		t.Fatal(err)
	}
	if got := s.row(id).Origin; got != ([2]bool{false, true}) {
		t.Fatal(got)
	}
	if _, err = s.Resolve(NodeDecision{ID: id, Choice: "xml", XML: "<d>manual</d>"}); err != nil {
		t.Fatal(err)
	}
	if got := s.row(id).Origin; got != ([2]bool{}) {
		t.Fatal(got)
	}
}

func TestMoveConflictPreservesIndependentAutomaticChanges(t *testing.T) {
	base := "<r>\n<Name>before</Name>\n<a>\n<x/>\n<y/>\n</a>\n<b/>\n</r>"
	local := strings.Replace(base, "</r>", "<custom/>\n</r>", 1)
	remote := "<r>\n<Name>supplier</Name>\n<a/>\n<b>\n<x/>\n<y/>\n</b>\n</r>"
	s, err := New(fixture(t, base, local, remote))
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Compare()
	if err != nil || len(st.Conflicts) != 1 || st.CanSave {
		t.Fatal(st, err)
	}
	detail, err := s.Node("root")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail.ResultXML, "<Name>supplier</Name>") || !strings.Contains(detail.ResultXML, "<custom/>") {
		t.Fatal("independent supplier and local changes lost", detail.ResultXML)
	}
	if strings.Count(detail.ResultXML, "<x/>") != 1 || strings.Count(detail.ResultXML, "<y/>") != 1 {
		t.Fatal("unresolved moves must retain their BASE locations", detail.ResultXML)
	}
	doc, err := xmltree.Parse([]byte(detail.ResultXML))
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range doc.Root.Children() {
		if child.Name.Local == "a" && len(child.Children()) != 2 || child.Name.Local == "b" && len(child.Children()) != 0 {
			t.Fatal("move preview changed source/destination membership", detail.ResultXML)
		}
	}
	name := reviewID(t, s, "Name")
	if got := s.row(name).Origin; got != ([2]bool{false, true}) {
		t.Fatal("supplier origin missing", got)
	}
	if got := s.row(reviewID(t, s, "custom")).Origin; got != ([2]bool{true, false}) {
		t.Fatal("local origin missing", got)
	}
	child, err := s.Node(name)
	if err != nil {
		t.Fatal(err)
	}
	for side, regions := range child.ConflictLines {
		if len(regions) != 0 {
			t.Fatal("independent supplier edit marked conflicted", side, regions)
		}
	}
	for side, regions := range detail.ConflictLines {
		if len(regions) == 0 {
			t.Fatal("move regions missing", side)
		}
		lines := strings.Split(detail.XML[side], "\n")
		for _, span := range regions {
			for _, line := range lines[span[0]:span[1]] {
				if !strings.Contains(line, "<x") && !strings.Contains(line, "<y") {
					t.Fatal("unrelated line marked conflicted", side, line)
				}
			}
		}
	}
}
