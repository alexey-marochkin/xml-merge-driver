// Package xmltree retains physical XML alongside its semantic DOM.
package xmltree

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

type Name struct{ URI, Local string }

func (n Name) String() string { return "{" + n.URI + "}" + n.Local }

type Attribute struct {
	Name                          Name
	LexicalName                   string
	Value                         string
	leading, assignment, rawValue string
	quote                         byte
}

type Part struct {
	Element *Node
	Kind    string
	Raw     string
	Text    string
}

type Node struct {
	Name            Name
	LexicalName     string
	Attributes      []Attribute
	Parts           []Part
	Parent          *Node
	Start, End      int
	openTail, close string
	selfClosing     bool
}

type Document struct {
	Root   *Node
	Parts  []Part
	Format Format
	Source string
}

func Parse(data []byte) (*Document, error) {
	source, format, err := decode(data)
	if err != nil {
		return nil, err
	}
	d := &Document{Format: format, Source: source}
	dec := xml.NewDecoder(strings.NewReader(source))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var stack []*Node
	var scopes []map[string]string
	appendPart := func(p Part) {
		if len(stack) == 0 {
			d.Parts = append(d.Parts, p)
		} else {
			n := stack[len(stack)-1]
			n.Parts = append(n.Parts, p)
		}
	}
	for {
		start := int(dec.InputOffset())
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML at byte %d: %w", start, err)
		}
		end := int(dec.InputOffset())
		raw := source[start:end]
		switch t := t.(type) {
		case xml.StartElement:
			if len(stack) >= 2048 {
				return nil, fmt.Errorf("XML nesting exceeds 2048 elements")
			}
			n, err := parseOpening(raw)
			if err != nil {
				return nil, err
			}
			n.Start, n.Name = start, Name{t.Name.Space, t.Name.Local}
			var scope map[string]string
			if len(scopes) > 0 {
				scope = scopes[len(scopes)-1]
			} else {
				scope = map[string]string{"xml": "http://www.w3.org/XML/1998/namespace"}
			}
			copied := false
			for _, a := range t.Attr {
				declaration := a.Name.Space == "xmlns" || a.Name.Space == "" && a.Name.Local == "xmlns"
				if declaration && !copied {
					clone := make(map[string]string, len(scope)+1)
					for k, v := range scope {
						clone[k] = v
					}
					scope, copied = clone, true
				}
				if a.Name.Space == "xmlns" {
					if a.Name.Local == "xmlns" || a.Name.Local == "xml" && a.Value != "http://www.w3.org/XML/1998/namespace" || a.Name.Local != "xml" && a.Value == "http://www.w3.org/XML/1998/namespace" || a.Value == "http://www.w3.org/2000/xmlns/" || a.Value == "" {
						return nil, fmt.Errorf("invalid namespace declaration %s", a.Name.Local)
					}
					scope[a.Name.Local] = a.Value
				}
				if a.Name.Space == "" && a.Name.Local == "xmlns" {
					if a.Value == "http://www.w3.org/XML/1998/namespace" || a.Value == "http://www.w3.org/2000/xmlns/" {
						return nil, fmt.Errorf("reserved namespace URI")
					}
					scope[""] = a.Value
				}
			}
			if strings.HasPrefix(n.LexicalName, "xmlns:") {
				return nil, fmt.Errorf("xmlns cannot be an element prefix")
			}
			if len(t.Attr) != len(n.Attributes) {
				return nil, fmt.Errorf("attribute token mismatch")
			}
			if err := checkPrefix(n.LexicalName, scope); err != nil {
				return nil, err
			}
			seen := map[Name]bool{}
			for i, a := range t.Attr {
				if err := checkPrefix(n.Attributes[i].LexicalName, scope); err != nil {
					return nil, err
				}
				name := Name{a.Name.Space, a.Name.Local}
				if seen[name] {
					return nil, fmt.Errorf("duplicate attribute %s", name)
				}
				seen[name] = true
				n.Attributes[i].Name, n.Attributes[i].Value = name, a.Value
			}
			if len(stack) == 0 {
				if d.Root != nil {
					return nil, fmt.Errorf("multiple root elements")
				}
				d.Root = n
			} else {
				n.Parent = stack[len(stack)-1]
			}
			appendPart(Part{Element: n, Kind: "element"})
			stack, scopes = append(stack, n), append(scopes, scope)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unexpected end element")
			}
			n := stack[len(stack)-1]
			if !n.selfClosing {
				n.close = raw
			}
			n.End = end
			stack, scopes = stack[:len(stack)-1], scopes[:len(scopes)-1]
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(string(t)) != "" {
				return nil, fmt.Errorf("text outside root element")
			}
			kind := "text"
			if strings.HasPrefix(raw, "<![CDATA[") {
				kind = "cdata"
			}
			appendPart(Part{Kind: kind, Raw: raw, Text: string(t)})
		case xml.Comment:
			appendPart(Part{Kind: "comment", Raw: raw, Text: string(t)})
		case xml.ProcInst:
			appendPart(Part{Kind: "instruction", Raw: raw})
		case xml.Directive:
			return nil, fmt.Errorf("DTD and XML directives are not supported")
		}
	}
	if d.Root == nil {
		return nil, fmt.Errorf("XML has no root element")
	}
	return d, nil
}

func checkPrefix(name string, scope map[string]string) error {
	if strings.Count(name, ":") > 1 {
		return fmt.Errorf("invalid qualified name %q", name)
	}
	if p, _, ok := strings.Cut(name, ":"); ok && p != "xmlns" {
		if scope[p] == "" {
			return fmt.Errorf("undeclared namespace prefix %q", p)
		}
	}
	return nil
}

func whitespace(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }

func parseOpening(raw string) (*Node, error) {
	n := &Node{}
	i := 1
	for i < len(raw) && !whitespace(raw[i]) && raw[i] != '/' && raw[i] != '>' {
		i++
	}
	n.LexicalName = raw[1:i]
	for i < len(raw) {
		lead := i
		for i < len(raw) && whitespace(raw[i]) {
			i++
		}
		if i >= len(raw) {
			break
		}
		if raw[i] == '/' || raw[i] == '>' {
			n.openTail, n.selfClosing = raw[lead:], raw[i] == '/'
			return n, nil
		}
		nameStart := i
		for i < len(raw) && !whitespace(raw[i]) && raw[i] != '=' {
			i++
		}
		nameEnd := i
		for i < len(raw) && raw[i] != '"' && raw[i] != '\'' {
			i++
		}
		if i >= len(raw) {
			break
		}
		quote := raw[i]
		valueStart := i + 1
		i++
		for i < len(raw) && raw[i] != quote {
			i++
		}
		if i >= len(raw) {
			break
		}
		n.Attributes = append(n.Attributes, Attribute{LexicalName: raw[nameStart:nameEnd], leading: raw[lead:nameStart], assignment: raw[nameEnd : valueStart-1], quote: quote, rawValue: raw[valueStart:i]})
		i++
	}
	return nil, fmt.Errorf("malformed opening element")
}

func (n *Node) Children() []*Node {
	var nodes []*Node
	for _, p := range n.Parts {
		if p.Element != nil {
			nodes = append(nodes, p.Element)
		}
	}
	return nodes
}

// DirectText deliberately excludes descendants. RawParts retains physical CR/LF.
func (n *Node) DirectText() string {
	var b strings.Builder
	for _, p := range n.Parts {
		if p.Kind == "text" || p.Kind == "cdata" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func (n *Node) Attribute(name Name) (string, bool) {
	for _, a := range n.Attributes {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

func (n *Node) Opening() string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(n.LexicalName)
	for _, a := range n.Attributes {
		b.WriteString(a.leading)
		b.WriteString(a.LexicalName)
		b.WriteString(a.assignment)
		b.WriteByte(a.quote)
		b.WriteString(a.rawValue)
		b.WriteByte(a.quote)
	}
	b.WriteString(n.openTail)
	return b.String()
}

func (n *Node) writeXML(w io.Writer) error {
	if _, err := io.WriteString(w, n.Opening()); err != nil {
		return err
	}
	if err := writeParts(w, n.Parts); err != nil {
		return err
	}
	_, err := io.WriteString(w, n.close)
	return err
}

func writeParts(w io.Writer, parts []Part) error {
	for _, p := range parts {
		if p.Element != nil {
			if err := p.Element.writeXML(w); err != nil {
				return err
			}
		} else if _, err := io.WriteString(w, p.Raw); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) XML() string { var b strings.Builder; _ = n.writeXML(&b); return b.String() }

func (n *Node) SetParts(parts []Part) {
	n.Parts = parts
	if n.selfClosing && len(parts) > 0 {
		n.selfClosing = false
		n.openTail = strings.TrimSuffix(n.openTail, "/>") + ">"
		n.close = "</" + n.LexicalName + ">"
	}
}

func PartsXML(parts []Part) string {
	var b strings.Builder
	_ = writeParts(&b, parts)
	return b.String()
}

func (d *Document) Bytes() ([]byte, error) {
	var b strings.Builder
	if err := writeParts(&b, d.Parts); err != nil {
		return nil, err
	}
	return d.Format.Encode(b.String())
}

// ReplaceAttribute preserves the slot, quote and spacing of an existing attribute.
// A newly added attribute retains the incoming spelling and is appended.
func (n *Node) ReplaceAttribute(name Name, incoming *Attribute) {
	for i := range n.Attributes {
		a := &n.Attributes[i]
		if a.Name != name {
			continue
		}
		if incoming == nil {
			n.Attributes = append(n.Attributes[:i], n.Attributes[i+1:]...)
			return
		}
		a.Value, a.rawValue = incoming.Value, escapeAttribute(incoming.Value, a.quote)
		return
	}
	if incoming != nil {
		a := *incoming
		a.leading = " "
		n.Attributes = append(n.Attributes, a)
	}
}

func escapeAttribute(s string, quote byte) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", "\r", "&#13;", "\n", "&#10;", "\t", "&#9;")
	s = r.Replace(s)
	if quote == '\'' {
		return strings.ReplaceAll(s, "'", "&apos;")
	}
	return strings.ReplaceAll(s, "\"", "&quot;")
}
