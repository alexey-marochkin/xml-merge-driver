package rules

import (
	"fmt"
	"sort"
	"strings"

	"xmlmerge/internal/xmltree"
)

// ValidateKeys checks each sibling group independently, including groups that
// have not yet been matched across documents.
func ValidateKeys(rule *Rule, sets ...[]*xmltree.Node) error {
	for _, nodes := range sets {
		seen := map[string]bool{}
		for _, n := range nodes {
			key, err := rule.Key(n)
			if err != nil {
				return err
			}
			if seen[key] {
				return fmt.Errorf("значения ключа повторяются среди соседних узлов")
			}
			seen[key] = true
		}
	}
	return nil
}

// Infer proposes a deterministic key and verifies it over every supplied group.
func Infer(sets ...[]*xmltree.Node) *Rule {
	var sample *xmltree.Node
	for _, s := range sets {
		if len(s) > 0 {
			sample = s[0]
			break
		}
	}
	if sample == nil {
		return nil
	}
	var candidates []Rule
	attrs := append([]xmltree.Attribute(nil), sample.Attributes...)
	rank := func(s string) int {
		switch strings.ToLower(s) {
		case "uuid":
			return 0
		case "id":
			return 1
		case "key":
			return 2
		case "name":
			return 3
		}
		return 4
	}
	sort.SliceStable(attrs, func(i, j int) bool { return rank(attrs[i].Name.Local) < rank(attrs[j].Name.Local) })
	var af []Field
	for _, a := range attrs {
		if a.Name.URI == "xmlns" || a.LexicalName == "xmlns" {
			continue
		}
		f := Field{Name: a.Name.Local, Namespace: a.Name.URI}
		af = append(af, f)
		candidates = append(candidates, Rule{Mode: "attribute", Fields: []Field{f}})
	}
	var ef []Field
	for _, c := range sample.Children() {
		if len(c.Children()) == 0 {
			f := Field{Name: c.Name.Local, Namespace: c.Name.URI}
			ef = append(ef, f)
			candidates = append(candidates, Rule{Mode: "element", Fields: []Field{f}})
		}
	}
	for _, group := range []struct {
		mode   string
		fields []Field
	}{{"attribute", af}, {"element", ef}} {
		if len(group.fields) > 12 {
			continue
		}
		for i := range group.fields {
			for j := i + 1; j < len(group.fields); j++ {
				candidates = append(candidates, Rule{Mode: group.mode, Fields: []Field{group.fields[i], group.fields[j]}})
			}
		}
	}
	if len(sample.Children()) == 0 {
		candidates = append(candidates, Rule{Mode: "text"})
	}
	for _, c := range candidates {
		if ValidateKeys(&c, sets...) == nil {
			return &c
		}
	}
	return nil
}

func Clone(db *Database) *Database {
	out := New()
	out.Version = db.Version
	for _, p := range db.Profiles {
		cp := Profile{Root: p.Root, Namespace: p.Namespace}
		for _, e := range p.KnownElements {
			e.Attributes = append([]Field(nil), e.Attributes...)
			e.Elements = append([]Field(nil), e.Elements...)
			cp.KnownElements = append(cp.KnownElements, e)
		}
		for _, r := range p.Rules {
			r.Fields = append([]Field(nil), r.Fields...)
			cp.Rules = append(cp.Rules, r)
		}
		out.Profiles = append(out.Profiles, cp)
	}
	return out
}

func (db *Database) Put(root xmltree.Name, rule Rule) {
	if rule.Selector != "" || rule.TrimSpace || root.URI == "*" {
		db.Version = 2
	}
	p := db.Profile(root)
	if p == nil || p.Namespace != root.URI {
		created := Profile{Root: root.Local, Namespace: root.URI}
		if p != nil {
			created.Rules = append([]Rule(nil), p.Rules...)
			created.KnownElements = append([]KnownElement(nil), p.KnownElements...)
		}
		db.Profiles = append(db.Profiles, created)
		p = &db.Profiles[len(db.Profiles)-1]
	}
	for i := range p.Rules {
		if p.Rules[i].Path == rule.Path && p.Rules[i].Selector == rule.Selector {
			p.Rules[i] = rule
			return
		}
	}
	p.Rules = append(p.Rules, rule)
}

// Overlay gives exact team paths priority and never mutates either source.
func Overlay(team, personal *Database) *Database {
	out := Clone(personal)
	if team.Version > out.Version {
		out.Version = team.Version
	}
	for _, p := range team.Profiles {
		// Preserve the observed element catalog, including profiles with no keys.
		target := out.Profile(xmltree.Name{URI: p.Namespace, Local: p.Root})
		if target == nil || target.Namespace != p.Namespace {
			out.Profiles = append(out.Profiles, Profile{Root: p.Root, Namespace: p.Namespace})
			target = &out.Profiles[len(out.Profiles)-1]
		}
		for _, e := range p.KnownElements {
			found := false
			for i, old := range target.KnownElements {
				if old.Name == e.Name && old.Namespace == e.Namespace {
					target.KnownElements[i] = e
					found = true
					break
				}
			}
			if !found {
				target.KnownElements = append(target.KnownElements, e)
			}
		}
		for _, r := range p.Rules {
			out.Put(xmltree.Name{URI: p.Namespace, Local: p.Root}, r)
		}
	}
	return out
}
