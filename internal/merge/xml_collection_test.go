package merge

import (
	"testing"

	"xmlmerge/internal/rules"
)

func TestExplicitReferenceSetMembership(t *testing.T) {
	db := &rules.Database{Version: 2, Profiles: []rules.Profile{{Root: "r", Rules: []rules.Rule{
		{Selector: "*", Mode: "auto", Order: "significant"},
		{Selector: "i", Mode: "text", Order: "insignificant", AllowDeleteAdd: true},
	}}}}
	result, err := Merge([]byte(`<r><i>old</i><i>keep</i></r>`), []byte(`<r><i>old</i><i>keep</i><i>custom</i></r>`), []byte(`<r><i>new</i><i>keep</i></r>`), db)
	if err != nil || len(result.Conflicts) != 0 {
		t.Fatalf("reference set: %+v %v", result, err)
	}
	if string(result.Data) != `<r><i>new</i><i>keep</i><i>custom</i></r>` {
		t.Fatalf("lost reference membership: %s", result.Data)
	}
	// Set semantics must retain the ordinary deletion-versus-edit conflict.
	result, err = Merge([]byte(`<r><i a="1">old</i></r>`), []byte(`<r><i a="2">old</i></r>`), []byte(`<r/>`), db)
	if err != nil || len(result.Conflicts) == 0 {
		t.Fatalf("deleted modified reference accepted: %+v %v", result, err)
	}
}

func TestNestedCharacteristicKey(t *testing.T) {
	db := &rules.Database{Version: 2, Profiles: []rules.Profile{{Root: "r", Rules: []rules.Rule{
		{Selector: "*", Mode: "auto", Order: "significant"},
		{Selector: "c", Mode: "element", Order: "significant", Fields: []rules.Field{{Name: "types/key"}}},
	}}}}
	base := []byte(`<r><c v="1"><types><key>additional</key></types></c><c v="1"><types><key>information</key></types></c></r>`)
	local := []byte(`<r><c v="2"><types><key>additional</key></types></c><c v="1"><types><key>information</key></types></c></r>`)
	remote := []byte(`<r><c v="1"><types><key>additional</key></types></c><c v="3"><types><key>information</key></types></c></r>`)
	result, err := Merge(base, local, remote, db)
	if err != nil || len(result.Conflicts) != 0 || string(result.Data) != `<r><c v="2"><types><key>additional</key></types></c><c v="3"><types><key>information</key></types></c></r>` {
		t.Fatalf("nested keys: %+v %v", result, err)
	}
}

func TestCharacteristicSourceAttributeKey(t *testing.T) {
	db := &rules.Database{Version: 2, Profiles: []rules.Profile{{Root: "r", Rules: []rules.Rule{
		{Selector: "*", Mode: "auto", Order: "significant"},
		{Selector: "c", Mode: "element", Order: "significant", Fields: []rules.Field{{Name: "types/@from"}}},
	}}}}
	base := []byte(`<r><c><types from="additional"><key>id</key></types><use>false</use></c><c><types from="information"><key>id</key></types><use>false</use></c></r>`)
	local := []byte(`<r><c><types from="additional"><key>-1</key></types><use>false</use></c><c><types from="information"><key>-1</key></types><use>false</use></c></r>`)
	remote := []byte(`<r><c><types from="additional"><key>id</key></types><use>true</use></c><c><types from="information"><key>id</key></types><use>false</use></c></r>`)
	result, err := Merge(base, local, remote, db)
	want := `<r><c><types from="additional"><key>-1</key></types><use>true</use></c><c><types from="information"><key>-1</key></types><use>false</use></c></r>`
	if err != nil || len(result.Conflicts) != 0 || string(result.Data) != want {
		t.Fatalf("source attribute keys: %+v %v", result, err)
	}
}
