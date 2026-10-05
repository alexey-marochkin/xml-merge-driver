package rules

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xmlmerge/internal/xmltree"
)

type Field struct {
	Name      string `xml:"name,attr"`
	Namespace string `xml:"namespace,attr,omitempty"`
	Lexical   bool   `xml:"lexical,attr,omitempty"`
	TrimSpace bool   `xml:"trim-space,attr,omitempty"`
}
type Rule struct {
	Path            string  `xml:"path,attr,omitempty"`
	Selector        string  `xml:"selector,attr,omitempty"`
	TrimSpace       bool    `xml:"trim-space,attr,omitempty"`
	NoInherit       bool    `xml:"no-inherit,attr,omitempty"`
	AllowDeleteAdd  bool    `xml:"allow-delete-add,attr,omitempty"`
	Mode            string  `xml:"mode,attr"`
	Order           string  `xml:"order,attr"`
	ConfiguredOrder string  `xml:"-" json:",omitempty"`
	Origin          string  `xml:"origin,attr,omitempty"`
	Fields          []Field `xml:"field"`
}
type Profile struct {
	Root          string         `xml:"root,attr"`
	Namespace     string         `xml:"namespace,attr,omitempty"`
	Rules         []Rule         `xml:"rule"`
	KnownElements []KnownElement `xml:"known-element"`
}

// Inventory entries describe observed XML without creating identity overrides.
type KnownElement struct {
	Name       string  `xml:"name,attr"`
	Namespace  string  `xml:"namespace,attr,omitempty"`
	Attributes []Field `xml:"attribute"`
	Elements   []Field `xml:"element"`
}
type Database struct {
	XMLName  xml.Name  `xml:"xmlmerge"`
	Version  int       `xml:"version,attr"`
	Profiles []Profile `xml:"profile"`
}

func New() *Database { return &Database{Version: 1} }

func DefaultPath() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "XmlMerge", "rules.xml") }

func Load(path string) (*Database, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	// The lossless parser also rejects malformed/ambiguous XML before unmarshalling.
	if _, err := xmltree.Parse(data); err != nil {
		return nil, err
	}
	var db Database
	if err := xml.Unmarshal(data, &db); err != nil {
		return nil, err
	}
	if err := db.Validate(); err != nil {
		return nil, err
	}
	return &db, nil
}

func (db *Database) Validate() error {
	if db.Version != 1 && db.Version != 2 {
		return fmt.Errorf("unsupported rules version %d", db.Version)
	}
	seen := map[xmltree.Name]bool{}
	for _, p := range db.Profiles {
		name := xmltree.Name{URI: p.Namespace, Local: p.Root}
		if p.Root == "" || seen[name] {
			return fmt.Errorf("missing or duplicate profile root %s", name)
		}
		seen[name] = true
		known := map[xmltree.Name]bool{}
		for _, e := range p.KnownElements {
			key := xmltree.Name{URI: e.Namespace, Local: e.Name}
			if db.Version < 2 || e.Name == "" || known[key] {
				return fmt.Errorf("invalid or duplicate known element %s", key)
			}
			known[key] = true
		}
		if p.Namespace == "*" && db.Version < 2 {
			return fmt.Errorf("namespace wildcard requires rules version 2")
		}
		paths := map[string]bool{}
		for _, r := range p.Rules {
			rootPath := "/" + name.String()
			id := r.Path
			if r.Selector != "" {
				if db.Version < 2 || r.Path != "" || !validSelector(r.Selector) {
					return fmt.Errorf("invalid name selector %q", r.Selector)
				}
				id = "selector:" + r.Selector
			} else if r.Path != rootPath && !strings.HasPrefix(r.Path, rootPath+"/") {
				return fmt.Errorf("invalid or duplicate rule path %q", r.Path)
			}
			if paths[id] {
				return fmt.Errorf("duplicate rule %q", id)
			}
			paths[id] = true
			if (r.TrimSpace || r.NoInherit) && db.Version < 2 {
				return fmt.Errorf("trim-space requires rules version 2")
			}
			if r.Order != "significant" && r.Order != "insignificant" && r.Order != "default" {
				return fmt.Errorf("invalid child order %q", r.Order)
			}
			if r.Order == "default" {
				found := false
				for _, common := range p.Rules {
					if common.Selector == "*" && common.Order != "default" {
						found = true
					}
				}
				if db.Version < 2 || r.Selector == "*" || !found {
					return fmt.Errorf("default order requires a profile default")
				}
			}
			switch r.Mode {
			case "attribute", "element":
				if len(r.Fields) == 0 {
					return fmt.Errorf("rule %s requires fields", r.Path)
				}
				for _, f := range r.Fields {
					if f.Name == "" {
						return fmt.Errorf("empty field in %s", r.Path)
					}
					if f.Lexical && (db.Version < 2 || f.Namespace != "" || !validQName(f.Name)) {
						return fmt.Errorf("invalid lexical field %q", f.Name)
					}
					if f.TrimSpace && db.Version < 2 {
						return fmt.Errorf("field trim-space requires rules version 2")
					}
				}
			case "text", "name":
				if len(r.Fields) != 0 {
					return fmt.Errorf("%s rule cannot have fields", r.Mode)
				}
			case "auto":
				if db.Version < 2 || r.Selector != "*" || len(r.Fields) != 0 {
					return fmt.Errorf("auto mode is only allowed for the profile default")
				}
			default:
				return fmt.Errorf("invalid identification mode %q", r.Mode)
			}
		}
	}
	return nil
}

func Path(n *xmltree.Node) string {
	var segments []string
	for p := n; p != nil; p = p.Parent {
		segments = append(segments, p.Name.String())
	}
	var b strings.Builder
	for i := len(segments) - 1; i >= 0; i-- {
		b.WriteByte('/')
		b.WriteString(segments[i])
	}
	return b.String()
}

func (db *Database) Profile(name xmltree.Name) *Profile {
	var fallback *Profile
	for i := range db.Profiles {
		p := &db.Profiles[i]
		if p.Root == name.Local && p.Namespace == name.URI {
			return p
		}
		if p.Root == name.Local && p.Namespace == "*" {
			fallback = p
		}
	}
	return fallback
}

func (p *Profile) Effective(n *xmltree.Node) *Rule {
	if p == nil {
		return nil
	}
	// Concrete node settings override an imported name template; path settings
	// still inherit through ancestors. Source format templates explicitly opt out.
	for i := range p.Rules {
		if p.Rules[i].Path == Path(n) && p.Rules[i].Selector == "" {
			return p.resolve(&p.Rules[i])
		}
	}
	var matched *Rule
	for i := range p.Rules {
		if matchesSelector(p.Rules[i].Selector, n) && (matched == nil || selectorSpecificity(p.Rules[i].Selector) > selectorSpecificity(matched.Selector)) {
			matched = &p.Rules[i]
		}
	}
	if matched != nil {
		return p.resolve(matched)
	}
	// The profile default is the fallback for every tag without its own rule.
	// Ancestor inheritance remains only for older profiles without a default.
	for i := range p.Rules {
		if p.Rules[i].Selector == "*" {
			return &p.Rules[i]
		}
	}
	for node := n.Parent; node != nil; node = node.Parent {
		path := Path(node)
		for i := range p.Rules {
			if p.Rules[i].Selector == "" && p.Rules[i].Path == path {
				return p.resolve(&p.Rules[i])
			}
		}
		for i := range p.Rules {
			if !p.Rules[i].NoInherit && matchesSelector(p.Rules[i].Selector, node) {
				return p.resolve(&p.Rules[i])
			}
		}
	}
	return nil
}

func (p *Profile) resolve(rule *Rule) *Rule {
	if rule.Order != "default" {
		return rule
	}
	resolved := *rule
	resolved.ConfiguredOrder = "default"
	for _, common := range p.Rules {
		if common.Selector == "*" {
			resolved.Order = common.Order
			return &resolved
		}
	}
	return rule
}

// AppliesDirectly distinguishes an own name rule from an inherited parent rule.
func (r *Rule) AppliesDirectly(n *xmltree.Node) bool {
	return r != nil && (r.Path == Path(n) || matchesSelector(r.Selector, n))
}

// Key is a tuple encoded without delimiter collisions. Empty and absent differ.
func (r *Rule) Key(n *xmltree.Node) (string, error) {
	values := []string{n.Name.String()}
	switch r.Mode {
	case "name":
		// Expanded element name identifies a singleton property; its value
		// remains ordinary mergeable content. Duplicate names still fail.
	case "attribute":
		for _, f := range r.Fields {
			v, ok := n.Attribute(xmltree.Name{URI: f.Namespace, Local: f.Name})
			if f.Lexical {
				ok = false
				for _, a := range n.Attributes {
					if a.LexicalName == f.Name {
						v, ok = a.Value, true
						break
					}
				}
			}
			if !ok {
				return "", fmt.Errorf("%s: missing attribute %s", Path(n), f.Name)
			}
			if f.TrimSpace {
				v = strings.Trim(v, " \t\r\n")
			}
			values = append(values, v)
		}
	case "element":
	fields:
		for _, f := range r.Fields {
			parent := n
			var found *xmltree.Node
			for _, segment := range strings.Split(f.Name, "/") {
				if strings.HasPrefix(segment, "@") {
					value, ok := parent.Attribute(xmltree.Name{Local: strings.TrimPrefix(segment, "@")})
					if !ok {
						return "", fmt.Errorf("%s: missing key attribute %s", Path(n), f.Name)
					}
					if f.TrimSpace {
						value = strings.Trim(value, " \t\r\n")
					}
					values = append(values, value)
					continue fields
				}
				found = nil
				for _, child := range parent.Children() {
					if (!f.Lexical && child.Name == (xmltree.Name{URI: f.Namespace, Local: segment})) || (f.Lexical && child.LexicalName == segment) {
						if found != nil {
							return "", fmt.Errorf("%s: repeated key element %s", Path(n), f.Name)
						}
						found = child
					}
				}
				if found == nil {
					return "", fmt.Errorf("%s: missing key element %s", Path(n), f.Name)
				}
				parent = found
			}
			if found == nil || len(found.Children()) != 0 {
				return "", fmt.Errorf("%s: key element %s must have a scalar value", Path(n), f.Name)
			}
			value := found.DirectText()
			if f.TrimSpace {
				value = strings.Trim(value, " \t\r\n")
			}
			values = append(values, value)
		}
	case "text":
		if len(n.Children()) != 0 {
			return "", fmt.Errorf("%s: text identity requires scalar content", Path(n))
		}
		value := n.DirectText()
		if r.TrimSpace {
			value = strings.Trim(value, " \t\r\n")
		}
		values = append(values, value)
	default:
		return "", fmt.Errorf("invalid rule mode %q", r.Mode)
	}
	data, _ := json.Marshal(values)
	return string(data), nil
}
