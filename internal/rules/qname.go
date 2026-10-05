package rules

import (
	"strings"
	"xmlmerge/internal/xmltree"
)

func validSelector(s string) bool {
	if s == "*" {
		return true
	}
	parts, ok := selectorParts(s)
	if !ok {
		return false
	}
	for _, name := range parts {
		if strings.HasPrefix(name, "{") {
			name = name[strings.IndexByte(name, '}')+1:]
		}
		if !validQName(name) {
			return false
		}
	}
	return true
}

func matchesSelector(selector string, n *xmltree.Node) bool {
	if selector == "" || selector == "*" {
		return false
	}
	parts, ok := selectorParts(selector)
	if !ok {
		return false
	}
	for i := len(parts) - 1; i >= 0; i-- {
		if n == nil {
			return false
		}
		name := n.LexicalName
		if strings.HasPrefix(parts[i], "{") {
			name = n.Name.String()
		}
		if name != parts[i] {
			return false
		}
		n = n.Parent
	}
	return true
}

func selectorSpecificity(s string) int {
	parts, _ := selectorParts(s)
	score := len(parts) * 2
	if strings.HasPrefix(s, "{") {
		score++
	}
	return score
}

func selectorParts(s string) ([]string, bool) {
	var parts []string
	for s != "" {
		start := 0
		if s[0] == '{' {
			end := strings.IndexByte(s, '}')
			if end < 0 {
				return nil, false
			}
			start = end + 1
		}
		i := strings.IndexByte(s[start:], '/')
		if i < 0 {
			parts = append(parts, s)
			return parts, true
		}
		i += start
		parts = append(parts, s[:i])
		s = s[i+1:]
		if s == "" {
			return nil, false
		}
	}
	return nil, false
}

// Source format names may carry prefixes but do not declare their namespace URIs.
func validQName(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) > 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for i, c := range p {
			if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0xC0 {
				continue
			}
			if i > 0 && (c == '-' || c == '.' || c >= '0' && c <= '9') {
				continue
			}
			return false
		}
	}
	return true
}
