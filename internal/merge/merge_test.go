package merge

import (
	"testing"

	"xmlmerge/internal/rules"
)

func TestMerge(t *testing.T) {
	for _, tc := range []struct {
		name, b, l, r, want string
		conflict            bool
	}{
		{"independent attributes", `<r a = '1' b="2"/>`, `<r a = '3' b="2"/>`, `<r a = '1' b="4"/>`, `<r a = '3' b="4"/>`, false},
		{"same attribute", `<r a="1"/>`, `<r a="2"/>`, `<r a="3"/>`, "", true},
		{"text", `<r>old</r>`, `<r>old</r>`, `<r>new</r>`, `<r>new</r>`, false},
		{"CDATA", "<r><![CDATA[a\r\nb]]></r>", "<r><![CDATA[a\r\nb]]></r>", "<r><![CDATA[a\nb\rc]]></r>", "<r><![CDATA[a\nb\rc]]></r>", false},
		{"independent nodes", `<r><a>1</a><b>2</b></r>`, `<r><a>3</a><b>2</b></r>`, `<r><a>1</a><b>4</b></r>`, `<r><a>3</a><b>4</b></r>`, false},
		{"delete edit", `<r><a>1</a></r>`, `<r/>`, `<r><a>2</a></r>`, "", true},
		{"addition", `<r><a/></r>`, `<r><a/></r>`, `<r><a/><b/></r>`, `<r><a/><b/></r>`, false},
		{"attributes append", `<r z="1"/>`, `<r z="1" l="2"/>`, `<r b="3" z="1" a="4"/>`, `<r z="1" l="2" b="3" a="4"/>`, false},
		{"key changed", `<r><i id="1"/><i id="2"/></r>`, `<r><i id="1"/><i id="2"/></r>`, `<r><i id="3"/><i id="2"/></r>`, "", true},
		{"same addition different positions", `<r><a/><b/></r>`, `<r><a/><x/><b/></r>`, `<r><a/><b/><x/></r>`, "", true},
		{"delete versus cross-parent move", `<r><a><x/></a><b/></r>`, `<r><a/><b/></r>`, `<r><a/><b><x/></b></r>`, "", true},
		{"different prefix bindings on inserted node", `<r xmlns:p="old"><a/></r>`, `<r xmlns:p="old"><a/></r>`, `<r xmlns:p="new"><a/><p:x/></r>`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Merge([]byte(tc.b), []byte(tc.l), []byte(tc.r), rules.New())
			if err != nil {
				t.Fatal(err)
			}
			if (len(got.Conflicts) > 0) != tc.conflict {
				t.Fatalf("conflicts: %+v, result: %s", got.Conflicts, got.Data)
			}
			if !tc.conflict && string(got.Data) != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got.Data)
			}
		})
	}
}

func TestUnorderedChildren(t *testing.T) {
	db := rules.New()
	db.Profiles = []rules.Profile{{Root: "r", Rules: []rules.Rule{
		{Path: "/{}r", Mode: "text", Order: "insignificant"},
		{Path: "/{}r/{}i", Mode: "attribute", Order: "significant", Fields: []rules.Field{{Name: "id"}}},
	}}}
	b := `<r><i id="1" v="a"/><i id="2" v="b"/></r>`
	l := `<r><i id="2" v="local"/><i id="1" v="a"/></r>`
	r := `<r><i id="1" v="remote"/><i id="3" v="new"/><i id="2" v="b"/></r>`
	got, err := Merge([]byte(b), []byte(l), []byte(r), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Conflicts) > 0 {
		t.Fatal(got.Conflicts)
	}
	want := `<r><i id="2" v="local"/><i id="1" v="remote"/><i id="3" v="new"/></r>`
	if string(got.Data) != want {
		t.Fatalf("%s", got.Data)
	}
}

func TestExplicitKeyOnSingletonIsRespected(t *testing.T) {
	db := rules.New()
	db.Profiles = []rules.Profile{{Root: "r", Rules: []rules.Rule{{Path: "/{}r/{}i", Mode: "attribute", Order: "significant", Fields: []rules.Field{{Name: "id"}}}}}}
	res, err := Merge([]byte(`<r><i id="old"/></r>`), []byte(`<r><i id="old"/></r>`), []byte(`<r><i id="new"/></r>`), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("explicit identity silently replaced by structural matching")
	}
}
