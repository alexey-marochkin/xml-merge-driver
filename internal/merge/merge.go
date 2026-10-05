// Package merge implements conservative structural three-way merging.
package merge

import (
	"fmt"
	"sort"
	"strings"

	"xmlmerge/internal/prepare"
	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

type Conflict struct {
	Path, Reason, TargetID string
	// RelatedIDs narrows a branch-level conflict to the actual affected nodes.
	// TargetID remains the common branch used for navigation and resolution.
	RelatedIDs []string `json:",omitempty"`
}
type Result struct {
	Data        []byte
	PreviewData []byte // Never save this while Conflicts is nonempty.
	Conflicts   []Conflict
	Learned     []rules.Rule
	NeedsRules  bool
}
type engine struct {
	profile    *rules.Profile
	conflicts  []Conflict
	learned    []rules.Rule
	structural map[string]bool
	moveReason string
	moveNode   *xmltree.Node
}

func Merge(base, local, remote []byte, db *rules.Database) (*Result, error) {
	return MergeWithPendingConflicts(base, local, remote, db, nil)
}

// MergeWithPendingConflicts preserves unresolved interactive move endpoints.
// Editing one endpoint can hide a move from detection (for example by making
// its key appear twice); that must not accept the other endpoint implicitly.
func MergeWithPendingConflicts(base, local, remote []byte, db *rules.Database, pending []Conflict) (*Result, error) {
	b, err := xmltree.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	l, err := xmltree.Parse(local)
	if err != nil {
		return nil, fmt.Errorf("local: %w", err)
	}
	r, err := xmltree.Parse(remote)
	if err != nil {
		return nil, fmt.Errorf("remote: %w", err)
	}
	if b.Root.Name != l.Root.Name || b.Root.Name != r.Root.Name {
		return &Result{Conflicts: []Conflict{{Path: "/", Reason: "root elements differ", TargetID: "root"}}}, nil
	}
	preparation, err := prepare.Analyze([]*xmltree.Document{b, l, r}, db, true)
	if err != nil {
		return nil, err
	}
	if !preparation.Ready {
		result := &Result{NeedsRules: true, Learned: preparation.Learned}
		for _, g := range preparation.Groups {
			if g.Status == "missing" {
				result.Conflicts = append(result.Conflicts, Conflict{Path: g.Path, Reason: g.Message})
			}
		}
		return result, nil
	}
	p := preparation.Database.Profile(l.Root.Name)
	// A private rule snapshot isolates inference and document editing from the caller.
	profile := &rules.Profile{Root: l.Root.Name.Local, Namespace: l.Root.Name.URI}
	if p != nil {
		profile.Rules = append([]rules.Rule(nil), p.Rules...)
	}
	e := &engine{profile: profile, learned: preparation.Learned, structural: map[string]bool{}}
	for _, g := range preparation.Groups {
		if g.Status == "structural" {
			e.structural[g.Path] = true
		}
	}
	// Cross-parent moves require a global merge of identities, which is a later
	// stage. Recognizable moves must not be mistaken for unrelated delete/add.
	e.crossParentMove(b.Root, l.Root)
	e.crossParentMove(b.Root, r.Root)
	e.node(b.Root, l.Root, r.Root)
	for _, c := range pending {
		found := false
		for i := range e.conflicts {
			current := &e.conflicts[i]
			if current.TargetID != c.TargetID || (current.Reason != c.Reason && !(len(current.RelatedIDs) > 0 && len(c.RelatedIDs) > 0 && strings.HasPrefix(current.Reason, "Перемещение между родителями:") && strings.HasPrefix(c.Reason, "Перемещение между родителями:"))) {
				continue
			}
			current.Reason = c.Reason
			seen := map[string]bool{}
			for _, id := range current.RelatedIDs {
				seen[id] = true
			}
			for _, id := range c.RelatedIDs {
				if !seen[id] {
					current.RelatedIDs = append(current.RelatedIDs, id)
					seen[id] = true
				}
			}
			found = true
			break
		}
		if !found {
			e.conflicts = append(e.conflicts, c)
		}
	}
	// Prolog/epilog modifications are not silently discarded.
	for _, side := range []*xmltree.Document{b, r} {
		if outer(side) != outer(l) {
			e.conflict(l.Root, "different XML prolog/epilog; manual resolution required")
			break
		}
	}
	result := &Result{Conflicts: e.conflicts, Learned: e.learned}
	if len(e.conflicts) > 0 {
		result.PreviewData = e.previewBaseConflicts(b, l)
		return result, nil
	}
	result.Data, err = l.Bytes()
	if err != nil {
		return nil, err
	}
	parsed, err := xmltree.Parse(result.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid merged result: %w", err)
	}
	if !sameExpandedNames(l.Root, parsed.Root) {
		return &Result{Conflicts: []Conflict{{Path: rules.Path(l.Root), Reason: "namespace context changed in transferred XML; manual resolution required", TargetID: "root"}}, Learned: e.learned}, nil
	}
	return result, nil
}

func (e *engine) crossParentMove(base, side *xmltree.Node) {
	type location struct {
		parent string
		count  int
		node   *xmltree.Node
	}
	collect := func(root *xmltree.Node) map[string]location {
		out := map[string]location{}
		var visit func(*xmltree.Node, string)
		visit = func(n *xmltree.Node, parent string) {
			identity := n.Name.String()
			if rule := e.profile.Effective(n); rule != nil {
				if key, err := rule.Key(n); err == nil {
					identity = key
				}
			} else {
				for _, field := range []string{"uuid", "id"} {
					if value, ok := n.Attribute(xmltree.Name{Local: field}); ok {
						identity += fmt.Sprintf("[%s=%q]", field, value)
						break
					}
				}
			}
			if n != root {
				loc := out[identity]
				loc.count++
				loc.parent = parent
				loc.node = n
				out[identity] = loc
			}
			for _, c := range n.Children() {
				visit(c, parent+"/"+identity)
			}
		}
		visit(root, "")
		return out
	}
	b, s := collect(base), collect(side)
	keys := make([]string, 0, len(b))
	for key := range b {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		before := b[key]
		if after, ok := s[key]; ok && before.count == 1 && after.count == 1 && before.parent != after.parent {
			e.moveReason = key
			e.moveNode = base
			// Resolve the smallest shared containing branch, not the document root.
			var a, z []*xmltree.Node
			for n := before.node.Parent; n != nil; n = n.Parent {
				a = append(a, n)
			}
			for n := after.node.Parent; n != nil; n = n.Parent {
				z = append(z, n)
			}
			for i, j := len(a)-1, len(z)-1; i >= 0 && j >= 0; i, j = i-1, j-1 {
				if NodeID(a[i], e.profile, e.structural) != NodeID(z[j], e.profile, e.structural) {
					break
				}
				e.moveNode = a[i]
			}
			target := NodeID(e.moveNode, e.profile, e.structural)
			related := []string{NodeID(before.node, e.profile, e.structural), NodeID(after.node, e.profile, e.structural)}
			found := false
			for i := range e.conflicts {
				c := &e.conflicts[i]
				if c.TargetID == target && strings.HasPrefix(c.Reason, "Перемещение между родителями:") {
					c.RelatedIDs = append(c.RelatedIDs, related...)
					if !strings.Contains(c.Reason, key) {
						c.Reason += "; " + key
					}
					found = true
					break
				}
			}
			if !found {
				e.conflict(e.moveNode, "Перемещение между родителями: "+key)
				e.conflicts[len(e.conflicts)-1].RelatedIDs = related
			}
		}
	}
}

// A copied prefix can resolve to a different URI under its new parent. Validate
// the serialized DOM against intended expanded names, not just well-formedness.
func sameExpandedNames(a, b *xmltree.Node) bool {
	if a.Name != b.Name || len(a.Attributes) != len(b.Attributes) {
		return false
	}
	for i := range a.Attributes {
		if a.Attributes[i].Name != b.Attributes[i].Name {
			return false
		}
	}
	ac, bc := a.Children(), b.Children()
	if len(ac) != len(bc) {
		return false
	}
	for i := range ac {
		if !sameExpandedNames(ac[i], bc[i]) {
			return false
		}
	}
	return true
}

func outer(d *xmltree.Document) string {
	var b strings.Builder
	for _, p := range d.Parts {
		if p.Element == nil && p.Kind != "instruction" {
			b.WriteString(p.Raw)
		} else if p.Kind == "instruction" && !strings.HasPrefix(p.Raw, "<?xml ") {
			b.WriteString(p.Raw)
		}
	}
	return b.String()
}

func (e *engine) conflict(n *xmltree.Node, why string) {
	e.conflicts = append(e.conflicts, Conflict{Path: rules.Path(n), Reason: why, TargetID: NodeID(n, e.profile, e.structural)})
}

func groups(n *xmltree.Node) map[xmltree.Name][]*xmltree.Node {
	g := map[xmltree.Name][]*xmltree.Node{}
	for _, c := range n.Children() {
		g[c.Name] = append(g[c.Name], c)
	}
	return g
}

func names(gs ...map[xmltree.Name][]*xmltree.Node) []xmltree.Name {
	set := map[xmltree.Name]bool{}
	for _, g := range gs {
		for n := range g {
			set[n] = true
		}
	}
	var out []xmltree.Name
	for n := range set {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func index(nodes []*xmltree.Node, rule *rules.Rule) (map[string]*xmltree.Node, []string, error) {
	out := map[string]*xmltree.Node{}
	var order []string
	for _, n := range nodes {
		key := n.Name.String()
		if rule != nil {
			var err error
			key, err = rule.Key(n)
			if err != nil {
				return nil, nil, err
			}
		} else if len(nodes) > 1 {
			return nil, nil, fmt.Errorf("repeated elements require an identification rule")
		}
		if out[key] != nil {
			return nil, nil, fmt.Errorf("duplicate identification key")
		}
		out[key] = n
		order = append(order, key)
	}
	return out, order, nil
}

func attrs(n *xmltree.Node) map[xmltree.Name]xmltree.Attribute {
	m := map[xmltree.Name]xmltree.Attribute{}
	for _, a := range n.Attributes {
		m[a.Name] = a
	}
	return m
}
func attrEqual(a, b map[xmltree.Name]xmltree.Attribute, k xmltree.Name) bool {
	av, aok := a[k]
	bv, bok := b[k]
	return aok == bok && av.Value == bv.Value
}

func (e *engine) node(b, l, r *xmltree.Node) {
	remoteXML := r.XML()
	if b.XML() == remoteXML || l.XML() == remoteXML {
		return
	}
	if b.Name != l.Name || b.Name != r.Name {
		e.conflict(l, "element type changed")
		return
	}
	ba, la, ra := attrs(b), attrs(l), attrs(r)
	keys := map[xmltree.Name]bool{}
	for k := range ba {
		keys[k] = true
	}
	for k := range la {
		keys[k] = true
	}
	for k := range ra {
		keys[k] = true
	}
	// Existing local slots remain in place; new remote attributes follow remote order.
	var ordered []xmltree.Name
	for _, a := range l.Attributes {
		ordered = append(ordered, a.Name)
		delete(keys, a.Name)
	}
	for _, a := range r.Attributes {
		if keys[a.Name] {
			ordered = append(ordered, a.Name)
			delete(keys, a.Name)
		}
	}
	for _, a := range b.Attributes {
		if keys[a.Name] {
			ordered = append(ordered, a.Name)
			delete(keys, a.Name)
		}
	}
	for _, k := range ordered {
		if attrEqual(la, ra, k) || attrEqual(ba, ra, k) {
			continue
		}
		if !attrEqual(ba, la, k) {
			e.conflict(l, "attribute changed differently: "+k.String())
			continue
		}
		if k.URI == "xmlns" || k.Local == "xmlns" {
			e.conflict(l, "namespace declaration changed; manual resolution required")
			continue
		}
		if a, ok := ra[k]; ok {
			l.ReplaceAttribute(k, &a)
		} else {
			l.ReplaceAttribute(k, nil)
		}
	}
	if simple(b) && simple(l) && simple(r) {
		e.children(b, l, r)
		return
	}
	bc, lc, rc := xmltree.PartsXML(b.Parts), xmltree.PartsXML(l.Parts), xmltree.PartsXML(r.Parts)
	if lc == rc || bc == rc {
		return
	}
	if bc == lc {
		l.SetParts(r.Parts)
	} else {
		e.conflict(l, "text, CDATA or mixed content changed differently")
	}
}

func simple(n *xmltree.Node) bool {
	for _, p := range n.Parts {
		if p.Element == nil && (p.Kind != "text" || strings.TrimSpace(p.Text) != "") {
			return false
		}
	}
	return true
}

func (e *engine) children(b, l, r *xmltree.Node) {
	bg, lg, rg := groups(b), groups(l), groups(r)
	lookup := map[*xmltree.Node]string{}
	bm, lm, rm := map[string]*xmltree.Node{}, map[string]*xmltree.Node{}, map[string]*xmltree.Node{}
	for _, name := range names(bg, lg, rg) {
		all := append(append(append([]*xmltree.Node{}, bg[name]...), lg[name]...), rg[name]...)
		rule := e.profile.Effective(all[0])
		if e.structural[rules.Path(all[0])] {
			rule = nil
		}
		for i, set := range [][]*xmltree.Node{bg[name], lg[name], rg[name]} {
			m, _, err := index(set, rule)
			if err != nil {
				e.conflict(l, err.Error()+": "+name.String())
				return
			}
			target := []map[string]*xmltree.Node{bm, lm, rm}[i]
			for key, n := range m {
				target[key] = n
				lookup[n] = key
			}
		}
	}
	// Missing old keys plus new keys may be a key change, not delete/add.
	for _, side := range []map[string]*xmltree.Node{lm, rm} {
		for key, bn := range bm {
			if side[key] == nil {
				if rule := e.profile.Effective(bn); rule != nil && rule.AllowDeleteAdd {
					continue
				}
				for newKey, newNode := range side {
					if bm[newKey] == nil && bn.Name == newNode.Name {
						e.conflict(l, "possible identity change; explicit correspondence required: "+bn.Name.String())
						return
					}
				}
			}
		}
	}
	kept := map[string]*xmltree.Node{}
	for key, bn := range bm {
		ln, rn := lm[key], rm[key]
		switch {
		case ln == nil && rn == nil:
		case ln == nil:
			if bn.XML() != rn.XML() {
				e.conflict(rn, "local deletion conflicts with remote edit")
			}
		case rn == nil:
			if bn.XML() != ln.XML() {
				e.conflict(ln, "remote deletion conflicts with local edit")
			}
		default:
			e.node(bn, ln, rn)
			kept[key] = ln
		}
	}
	for key, ln := range lm {
		if bm[key] == nil {
			if rn := rm[key]; rn != nil && ln.XML() != rn.XML() {
				e.conflict(ln, "different additions share an identity")
			}
			kept[key] = ln
		}
	}
	for key, rn := range rm {
		if bm[key] == nil && lm[key] == nil {
			kept[key] = rn
		}
	}
	seq := func(n *xmltree.Node) []string {
		var s []string
		for _, c := range n.Children() {
			if kept[lookup[c]] != nil {
				s = append(s, lookup[c])
			}
		}
		return s
	}
	bs, ls, rs := seq(b), seq(l), seq(r)
	order := append([]string(nil), ls...)
	significant := true
	if rule := e.profile.Effective(l); rule != nil {
		significant = rule.Order != "insignificant"
	}
	if significant {
		// Compare relative order of survivors shared by all three versions.
		common := map[string]bool{}
		for _, k := range bs {
			if lm[k] != nil && rm[k] != nil {
				common[k] = true
			}
		}
		filter := func(s []string) string {
			var out []string
			for _, k := range s {
				if common[k] {
					out = append(out, k)
				}
			}
			return strings.Join(out, "\x00")
		}
		bo, lo, ro := filter(bs), filter(ls), filter(rs)
		if lo != bo && ro != bo && lo != ro {
			e.conflict(l, "incompatible child order")
			return
		}
		if lo == bo && ro != bo {
			order = append([]string(nil), rs...)
		}
		// A shared newly added node must not be placed incompatibly by both sides.
		lp, rp := map[string]int{}, map[string]int{}
		for i, k := range ls {
			lp[k] = i
		}
		for i, k := range rs {
			rp[k] = i
		}
		for _, k := range ls {
			if bm[k] != nil {
				continue
			}
			if _, ok := rp[k]; !ok {
				continue
			}
			for other, li := range lp {
				ri, ok := rp[other]
				if ok && (lp[k] < li) != (rp[k] < ri) {
					e.conflict(l, "incompatible position of a shared addition")
					return
				}
			}
		}
	}
	order = insertMissing(order, rs)
	order = insertMissing(order, ls)
	// Keep each existing local element's immediately preceding whitespace.
	prefix := map[string][]xmltree.Part{}
	var pending []xmltree.Part
	for _, p := range l.Parts {
		if p.Element == nil {
			pending = append(pending, p)
		} else {
			prefix[lookup[p.Element]] = pending
			pending = nil
		}
	}
	remotePrefix := map[string][]xmltree.Part{}
	var rp []xmltree.Part
	for _, p := range r.Parts {
		if p.Element == nil {
			rp = append(rp, p)
		} else {
			remotePrefix[lookup[p.Element]] = rp
			rp = nil
		}
	}
	var parts []xmltree.Part
	for _, key := range order {
		n := kept[key]
		if n == nil {
			continue
		}
		pre, ok := prefix[key]
		if !ok {
			pre = remotePrefix[key]
		}
		parts = append(parts, pre...)
		parts = append(parts, xmltree.Part{Kind: "element", Element: n})
	}
	parts = append(parts, pending...)
	l.SetParts(parts)
}

func insertMissing(order, incoming []string) []string {
	present := map[string]bool{}
	for _, k := range order {
		present[k] = true
	}
	for i, key := range incoming {
		if present[key] {
			continue
		}
		positions := map[string]int{}
		for j, k := range order {
			positions[k] = j
		}
		if _, ok := positions[key]; ok {
			continue
		}
		at := len(order)
		anchored := false
		for j := i - 1; j >= 0; j-- {
			if p, ok := positions[incoming[j]]; ok {
				at = p + 1
				anchored = true
				break
			}
		}
		if !anchored {
			for j := i + 1; j < len(incoming); j++ {
				if p, ok := positions[incoming[j]]; ok {
					at = p
					break
				}
			}
		}
		order = append(order, "")
		copy(order[at+1:], order[at:])
		order[at] = key
		present[key] = true
	}
	return order
}
