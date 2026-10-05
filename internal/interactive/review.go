package interactive

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"xmlmerge/internal/merge"
	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

type TreeValue struct {
	Attributes map[string]string
	Text       string
}
type TreeRow struct {
	Origin                                   [2]bool
	HasDecision                              bool
	ID, Parent, Name, Key                    string
	Values                                   [3]*TreeValue
	HasChildren, Changed, Conflict, Resolved bool
	Children                                 []string
	Result                                   *TreeValue
	Take                                     [2]bool
}
type reviewNode struct {
	row   TreeRow
	nodes [3]*xmltree.Node
}
type NodeDecision struct{ ID, Choice, XML string }
type NodeDetail struct {
	ID, Name      string
	XML           [3]string
	Present       [3]bool
	Decision      *NodeDecision
	Editable      bool
	ResultXML     string
	ResultPresent bool
	ConflictLines [3][][2]int
}

func (s *Session) buildReview() {
	s.nodes = map[string]*reviewNode{}
	s.nodeOrder = nil
	if s.decisions == nil {
		s.decisions = map[string]NodeDecision{}
	}
	structural := map[string]bool{}
	for _, g := range s.report.Groups {
		if g.Status == "structural" {
			structural[g.Path] = true
		}
	}
	profile := s.db.Profile(s.docs[0].Root.Name)
	// Union in document order, preserving identifiers shared by all three inputs.
	for side, doc := range s.docs {
		var visit func(*xmltree.Node, string)
		visit = func(n *xmltree.Node, parent string) {
			id := merge.NodeID(n, profile, structural)
			v := s.nodes[id]
			if v == nil {
				label := n.LexicalName
				for _, a := range n.Attributes {
					if a.Name.Local == "name" {
						label += " · " + a.Value
						break
					}
				}
				for _, c := range n.Children() {
					if c.Name.Local == "Properties" {
						for _, p := range c.Children() {
							if p.Name.Local == "Name" {
								label += " · " + p.DirectText()
							}
						}
					}
				}
				v = &reviewNode{row: TreeRow{ID: id, Parent: parent, Name: label, Key: rules.Path(n)}}
				s.nodes[id] = v
				if parent != "" {
					p := s.nodes[parent]
					p.row.Children = append(p.row.Children, id)
					p.row.HasChildren = true
				}
			}
			v.nodes[side] = n
			// Whitespace between elements is layout, not a value to display.
			text := n.DirectText()
			if strings.TrimSpace(text) == "" {
				text = ""
			}
			v.row.Values[side] = treeValue(n)
			for _, c := range n.Children() {
				visit(c, id)
			}
		}
		visit(doc.Root, "")
	}
	var walk func(string)
	walk = func(id string) {
		v := s.nodes[id]
		s.nodeOrder = append(s.nodeOrder, id)
		v.row.Changed = nodeSurface(v.nodes[0]) != nodeSurface(v.nodes[1]) || nodeSurface(v.nodes[0]) != nodeSurface(v.nodes[2])
		for _, c := range v.row.Children {
			walk(c)
		}
	}
	walk("root")
}

func nodeSurface(n *xmltree.Node) string {
	if n == nil {
		return "absent"
	}
	var a []string
	for _, v := range n.Attributes {
		a = append(a, v.Name.String()+"="+v.Value)
	}
	sort.Strings(a)
	text := n.DirectText()
	if strings.TrimSpace(text) == "" {
		text = ""
	}
	for _, p := range n.Parts {
		if p.Element == nil && p.Kind != "text" && p.Kind != "cdata" {
			text += "\x00" + p.Raw
		}
	}
	// Includes child order so a pure reorder has a navigation destination.
	var children []string
	for _, c := range n.Children() {
		children = append(children, c.Opening())
	}
	return n.Name.String() + "\x00" + strings.Join(a, "\x00") + "\x01" + text + "\x01" + strings.Join(children, "\x00")
}
func (s *Session) changeCount() int {
	n := 0
	for _, v := range s.nodes {
		if v.row.Changed {
			n++
		}
	}
	return n
}
func (s *Session) row(id string) TreeRow {
	row := s.nodes[id].row
	_, row.HasDecision = s.decisions[id]
	if n := s.center[id]; n != nil {
		row.Result = treeValue(n)
	}
	resultSignature := reviewSignature(s.center[id])
	changed := resultSignature != reviewSignature(s.nodes[id].nodes[0])
	for i, side := range []int{1, 2} {
		row.Take[i] = reviewSignature(s.nodes[id].nodes[side]) != resultSignature
		row.Origin[i] = changed && !row.Take[i]
	}
	if d, ok := s.decisions[id]; ok {
		if d.Choice == "local" {
			row.Origin = [2]bool{true, false}
		}
		if d.Choice == "remote" {
			row.Origin = [2]bool{false, true}
		}
	}
	for n := s.nodes[id]; n != nil; n = s.nodes[n.row.Parent] {
		if _, ok := s.decisions[n.row.ID]; ok {
			row.Resolved = true
			break
		}
	}
	for _, c := range s.conflicts {
		if c.TargetID == id {
			row.Conflict = true
		}
	}
	return row
}
func (s *Session) Tree(parent string) ([]TreeRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != "compared" {
		return nil, fmt.Errorf("сначала выполните сравнение")
	}
	if parent == "" {
		return []TreeRow{s.row("root")}, nil
	}
	n := s.nodes[parent]
	if n == nil {
		return nil, fmt.Errorf("ветвь не найдена")
	}
	out := []TreeRow{}
	for _, id := range n.row.Children {
		out = append(out, s.row(id))
	}
	return out, nil
}
func (s *Session) Navigation() ([]TreeRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != "compared" {
		return nil, fmt.Errorf("сначала выполните сравнение")
	}
	out := []TreeRow{}
	for _, id := range s.nodeOrder {
		out = append(out, s.row(id))
	}
	return out, nil
}

// A portable fragment retains inherited namespaces, including QName-valued attributes.
func standalone(n *xmltree.Node) string {
	if n == nil {
		return ""
	}
	namespaces := map[string]xmltree.Attribute{}
	for p := n; p != nil; p = p.Parent {
		for _, a := range p.Attributes {
			if a.Name.URI == "xmlns" || a.LexicalName == "xmlns" {
				if _, ok := namespaces[a.LexicalName]; !ok {
					namespaces[a.LexicalName] = a
				}
			}
		}
	}
	copy := *n
	copy.Attributes = append([]xmltree.Attribute(nil), n.Attributes...)
	keys := []string{}
	for k := range namespaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := namespaces[k]
		copy.ReplaceAttribute(a.Name, &a)
	}
	return copy.XML()
}
func (s *Session) Node(id string) (NodeDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.nodes[id]
	if n == nil || s.phase != "compared" {
		return NodeDetail{}, fmt.Errorf("узел не найден")
	}
	out := NodeDetail{ID: id, Name: n.row.Name, Editable: true}
	if result := s.center[id]; result != nil {
		out.ResultXML = standalone(result)
		out.ResultPresent = true
	}
	for i, v := range n.nodes {
		out.Present[i] = v != nil
		out.XML[i] = standalone(v)

		if v != nil {
			fragment, _ := xmltree.Parse([]byte(out.XML[i]))
			clones := map[*xmltree.Node]*xmltree.Node{}
			var pair func(*xmltree.Node, *xmltree.Node)
			pair = func(a, b *xmltree.Node) {
				clones[a] = b
				ac, bc := a.Children(), b.Children()
				for j := range ac {
					pair(ac[j], bc[j])
				}
			}
			pair(v, fragment.Root)
			for _, c := range s.conflicts {
				ids := c.RelatedIDs
				if len(ids) == 0 {
					ids = []string{c.TargetID}
				}
				for _, id := range ids {
					if target := s.nodes[id]; target != nil {
						conflict := target.nodes[i]
						if within := clones[conflict]; within != nil {
							out.ConflictLines[i] = append(out.ConflictLines[i], [2]int{strings.Count(fragment.Source[:within.Start], "\n"), strings.Count(fragment.Source[:within.End], "\n") + 1})
						} else {
							for p := v; p != nil; p = p.Parent {
								if p == conflict {
									out.ConflictLines[i] = append(out.ConflictLines[i], [2]int{0, strings.Count(out.XML[i], "\n") + 1})
									break
								}
							}
						}
					}
				}
			}
		}
	}
	if d, ok := s.decisions[id]; ok {
		out.Decision = &d
	}
	for p := s.nodes[n.row.Parent]; p != nil; p = s.nodes[p.row.Parent] {

	}
	return out, nil
}
func (s *Session) Resolve(d NodeDecision) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() || s.phase != "compared" {
		return s.state(), fmt.Errorf("сначала выполните сравнение")
	}
	n := s.nodes[d.ID]
	if n == nil {
		return s.state(), fmt.Errorf("узел не найден")
	}
	taking := d.Choice == "take-local" || d.Choice == "take-remote"
	if taking {
		d.Choice = strings.TrimPrefix(d.Choice, "take-")
	}
	for p := s.nodes[n.row.Parent]; p != nil; p = s.nodes[p.row.Parent] {
		if _, ok := s.decisions[p.row.ID]; ok {
			if !taking && d.Choice != "xml" {
				return s.state(), fmt.Errorf("сначала отмените решение для родительского узла")
			}
			replacement := d.XML
			sourceSide := 1
			if taking {
				side := 1
				if d.Choice == "remote" {
					side = 2
				}
				replacement = standalone(n.nodes[side])
				sourceSide = side
			}
			var composed string
			var err error
			if s.center[p.row.ID] == nil {
				composed, err = s.restorePath(p.row.ID, d.ID, replacement, sourceSide)
			} else {
				composed, err = s.composeNode(p.row.ID, d.ID, replacement, sourceSide)
			}
			if err != nil {
				return s.state(), err
			}
			d = NodeDecision{ID: p.row.ID, Choice: "xml", XML: composed}
			n = p
			break
		}
	}
	old := s.decisions
	next := map[string]NodeDecision{}
	for k, v := range old {
		next[k] = v
	}
	if d.Choice == "reset" {
		delete(next, d.ID)
	} else {
		for _, v := range s.nodes {
			if _, ok := next[v.row.ID]; ok && v.row.ID != d.ID {
				for p := s.nodes[v.row.Parent]; p != nil; p = s.nodes[p.row.Parent] {
					if p.row.ID == d.ID {
						if d.Choice == "xml" || taking {
							delete(next, v.row.ID)
							break
						}
						return s.state(), fmt.Errorf("сначала отмените решения дочерних узлов")
					}
				}
			}
		}
		switch d.Choice {
		case "base", "local", "remote":
			i := map[string]int{"base": 0, "local": 1, "remote": 2}[d.Choice]
			d.XML = standalone(n.nodes[i])
		case "xml":
			if strings.TrimSpace(d.XML) == "" && d.ID != "root" {
				d.XML = ""
				break
			}
			doc, err := xmltree.Parse([]byte(d.XML))
			if err != nil {
				return s.state(), fmt.Errorf("ошибка XML узла: %w", err)
			}
			for _, part := range doc.Parts {
				if part.Element == nil && strings.TrimSpace(part.Raw) != "" {
					return s.state(), fmt.Errorf("введите один XML-узел без пролога или внешних комментариев")
				}
			}
			d.XML = doc.Root.XML()
		default:
			return s.state(), fmt.Errorf("неизвестное решение")
		}
		if d.ID == "root" && d.XML == "" {
			return s.state(), fmt.Errorf("нельзя удалить корень")
		}
		next[d.ID] = d
	}
	s.decisions = next
	_, err := s.recompare()
	if err != nil {
		s.decisions = old
		return s.state(), err
	}
	if !maps.Equal(old, next) {
		s.undo = append(s.undo, old)
		if len(s.undo) > 100 {
			s.undo[0] = nil
			s.undo = s.undo[1:]
		}
	}
	return s.state(), nil
}

// Undo restores decisions together with the automatic preview and conflicts.
func (s *Session) Undo() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() || s.phase != "compared" {
		return s.state(), fmt.Errorf("сначала выполните сравнение")
	}
	if len(s.undo) == 0 {
		return s.state(), nil
	}
	old := s.decisions
	s.decisions = s.undo[len(s.undo)-1]
	if _, err := s.recompare(); err != nil {
		s.decisions = old
		return s.state(), err
	}
	s.undo[len(s.undo)-1] = nil
	s.undo = s.undo[:len(s.undo)-1]
	return s.state(), nil
}

// Apply explicit subtree decisions to all three private inputs. The normal
// merge then preserves independent edits outside these scopes and checks again.
func (s *Session) recompare() (State, error) {
	inputs := s.input
	// Only materialize absent input containers on paths to explicit decisions.
	needed := map[string]bool{}
	preferred := map[string]int{}
	for _, id := range s.nodeOrder {
		d, ok := s.decisions[id]
		if !ok {
			continue
		}
		source := 1
		if d.Choice == "remote" {
			source = 2
		} else if d.Choice == "base" {
			source = 0
		} else if d.Choice == "xml" && s.nodes[id].nodes[1] == nil {
			source = 2
		}
		for n := s.nodes[id]; n != nil; n = s.nodes[n.row.Parent] {
			if !needed[n.row.ID] {
				preferred[n.row.ID] = source
			}
			needed[n.row.ID] = true
		}
	}
	centerIDs := map[*xmltree.Node]string{}
	for id, n := range s.nodes {
		for _, v := range n.nodes {
			if v != nil {
				centerIDs[v] = id
			}
		}
	}
	for id, n := range s.initialCenter {
		centerIDs[n] = id
	}
	for side := range inputs {
		doc, err := xmltree.Parse(s.input[side])
		if err != nil {
			return s.state(), err
		}
		clones := map[*xmltree.Node]*xmltree.Node{}
		var pair func(*xmltree.Node, *xmltree.Node)
		pair = func(a, b *xmltree.Node) {
			clones[a] = b
			ac, bc := a.Children(), b.Children()
			for i := range ac {
				pair(ac[i], bc[i])
			}
		}
		pair(s.docs[side].Root, doc.Root)
		fallback := map[string]*xmltree.Node{}
		minimal := map[string]bool{}
		identityNeeded := map[string]bool{}
		var pairCenter func(*xmltree.Node, *xmltree.Node, bool)
		pairCenter = func(a, b *xmltree.Node, prune bool) {
			if id, ok := centerIDs[a]; ok {
				fallback[id] = b
				minimal[id] = prune
			}
			if prune {
				for keyNode := range s.identityPathNodes(a) {
					identityNeeded[centerIDs[keyNode]] = true
				}
			}
			ac, bc := a.Children(), b.Children()
			for i := range ac {
				pairCenter(ac[i], bc[i], prune)
			}
		}
		var build func(string) (*xmltree.Node, error)
		build = func(id string) (*xmltree.Node, error) {
			if d, ok := s.decisions[id]; ok {
				if d.XML == "" {
					return nil, nil
				}
				v, err := xmltree.Parse([]byte(d.XML))
				if err != nil {
					return nil, err
				}
				return v.Root, nil
			}
			item := s.nodes[id]
			original := item.nodes[side]
			n := clones[original]
			if n == nil || minimal[id] {
				n = fallback[id]
			}
			if needed[id] && (n == nil || (s.initialCenter[id] == nil && !minimal[id])) {
				seed := s.initialCenter[id]
				prune := seed == nil
				if seed == nil {
					seed = s.sourceNode(id, preferred[id])
					if seed == nil {
						return nil, fmt.Errorf("не найден исходный родительский узел")
					}
				}
				copied, err := xmltree.Parse([]byte(standalone(seed)))
				if err != nil {
					return nil, err
				}
				// standalone adds inherited declarations for parsing. The scaffold is
				// reattached under its original ancestors, so keep only its own ones.
				declared := map[xmltree.Name]bool{}
				for _, a := range seed.Attributes {
					declared[a.Name] = true
				}
				for _, a := range append([]xmltree.Attribute(nil), copied.Root.Attributes...) {
					if (a.Name.URI == "xmlns" || a.LexicalName == "xmlns") && !declared[a.Name] {
						copied.Root.ReplaceAttribute(a.Name, nil)
					}
				}
				pairCenter(seed, copied.Root, prune)
				n = copied.Root
			}
			if n == nil {
				return nil, nil
			}
			byNode := map[*xmltree.Node]string{}
			for _, cid := range item.row.Children {
				if minimal[id] && fallback[cid] != nil {
					byNode[fallback[cid]] = cid
				} else if c := s.nodes[cid].nodes[side]; c != nil {
					byNode[clones[c]] = cid
				} else if c := fallback[cid]; c != nil {
					byNode[c] = cid
				}
			}
			parts := []xmltree.Part{}
			seen := map[string]bool{}
			for _, p := range n.Parts {
				if p.Element != nil {
					cid, ok := byNode[p.Element]
					if !ok {
						return nil, fmt.Errorf("не удалось сопоставить дочерний узел в результате")
					}
					seen[cid] = true
					if minimal[id] && !needed[cid] && !identityNeeded[cid] {
						continue
					}
					c, err := build(cid)
					if err != nil {
						return nil, err
					}
					if c == nil {
						continue
					}
					p.Element = c
					c.Parent = n
				}
				parts = append(parts, p)
			}
			for _, cid := range item.row.Children {
				if !seen[cid] && needed[cid] {
					c, err := build(cid)
					if err != nil {
						return nil, err
					}
					if c != nil {
						c.Parent = n
						parts = append(parts, xmltree.Part{Kind: "element", Element: c})
					}
				}
			}
			n.SetParts(parts)
			return n, nil
		}
		root, err := build("root")
		if err != nil {
			return s.state(), err
		}
		doc.Root = root
		for i := range doc.Parts {
			if doc.Parts[i].Element != nil {
				doc.Parts[i].Element = root
			}
		}
		inputs[side], err = doc.Bytes()
		if err != nil {
			return s.state(), err
		}
	}
	var pending []merge.Conflict
	for _, conflict := range s.moveConflicts {
		remaining := conflict
		remaining.RelatedIDs = nil
		for _, id := range conflict.RelatedIDs {
			covered := false
			for n := s.nodes[id]; n != nil; n = s.nodes[n.row.Parent] {
				if _, ok := s.decisions[n.row.ID]; ok {
					covered = true
					break
				}
			}
			if !covered {
				remaining.RelatedIDs = append(remaining.RelatedIDs, id)
			}
		}
		if len(remaining.RelatedIDs) > 0 {
			pending = append(pending, remaining)
		}
	}
	result, err := merge.MergeWithPendingConflicts(inputs[0], inputs[1], inputs[2], s.db, pending)
	if err != nil {
		return s.state(), err
	}
	if result.NeedsRules {
		return s.state(), fmt.Errorf("изменение требует новых правил сопоставления; проверьте имена и ключи узла")
	}
	for i := range result.Conflicts {
		c := &result.Conflicts[i]
		if s.nodes[c.TargetID] == nil {
			c.TargetID = "root"
		}
	}
	positions := map[string]int{}
	for i, id := range s.nodeOrder {
		positions[id] = i
	}
	sort.SliceStable(result.Conflicts, func(i, j int) bool {
		a, b := result.Conflicts[i], result.Conflicts[j]
		if a.TargetID == b.TargetID {
			return a.Reason < b.Reason
		}
		return positions[a.TargetID] < positions[b.TargetID]
	})
	preview := result.Data
	if len(preview) == 0 {
		preview = result.PreviewData
	}
	if len(preview) == 0 {
		preview = inputs[0]
	}
	doc, err := xmltree.Parse(preview)
	if err != nil {
		return s.state(), err
	}
	initialComparison := s.phase != "compared"
	if initialComparison {
		s.moveConflicts = nil
		for _, c := range result.Conflicts {
			if len(c.RelatedIDs) > 0 {
				c.RelatedIDs = append([]string(nil), c.RelatedIDs...)
				s.moveConflicts = append(s.moveConflicts, c)
			}
		}
	}
	s.phase = "compared"
	s.conflicts = result.Conflicts
	s.result = result.Data
	s.center = map[string]*xmltree.Node{}
	structural := map[string]bool{}
	for _, g := range s.report.Groups {
		if g.Status == "structural" {
			structural[g.Path] = true
		}
	}
	profile := s.db.Profile(doc.Root.Name)
	var visit func(*xmltree.Node)
	visit = func(n *xmltree.Node) {
		s.center[merge.NodeID(n, profile, structural)] = n
		for _, c := range n.Children() {
			visit(c)
		}
	}
	visit(doc.Root)
	if initialComparison {
		s.initialCenter = make(map[string]*xmltree.Node, len(s.center))
		for id, n := range s.center {
			s.initialCenter[id] = n
		}
	}
	// An explicitly edited identity still belongs to its original review row.
	for id, d := range s.decisions {
		if d.XML != "" && s.center[id] == nil {
			edited, e := xmltree.Parse([]byte(d.XML))
			if e == nil {
				s.center[id] = edited.Root
			}
		}
	}
	return s.state(), nil
}

func treeValue(n *xmltree.Node) *TreeValue {
	text := n.DirectText()
	if strings.TrimSpace(text) == "" {
		text = ""
	}
	v := &TreeValue{Attributes: map[string]string{}, Text: text}
	for _, a := range n.Attributes {
		redundant := false
		if a.Name.URI == "xmlns" || a.LexicalName == "xmlns" {
			found := false
			for p := n.Parent; p != nil && !found; p = p.Parent {
				for _, inherited := range p.Attributes {
					if inherited.LexicalName == a.LexicalName {
						redundant = inherited.Value == a.Value
						found = true
						break
					}
				}
			}
		}
		if !redundant {
			v.Attributes[a.LexicalName] = a.Value
		}
	}
	return v
}

// Ignore layout and namespace declarations when deciding whether an arrow is useful.
func reviewSignature(n *xmltree.Node) string {
	if n == nil {
		return "absent"
	}
	var attrs []string
	for _, a := range n.Attributes {
		if a.Name.URI != "xmlns" && a.LexicalName != "xmlns" {
			attrs = append(attrs, a.Name.String()+"="+a.Value)
		}
	}
	sort.Strings(attrs)
	text := n.DirectText()
	if strings.TrimSpace(text) == "" {
		text = ""
	}
	out := n.Name.String() + "\x00" + strings.Join(attrs, "\x00") + "\x01" + text
	for _, p := range n.Parts {
		if p.Element != nil {
			out += "\x02" + reviewSignature(p.Element)
		} else if p.Kind != "text" && p.Kind != "cdata" {
			out += "\x03" + p.Raw
		}
	}
	return out
}

// A later arrow edits the current accepted parent, retaining previous sibling choices.
func (s *Session) composeNode(scope, target, replacement string, side int) (string, error) {
	original := s.center[scope]
	if original == nil {
		return "", fmt.Errorf("сначала восстановите родительский узел")
	}
	doc, err := xmltree.Parse([]byte(standalone(original)))
	if err != nil {
		return "", err
	}
	clones := map[*xmltree.Node]*xmltree.Node{}
	var pair func(*xmltree.Node, *xmltree.Node)
	pair = func(a, b *xmltree.Node) {
		clones[a] = b
		ac, bc := a.Children(), b.Children()
		for i := range ac {
			pair(ac[i], bc[i])
		}
	}
	pair(original, doc.Root)
	parent := clones[s.center[s.nodes[target].row.Parent]]
	if parent == nil {
		missing := s.nodes[target].row.Parent
		for p := s.nodes[missing].row.Parent; p != scope && s.center[p] == nil; p = s.nodes[missing].row.Parent {
			missing = p
		}
		restored, err := s.restorePath(missing, target, replacement, side)
		if err != nil {
			return "", err
		}
		return s.composeNode(scope, missing, restored, side)
	}
	var value *xmltree.Node
	if replacement != "" {
		r, e := xmltree.Parse([]byte(replacement))
		if e != nil {
			return "", e
		}
		value = r.Root
		value.Parent = parent
	}
	old := clones[s.center[target]]
	parts := append([]xmltree.Part(nil), parent.Parts...)
	pos := len(parts)
	if old != nil {
		for i, p := range parts {
			if p.Element == old {
				pos = i
				parts = append(parts[:i], parts[i+1:]...)
				break
			}
		}
	} else {
		siblings := s.nodes[s.nodes[target].row.Parent].row.Children
		after := false
		for _, id := range siblings {
			if id == target {
				after = true
				continue
			}
			if !after {
				continue
			}
			next := clones[s.center[id]]
			if next == nil {
				continue
			}
			for i, p := range parts {
				if p.Element == next {
					pos = i
					break
				}
			}
			if pos < len(parts) {
				break
			}
		}
	}
	if value != nil {
		parts = append(parts, xmltree.Part{})
		copy(parts[pos+1:], parts[pos:])
		parts[pos] = xmltree.Part{Kind: "element", Element: value}
	}
	parent.SetParts(parts)
	return doc.Root.XML(), nil
}
