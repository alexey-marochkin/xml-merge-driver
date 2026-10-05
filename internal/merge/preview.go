package merge

import "xmlmerge/internal/xmltree"

// Keep non-conflicting automatic changes, but display BASE at unresolved scopes.
// This is a preview only: Data remains empty until every conflict is resolved.
func (e *engine) previewBaseConflicts(base, draft *xmltree.Document) []byte {
	originals := map[string]*xmltree.Node{}
	var collect func(*xmltree.Node)
	collect = func(n *xmltree.Node) {
		originals[NodeID(n, e.profile, e.structural)] = n
		for _, c := range n.Children() {
			collect(c)
		}
	}
	collect(base.Root)
	draftIDs := map[string]bool{}
	var collectDraft func(*xmltree.Node)
	collectDraft = func(n *xmltree.Node) {
		draftIDs[NodeID(n, e.profile, e.structural)] = true
		for _, c := range n.Children() {
			collectDraft(c)
		}
	}
	collectDraft(draft.Root)
	unresolved := map[string]bool{}
	for _, c := range e.conflicts {
		ids := c.RelatedIDs
		if len(ids) == 0 {
			ids = []string{c.TargetID}
		}
		for _, id := range ids {
			unresolved[id] = true
			// Restore a missing source container when necessary, while preserving
			// independent automatic changes outside that container.
			for n := originals[id]; n != nil && n.Parent != nil; n = n.Parent {
				parent := NodeID(n.Parent, e.profile, e.structural)
				if draftIDs[parent] {
					break
				}
				unresolved[parent] = true
			}
		}
	}
	var build func(*xmltree.Node) *xmltree.Node
	build = func(n *xmltree.Node) *xmltree.Node {
		id := NodeID(n, e.profile, e.structural)
		if unresolved[id] {
			return originals[id]
		}
		parts := []xmltree.Part{}
		seen := map[string]bool{}
		for _, p := range n.Parts {
			if p.Element != nil {
				cid := NodeID(p.Element, e.profile, e.structural)
				seen[cid] = true
				p.Element = build(p.Element)
				if p.Element == nil {
					continue
				}
			}
			parts = append(parts, p)
		}
		if b := originals[id]; b != nil {
			children := b.Children()
			for i, c := range children {
				cid := NodeID(c, e.profile, e.structural)
				if !seen[cid] && unresolved[cid] {
					pos := len(parts)
					for _, next := range children[i+1:] {
						nid := NodeID(next, e.profile, e.structural)
						found := false
						for j, p := range parts {
							if p.Element != nil && NodeID(p.Element, e.profile, e.structural) == nid {
								pos = j
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					parts = append(parts, xmltree.Part{})
					copy(parts[pos+1:], parts[pos:])
					parts[pos] = xmltree.Part{Kind: "element", Element: c}
				}
			}
		}
		n.SetParts(parts)
		return n
	}
	root := build(draft.Root)
	for i := range draft.Parts {
		if draft.Parts[i].Element != nil {
			draft.Parts[i].Element = root
		}
	}
	draft.Root = root
	data, err := draft.Bytes()
	if err == nil {
		if parsed, err := xmltree.Parse(data); err == nil && sameExpandedNames(root, parsed.Root) {
			return data
		}
	}
	data, _ = base.Bytes()
	return data
}
