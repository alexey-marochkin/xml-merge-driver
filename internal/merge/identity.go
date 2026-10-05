package merge

import (
	"crypto/sha256"
	"encoding/hex"
	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

// NodeID identifies an aligned row, including its keyed ancestors.
func NodeID(n *xmltree.Node, p *rules.Profile, structural map[string]bool) string {
	if n.Parent == nil {
		return "root"
	}
	key := n.Name.String()
	if !structural[rules.Path(n)] && p != nil {
		if rule := p.Effective(n); rule != nil {
			if k, err := rule.Key(n); err == nil {
				key = k
			}
		}
	}
	h := sha256.Sum256([]byte(NodeID(n.Parent, p, structural) + "\x00" + key))
	return hex.EncodeToString(h[:12])
}
