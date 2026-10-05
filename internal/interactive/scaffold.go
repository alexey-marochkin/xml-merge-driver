package interactive

import (
	"fmt"
	"strings"
	"xmlmerge/internal/xmltree"
)

// Restore a path inside an explicitly deleted ancestor without reviving siblings.
func (s *Session) restorePath(scope, target, replacement string, side int) (string, error) {
	path := map[string]bool{}
	for n := s.nodes[target]; n != nil; n = s.nodes[n.row.Parent] {
		path[n.row.ID] = true
		if n.row.ID == scope {
			break
		}
	}
	var build func(string, map[*xmltree.Node]bool) (*xmltree.Node, error)
	build = func(id string, keys map[*xmltree.Node]bool) (*xmltree.Node, error) {
		if id == target {
			if replacement == "" {
				return nil, nil
			}
			d, e := xmltree.Parse([]byte(replacement))
			if e != nil {
				return nil, e
			}
			return d.Root, nil
		}
		source := s.sourceNode(id, side)
		if source == nil {
			return nil, fmt.Errorf("не найден исходный родительский узел")
		}
		doc, err := xmltree.Parse([]byte(standalone(source)))
		if err != nil {
			return nil, err
		}
		ownKeys := s.identityPathNodes(source)
		for n := range keys {
			ownKeys[n] = true
		}
		ids := map[*xmltree.Node]string{}
		for _, cid := range s.nodes[id].row.Children {
			for _, n := range s.nodes[cid].nodes {
				if n != nil {
					ids[n] = cid
				}
			}
		}
		parts := []xmltree.Part{}
		for _, p := range source.Parts {
			if p.Element != nil {
				cid := ids[p.Element]
				if !path[cid] && !ownKeys[p.Element] {
					continue
				}
				child, e := build(cid, ownKeys)
				if e != nil {
					return nil, e
				}
				if child == nil {
					continue
				}
				child.Parent = doc.Root
				p.Element = child
			}
			parts = append(parts, p)
		}
		doc.Root.SetParts(parts)
		return doc.Root, nil
	}
	root, err := build(scope, nil)
	if err != nil {
		return "", err
	}
	return root.XML(), nil
}

func (s *Session) sourceNode(id string, side int) *xmltree.Node {
	n := s.nodes[id]
	if n == nil {
		return nil
	}
	if n.nodes[side] != nil {
		return n.nodes[side]
	}
	for _, i := range []int{1, 2, 0} {
		if n.nodes[i] != nil {
			return n.nodes[i]
		}
	}
	return nil
}

// Retain only the child fields needed to identify a newly restored ancestor.
func (s *Session) identityPathNodes(n *xmltree.Node) map[*xmltree.Node]bool {
	keep := map[*xmltree.Node]bool{}
	profile := s.db.Profile(s.docs[0].Root.Name)
	if profile == nil {
		return keep
	}
	rule := profile.Effective(n)
	if rule == nil || rule.Mode != "element" {
		return keep
	}
	for _, field := range rule.Fields {
		parent := n
		for _, segment := range strings.Split(field.Name, "/") {
			if strings.HasPrefix(segment, "@") {
				break
			}
			var found *xmltree.Node
			for _, child := range parent.Children() {
				if field.Lexical && child.LexicalName == segment || !field.Lexical && child.Name == (xmltree.Name{URI: field.Namespace, Local: segment}) {
					found = child
					break
				}
			}
			if found == nil {
				break
			}
			keep[found] = true
			parent = found
		}
	}
	return keep
}
