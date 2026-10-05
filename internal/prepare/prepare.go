// Package prepare verifies rules before change comparison can begin.
package prepare

import (
	"fmt"
	"sort"

	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

type Set struct {
	Side  string
	Nodes []*xmltree.Node
}
type Group struct {
	Selector      string
	Path          string
	Name          string
	Depth         int
	Counts        [3]int
	Repeated      bool
	IsRoot        bool
	Status        string
	Message       string
	Rule          *rules.Rule
	InheritedFrom string
	Attributes    []rules.Field
	Elements      []rules.Field
	Sets          []Set `json:"-"`
}
type Report struct {
	Ready    bool
	Missing  int
	Groups   []*Group
	Learned  []rules.Rule
	Database *rules.Database `json:"-"`
}

func Analyze(docs []*xmltree.Document, db *rules.Database, auto bool) (*Report, error) {
	if len(docs) != 3 {
		return nil, fmt.Errorf("нужны base, local и remote")
	}
	root := docs[0].Root.Name
	for _, d := range docs {
		if d.Root.Name != root {
			return nil, fmt.Errorf("корневые элементы XML различаются")
		}
	}
	if err := db.Validate(); err != nil {
		return nil, err
	}
	r := &Report{Ready: true, Database: rules.Clone(db)}
	byPath := map[string]*Group{}
	rootGroup := &Group{Path: rules.Path(docs[0].Root), Name: docs[0].Root.LexicalName, Selector: root.String(), IsRoot: true}
	byPath[rootGroup.Path] = rootGroup
	for side, d := range docs {
		rootGroup.Counts[side] = 1
		rootGroup.Sets = append(rootGroup.Sets, Set{Side: []string{"base", "local", "remote"}[side], Nodes: []*xmltree.Node{d.Root}})
		var walk func(*xmltree.Node, int)
		walk = func(parent *xmltree.Node, depth int) {
			groups := map[xmltree.Name][]*xmltree.Node{}
			for _, n := range parent.Children() {
				groups[n.Name] = append(groups[n.Name], n)
			}
			for _, nodes := range groups {
				path := rules.Path(nodes[0])
				g := byPath[path]
				if g == nil {
					g = &Group{Path: path, Name: nodes[0].LexicalName, Selector: nodes[0].Name.String(), Depth: depth}
					byPath[path] = g
				}
				g.Counts[side] += len(nodes)
				g.Repeated = g.Repeated || len(nodes) > 1
				g.Sets = append(g.Sets, Set{Side: []string{"base", "local", "remote"}[side], Nodes: nodes})
			}
			for _, n := range parent.Children() {
				walk(n, depth+1)
			}
		}
		walk(d.Root, 1)
	}
	for _, g := range byPath {
		r.Groups = append(r.Groups, g)
	}
	sort.Slice(r.Groups, func(i, j int) bool {
		a, b := r.Groups[i], r.Groups[j]
		if a.Depth != b.Depth {
			return a.Depth < b.Depth
		}
		return a.Path < b.Path
	})
	setsByName := map[xmltree.Name][][]*xmltree.Node{}
	for _, g := range r.Groups {
		for _, set := range g.Sets {
			if len(set.Nodes) > 0 {
				name := set.Nodes[0].Name
				setsByName[name] = append(setsByName[name], set.Nodes)
			}
		}
	}
	for _, g := range r.Groups {
		var sets [][]*xmltree.Node
		attrs, elems := map[xmltree.Name]bool{}, map[xmltree.Name]bool{}
		for _, s := range g.Sets {
			sets = append(sets, s.Nodes)
			for _, n := range s.Nodes {
				for _, a := range n.Attributes {
					if a.Name.URI != "xmlns" && a.LexicalName != "xmlns" {
						attrs[a.Name] = true
					}
				}
				for _, c := range n.Children() {
					if len(c.Children()) == 0 {
						elems[c.Name] = true
					}
				}
			}
		}
		fields := func(m map[xmltree.Name]bool) []rules.Field {
			out := []rules.Field{}
			for n := range m {
				out = append(out, rules.Field{Name: n.Local, Namespace: n.URI})
			}
			sort.Slice(out, func(i, j int) bool {
				if out[i].Namespace != out[j].Namespace {
					return out[i].Namespace < out[j].Namespace
				}
				return out[i].Name < out[j].Name
			})
			return out
		}
		g.Attributes, g.Elements = fields(attrs), fields(elems)
		n := sets[0][0]
		rule := r.Database.Profile(root).Effective(n)
		if rule != nil {
			copyRule := *rule
			copyRule.Fields = append([]rules.Field(nil), rule.Fields...)
			// The editor knows actual namespace URIs and can replace imported
			// lexical references with stable expanded names on explicit save.
			for i, f := range copyRule.Fields {
				if !f.Lexical {
					continue
				}
				if rule.Mode == "attribute" {
					for _, a := range n.Attributes {
						if a.LexicalName == f.Name {
							copyRule.Fields[i] = rules.Field{Name: a.Name.Local, Namespace: a.Name.URI, TrimSpace: f.TrimSpace}
							break
						}
					}
				} else if rule.Mode == "element" {
					for _, c := range n.Children() {
						if c.LexicalName == f.Name {
							copyRule.Fields[i] = rules.Field{Name: c.Name.Local, Namespace: c.Name.URI, TrimSpace: f.TrimSpace}
							break
						}
					}
				}
			}
			g.Rule = &copyRule
			if rule.Selector != "" {
				g.Message = "Импортированная настройка по имени элемента: " + rule.Selector
			}
			if rule.Selector == "" && rule.Path != g.Path {
				g.InheritedFrom = rule.Path
			}
		}
		explicit := rule.AppliesDirectly(n)
		// The document root is known from the profile; its rule supplies defaults.
		if g.IsRoot {
			g.Status = "structural"
			g.Message = "Корень определен именем и пространством имен"
			continue
		}
		if rule != nil && rules.ValidateKeys(rule, sets...) == nil {
			g.Status = "ready"
			continue
		}
		// A structural fallback must never override an explicit rule for this path.
		if !g.Repeated && !explicit {
			g.Status = "structural"
			g.Message = "Единственный элемент этого имени внутри родителя"
			continue
		}
		// Never silently replace an explicit rule; a missing/inapplicable inherited
		// rule may receive its own automatically inferred override.
		if auto && !explicit {
			// A name rule must work at every observed depth, not only this path.
			if inferred := rules.Infer(setsByName[n.Name]...); inferred != nil {
				inferred.Selector = n.Name.String()
				inferred.Origin = "automatic"
				inferred.Order = "significant"
				if rule != nil {
					inferred.Order = rule.Order
				}
				if profile := r.Database.Profile(root); profile != nil {
					for _, common := range profile.Rules {
						if common.Selector == "*" {
							inferred.Order = "default"
							break
						}
					}
				}
				r.Database.Put(root, *inferred)
				r.Learned = append(r.Learned, *inferred)
				g.Rule = inferred
				g.InheritedFrom = ""
				g.Status = "ready"
				continue
			}
		}
		g.Status = "missing"
		g.Message = "Не удалось автоматически подобрать однозначное правило"
		if rule != nil {
			g.Message = rules.ValidateKeys(rule, sets...).Error()
		}
		r.Missing++
		r.Ready = false
	}
	return r, nil
}

type KeyRow struct {
	Side      string
	Group     int
	Index     int
	Key       string
	Error     string
	Duplicate bool
}
type Preview struct {
	Valid bool
	Rows  []KeyRow
	Error string
	Total int
}

func PreviewRule(g *Group, rule rules.Rule) Preview {
	p := Preview{Valid: true, Rows: []KeyRow{}}
	if g.IsRoot {
		p.Total = len(g.Sets)
		return p
	}
	for gi, s := range g.Sets {
		counts := map[string]int{}
		for _, n := range s.Nodes {
			if k, err := rule.Key(n); err == nil {
				counts[k]++
			}
		}
		for i, n := range s.Nodes {
			row := KeyRow{Side: s.Side, Group: gi + 1, Index: i + 1}
			p.Total++
			key, err := rule.Key(n)
			if err != nil {
				row.Error = err.Error()
				p.Valid = false
			} else {
				row.Key = key
				row.Duplicate = counts[key] > 1
				if row.Duplicate {
					p.Valid = false
				}
			}
			if len(p.Rows) < 120 {
				p.Rows = append(p.Rows, row)
			}
		}
	}
	if !p.Valid {
		p.Error = "Есть отсутствующие значения или повторяющиеся ключи. Правило пока не подходит для этой группы."
	}
	return p
}
