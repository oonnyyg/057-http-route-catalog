// Package routes provides a compiled HTTP route catalog: it parses and
// validates route templates, detects structural conflicts, matches request
// paths, and generates reverse links from the same catalog.
package routes

import (
	"fmt"
	"sort"
	"strings"
)

// Segment kinds in a parsed template.
const (
	segLiteral = iota
	segSingle  // {name}
	segMulti   // *name, only allowed as the last segment
)

type segment struct {
	kind int
	text string // literal text for segLiteral
	name string // parameter name for segSingle/segMulti
}

// Route is one validated route declaration.
type Route struct {
	Name   string
	Method string
	Path   string
	Line   int // 1-based position in the catalog source
	segs   []segment
}

// Issue describes a single validation problem.
type Issue struct {
	Line    int    `json:"line"`
	Route   string `json:"route"`
	Message string `json:"message"`
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

// parseTemplate splits a path template into segments, enforcing syntax rules.
// Trailing slashes are ignored except for the root path "/".
func parseTemplate(path string) ([]segment, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("path template must start with '/'")
	}
	if strings.ContainsAny(path, "%\\") {
		return nil, fmt.Errorf("path template must not contain '%%' or '\\'")
	}
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("path template must not contain a query string or fragment")
	}
	body := strings.TrimPrefix(path, "/")
	body = strings.TrimSuffix(body, "/")
	if body == "" {
		return nil, nil // root
	}
	parts := strings.Split(body, "/")
	segs := make([]segment, 0, len(parts))
	for i, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("path template contains consecutive slashes")
		}
		if p == "." || p == ".." {
			return nil, fmt.Errorf("path template must not contain '.' or '..' segments")
		}
		if strings.HasPrefix(p, "*") {
			name := p[1:]
			if !validParamName(name) {
				return nil, fmt.Errorf("invalid multi-segment parameter name %q", name)
			}
			if i != len(parts)-1 {
				return nil, fmt.Errorf("multi-segment parameter *%s must be the last segment", name)
			}
			segs = append(segs, segment{kind: segMulti, name: name})
			continue
		}
		if strings.HasPrefix(p, "{") || strings.HasPrefix(p, "}") {
			if !strings.HasPrefix(p, "{") || !strings.HasSuffix(p, "}") || len(p) < 3 {
				return nil, fmt.Errorf("malformed parameter segment %q", p)
			}
			name := p[1 : len(p)-1]
			if !validParamName(name) {
				return nil, fmt.Errorf("invalid parameter name %q", name)
			}
			segs = append(segs, segment{kind: segSingle, name: name})
			continue
		}
		if strings.ContainsAny(p, "{}") {
			return nil, fmt.Errorf("malformed parameter segment %q", p)
		}
		segs = append(segs, segment{kind: segLiteral, text: p})
	}
	// No duplicate parameter names within one route.
	seen := map[string]bool{}
	for _, s := range segs {
		if s.kind == segLiteral {
			continue
		}
		if seen[s.name] {
			return nil, fmt.Errorf("duplicate parameter name %q", s.name)
		}
		seen[s.name] = true
	}
	return segs, nil
}

// structuralKey identifies a template ignoring parameter names.
func structuralKey(segs []segment) string {
	var b strings.Builder
	for _, s := range segs {
		switch s.kind {
		case segLiteral:
			b.WriteString("L:" + s.text + "/")
		case segSingle:
			b.WriteString("S/")
		case segMulti:
			b.WriteString("M/")
		}
	}
	return b.String()
}

// Catalog is a validated set of routes.
type Catalog struct {
	routes   []*Route
	byName   map[string]*Route
	byMethod map[string][]*Route
}

// Len returns the number of routes in the catalog.
func (c *Catalog) Len() int { return len(c.routes) }

// Check parses the catalog source (one route per line:
// "NAME METHOD PATH") and returns all validation issues, sorted by route
// name for a stable report. An empty result means the catalog is valid.
func Check(src string) []Issue {
	_, issues := parse(src)
	return issues
}

// Compile parses and validates the catalog source. It returns an error
// listing every issue if the catalog is invalid; no partial catalog is
// produced.
func Compile(src string) (*Catalog, error) {
	c, issues := parse(src)
	if len(issues) > 0 {
		var b strings.Builder
		b.WriteString("invalid route catalog:")
		for _, is := range issues {
			fmt.Fprintf(&b, "\n  line %d (%s): %s", is.Line, is.Route, is.Message)
		}
		return nil, fmt.Errorf("%s", b.String())
	}
	return c, nil
}

func parse(src string) (*Catalog, []Issue) {
	c := &Catalog{
		byName:   map[string]*Route{},
		byMethod: map[string][]*Route{},
	}
	var issues []Issue
	add := func(line int, route, msg string) {
		issues = append(issues, Issue{Line: line, Route: route, Message: msg})
	}
	lines := strings.Split(src, "\n")
	lineNo := 0
	for _, raw := range lines {
		raw = strings.TrimRight(raw, "\r")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		lineNo++
		fields := strings.Fields(raw)
		if len(fields) != 3 {
			add(lineNo, "", "expected 3 fields: NAME METHOD PATH")
			continue
		}
		name, method, path := fields[0], strings.ToUpper(fields[1]), fields[2]
		if fields[1] == "" {
			add(lineNo, name, "empty HTTP method")
			continue
		}
		segs, err := parseTemplate(path)
		if err != nil {
			add(lineNo, name, err.Error())
			continue
		}
		if _, dup := c.byName[name]; dup {
			add(lineNo, name, "duplicate route name")
			continue
		}
		r := &Route{Name: name, Method: method, Path: path, Line: lineNo, segs: segs}
		c.byName[name] = r
		c.routes = append(c.routes, r)
		c.byMethod[method] = append(c.byMethod[method], r)
	}
	// Structural conflicts within the same method.
	for _, rs := range c.byMethod {
		byKey := map[string][]*Route{}
		for _, r := range rs {
			k := structuralKey(r.segs)
			byKey[k] = append(byKey[k], r)
		}
		for _, group := range byKey {
			if len(group) < 2 {
				continue
			}
			sort.Slice(group, func(i, j int) bool { return group[i].Line < group[j].Line })
			first := group[0]
			for _, other := range group[1:] {
				add(other.Line, other.Name,
					fmt.Sprintf("conflicts with route %q (line %d): same method and template structure", first.Name, first.Line))
			}
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Route != issues[j].Route {
			return issues[i].Route < issues[j].Route
		}
		return issues[i].Line < issues[j].Line
	})
	return c, issues
}
