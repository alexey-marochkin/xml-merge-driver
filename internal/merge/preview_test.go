package merge

import (
	"strings"
	"testing"
	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

func TestPreviewRetainsBaseAtConflicts(t *testing.T) {
	for _, tc := range []struct{ b, l, r, want string }{
		{`<r><a>0</a><b>0</b></r>`, `<r><b>L</b></r>`, `<r><a>R</a><b>0</b></r>`, `<r><a>0</a><b>L</b></r>`},
		{`<r><a>0</a><b>0</b></r>`, `<r><a>L</a><b>0</b></r>`, `<r><b>R</b></r>`, `<r><a>0</a><b>R</b></r>`},
		{`<r><b>0</b></r>`, `<r><a>L</a><b>L</b></r>`, `<r><a>R</a><b>0</b></r>`, `<r><b>L</b></r>`},
	} {
		result, err := Merge([]byte(tc.b), []byte(tc.l), []byte(tc.r), rules.New())
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Conflicts) == 0 || len(result.Data) != 0 {
			t.Fatal("unresolved preview became saveable")
		}
		if string(result.PreviewData) != tc.want {
			t.Fatalf("want %s got %s", tc.want, result.PreviewData)
		}
		if _, err := xmltree.Parse(result.PreviewData); err != nil {
			t.Fatal(err)
		}
	}
}
func TestMovePreviewKeepsBaseBranchAndIndependentChange(t *testing.T) {
	b := `<r><group><a><x/></a><b/></group><other>0</other></r>`
	l := `<r><group><a><x/></a><b/></group><other>L</other></r>`
	r := `<r><group><a/><b><x/></b></group><other>0</other></r>`
	result, err := Merge([]byte(b), []byte(l), []byte(r), rules.New())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) == 0 || len(result.Data) != 0 || !strings.Contains(string(result.PreviewData), `<a><x/></a>`) || !strings.Contains(string(result.PreviewData), `<other>L</other>`) {
		t.Fatal(result)
	}
	doc, err := xmltree.Parse(result.PreviewData)
	if err != nil {
		t.Fatal(err)
	}
	group := doc.Root.Children()[0].Children()
	if len(group) != 2 || len(group[1].Children()) != 0 {
		t.Fatal("move destination must stay empty", string(result.PreviewData))
	}
}

func TestMovePreviewRestoresDeletedSourceContainer(t *testing.T) {
	base := `<r><source><x/></source><target/><other>0</other></r>`
	for _, reverse := range []bool{false, true} {
		local := `<r><target/><other>L</other></r>`
		remote := `<r><source/><target><x/></target><other>0</other></r>`
		if reverse {
			local, remote = remote, local
		}
		result, err := Merge([]byte(base), []byte(local), []byte(remote), rules.New())
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Conflicts) == 0 || len(result.Data) != 0 {
			t.Fatal("move/delete must remain unresolved")
		}
		doc, err := xmltree.Parse(result.PreviewData)
		if err != nil {
			t.Fatal(err)
		}
		sourceFound := false
		for _, child := range doc.Root.Children() {
			if child.Name.Local == "source" {
				sourceFound = true
				if len(child.Children()) != 1 || child.Children()[0].Name.Local != "x" {
					t.Fatal(string(result.PreviewData))
				}
			}
			if child.Name.Local == "target" && len(child.Children()) != 0 {
				t.Fatal("duplicated moved node", string(result.PreviewData))
			}
		}
		if !sourceFound || !strings.Contains(string(result.PreviewData), "<other>L</other>") {
			t.Fatal("source or independent change missing", string(result.PreviewData))
		}
	}
}
