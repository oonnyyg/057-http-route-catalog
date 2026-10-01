// Package routes compiles a local HTTP route catalog that is used both for
// request matching and for reverse link generation, so diagnostics and
// dispatch always agree.
package routes

import (
	"fmt"
	"sort"
	"strings"
)

// Segment kinds in a parsed path template.
const (
	segStatic = iota // literal segment
	segParam         // {name} single-segment parameter
	segWild          // *name trailing multi-segment parameter
)

type segment struct {
	kind  int
	text  string // literal text for segStatic
	param string // parameter name for segParam and segWild
}

// Route is one validated catalog entry.
type Route struct {
	Name   string
	Method string // always upper-case
	Path   string // original template text
	Line   int    // 1-based position in the declaration source
	segs   []segment
}

// Problem describes a single validation issue found in a catalog.
type Problem struct {
	Route string `json:"route"`
	Line  int    `json:"line"`
	Issue string `json:"issue"`
}

// Catalog is a validated set of routes.
type Catalog struct {
	routes []*Route
	byName map[string]*Route
}

// Routes returns the routes sorted by name for stable reporting.
func (c *Catalog) Routes() []*Route {
	out := make([]*Route, len(c.routes))
	copy(out, c.routes)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Decl is a raw route declaration as read from the catalog source.
type Decl struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

func validParamName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// parseTemplate validates and splits a path template into segments.
func parseTemplate(path string) ([]segment, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("template must start with '/'")
	}
	if strings.ContainsAny(path, "%\\") {
		return nil, fmt.Errorf("template must not contain '%%' or '\\'")
	}
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("template must not contain a query string or fragment")
	}
	body := strings.TrimSuffix(path, "/")
	if body == "" {
		return nil, nil // root
	}
	parts := strings.Split(body[1:], "/")
	segs := make([]segment, 0, len(parts))
	seen := map[string]bool{}
	for i, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("template contains consecutive slashes")
		}
		if p == "." || p == ".." {
			return nil, fmt.Errorf("template contains dot segment %q", p)
		}
		if strings.HasPrefix(p, "{") || strings.HasPrefix(p, "}") {
			if len(p) < 3 || !strings.HasPrefix(p, "{") || !strings.HasSuffix(p, "}") {
				return nil, fmt.Errorf("malformed parameter segment %q", p)
			}
			name := p[1 : len(p)-1]
			if !validParamName(name) {
				return nil, fmt.Errorf("invalid parameter name %q", name)
			}
			if seen[name] {
				return nil, fmt.Errorf("duplicate parameter name %q in route", name)
			}
			seen[name] = true
			segs = append(segs, segment{kind: segParam, param: name})
			continue
		}
		if strings.HasPrefix(p, "*") {
			name := p[1:]
			if !validParamName(name) {
				return nil, fmt.Errorf("invalid wildcard parameter name %q", name)
			}
			if i != len(parts)-1 {
				return nil, fmt.Errorf("wildcard *%s must be the last segment", name)
			}
			if seen[name] {
				return nil, fmt.Errorf("duplicate parameter name %q in route", name)
			}
			seen[name] = true
			segs = append(segs, segment{kind: segWild, param: name})
			continue
		}
		if strings.ContainsAny(p, "{}") {
			return nil, fmt.Errorf("malformed parameter segment %q", p)
		}
		segs = append(segs, segment{kind: segStatic, text: p})
	}
	return segs, nil
}

// structuralKey renders a template ignoring parameter names so that
// structurally identical templates can be detected as conflicts.
func structuralKey(segs []segment) string {
	var b strings.Builder
	for _, s := range segs {
		switch s.kind {
		case segStatic:
			b.WriteString("/S:")
			b.WriteString(s.text)
		case segParam:
			b.WriteString("/P")
		case segWild:
			b.WriteString("/*")
		}
	}
	return b.String()
}

// Compile validates decls and returns a catalog, or all problems found.
// On any problem the catalog is nil and nothing usable is returned.
func Compile(decls []Decl) (*Catalog, []Problem) {
	var problems []Problem
	var routes []*Route
	byName := map[string]*Route{}
	for i, d := range decls {
		line := i + 1
		r := &Route{Name: d.Name, Method: strings.ToUpper(d.Method), Path: d.Path, Line: line}
		bad := false
		report := func(issue string) {
			problems = append(problems, Problem{Route: d.Name, Line: line, Issue: issue})
			bad = true
		}
		if d.Name == "" {
			report("route name must be non-empty")
		} else if prev, ok := byName[d.Name]; ok {
			problems = append(problems, Problem{Route: d.Name, Line: line,
				Issue: fmt.Sprintf("duplicate route name, first declared at line %d", prev.Line)})
			bad = true
		}
		if d.Method == "" {
			report("method must be non-empty")
		}
		segs, err := parseTemplate(d.Path)
		if err != nil {
			report(err.Error())
		} else {
			r.segs = segs
		}
		if bad {
			continue
		}
		byName[d.Name] = r
		routes = append(routes, r)
	}
	// Structural conflicts within the same method, ignoring parameter names.
	type key struct {
		method, shape string
	}
	groups := map[key][]*Route{}
	for _, r := range routes {
		k := key{r.Method, structuralKey(r.segs)}
		groups[k] = append(groups[k], r)
	}
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(i, j int) bool { return g[i].Line < g[j].Line })
		for i := 1; i < len(g); i++ {
			problems = append(problems, Problem{Route: g[i].Name, Line: g[i].Line,
				Issue: fmt.Sprintf("conflicts with route %q (line %d): same method and template structure",
					g[0].Name, g[0].Line)})
		}
	}
	if len(problems) > 0 {
		sort.Slice(problems, func(i, j int) bool {
			if problems[i].Route != problems[j].Route {
				return problems[i].Route < problems[j].Route
			}
			return problems[i].Line < problems[j].Line
		})
		return nil, problems
	}
	return &Catalog{routes: routes, byName: byName}, nil
}
