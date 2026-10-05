package gitdriver

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strings"

	"xmlmerge/internal/xmltree"
)

type Attribute struct {
	Name string `xml:"name,attr"`
}
type Change struct {
	Attributes []Attribute `xml:"attribute"`
}
type Policy struct {
	XMLName  xml.Name `xml:"merge-policy"`
	Version  int      `xml:"version,attr"`
	Supplier struct {
		Branch   string   `xml:"branch,attr,omitempty"` // Legacy single-branch format.
		Branches []string `xml:"branch"`
	} `xml:"supplier"`
	Files          []string `xml:"eligible-files>file"`
	KeepLocalFiles []string `xml:"keep-local-files>file"`
	Changes        []Change `xml:"eligible-changes>change"`
	RemotePaths    []string `xml:"prefer-remote-paths>contains"`
}

func Load(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if _, err := xmltree.Parse(data); err != nil {
		return nil, fmt.Errorf("merge policy: %w", err)
	}
	var p Policy
	if err := xml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

var attributeName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

func (p *Policy) SupplierBranches() []string {
	branches := append([]string(nil), p.Supplier.Branches...)
	if p.Supplier.Branch != "" {
		branches = append(branches, p.Supplier.Branch)
	}
	return branches
}

func (p *Policy) Validate() error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported merge policy version %d", p.Version)
	}
	branches := p.SupplierBranches()
	if len(branches) == 0 {
		return fmt.Errorf("at least one supplier branch is required")
	}
	seenBranches := map[string]bool{}
	for _, branch := range branches {
		if seenBranches[branch] {
			return fmt.Errorf("duplicate supplier branch %q", branch)
		}
		seenBranches[branch] = true
		if branch == "" || strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, " ~^:?*[\\\t\r\n") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") || strings.HasSuffix(branch, ".") {
			return fmt.Errorf("invalid supplier branch %q", branch)
		}
		for _, part := range strings.Split(branch, "/") {
			if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
				return fmt.Errorf("invalid supplier branch %q", branch)
			}
		}
	}
	for _, name := range append(append([]string(nil), p.Files...), p.KeepLocalFiles...) {
		if name == "" || strings.ContainsAny(name, "/\\*?") {
			return fmt.Errorf("eligible file must be an exact basename: %q", name)
		}
	}
	for _, change := range p.Changes {
		if len(change.Attributes) == 0 {
			return fmt.Errorf("empty eligible change")
		}
		seen := map[string]bool{}
		for _, a := range change.Attributes {
			if !attributeName.MatchString(a.Name) || seen[a.Name] {
				return fmt.Errorf("invalid or duplicate attribute %q", a.Name)
			}
			seen[a.Name] = true
		}
	}
	for _, part := range p.RemotePaths {
		if part == "" {
			return fmt.Errorf("empty path condition matches every file")
		}
	}
	return nil
}
