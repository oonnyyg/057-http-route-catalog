package routes

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// MatchResult is a successful match: the route name plus extracted values.
// Single-segment parameters map to a string; multi-segment parameters map
// to an ordered []string of segments.
type MatchResult struct {
	Route  string
	Values map[string]any
}

// MethodNotAllowedError reports a path that exists under other methods.
type MethodNotAllowedError struct {
	Path    string
	Methods []string // sorted, de-duplicated
}

func (e *MethodNotAllowedError) Error() string {
	return fmt.Sprintf("method not allowed for %s (allowed: %s)", e.Path, strings.Join(e.Methods, ", "))
}

// NotFoundError reports a path that matches no route under any method.
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no route matches path %s", e.Path)
}

// decodeSegment percent-decodes one raw path segment exactly once.
func decodeSegment(raw string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+2 > len(raw)-1 {
			return "", fmt.Errorf("invalid percent encoding in segment %q", raw)
		}
		hi, ok1 := unhex(raw[i+1])
		lo, ok2 := unhex(raw[i+2])
		if !ok1 || !ok2 {
			return "", fmt.Errorf("invalid percent encoding in segment %q", raw)
		}
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	s := b.String()
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("invalid UTF-8 in segment %q", raw)
	}
	if strings.ContainsAny(s, "/\\") {
		return "", fmt.Errorf("decoded segment %q contains a path separator", s)
	}
	if s == "." || s == ".." {
		return "", fmt.Errorf("decoded segment %q is not allowed", s)
	}
	return s, nil
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// splitRequestPath validates and decodes a request path into segments.
// The query string is stripped first; a trailing slash is ignored except
// for the root path.
func splitRequestPath(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("request path must start with '/'")
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	body := strings.TrimPrefix(path, "/")
	body = strings.TrimSuffix(body, "/")
	if body == "" {
		return nil, nil // root
	}
	rawSegs := strings.Split(body, "/")
	segs := make([]string, 0, len(rawSegs))
	for _, raw := range rawSegs {
		if raw == "" {
			return nil, fmt.Errorf("request path contains consecutive slashes")
		}
		s, err := decodeSegment(raw)
		if err != nil {
			return nil, err
		}
		segs = append(segs, s)
	}
	return segs, nil
}

type candidate struct {
	route  *Route
	values map[string]any
}

// matchRoute attempts to match decoded segments against one route.
func matchRoute(r *Route, segs []string) (map[string]any, bool) {
	values := map[string]any{}
	i := 0
	for _, rs := range r.segs {
		switch rs.kind {
		case segLiteral:
			if i >= len(segs) || segs[i] != rs.text {
				return nil, false
			}
			i++
		case segSingle:
			if i >= len(segs) {
				return nil, false
			}
			values[rs.name] = segs[i]
			i++
		case segMulti:
			rest := append([]string{}, segs[i:]...)
			values[rs.name] = rest
			i = len(segs)
		}
	}
	if i != len(segs) {
		return nil, false
	}
	return values, true
}

// segRank orders segment kinds for priority: literal < single < multi.
func segRank(k int) int { return k }

// less reports whether candidate a has higher priority than b for the same
// request. At the first differing segment, a literal beats a single-segment
// parameter, which beats a multi-segment parameter. A route that ends
// exactly beats one that ends with an empty multi-segment capture.
func less(a, b *Route) bool {
	n := len(a.segs)
	if len(b.segs) < n {
		n = len(b.segs)
	}
	for i := 0; i < n; i++ {
		ra, rb := segRank(a.segs[i].kind), segRank(b.segs[i].kind)
		if ra != rb {
			return ra < rb
		}
	}
	if len(a.segs) != len(b.segs) {
		// The shorter one ends exactly; the longer one can only be longer
		// via a trailing multi-segment parameter capturing zero segments.
		return len(a.segs) < len(b.segs)
	}
	// Fully identical structure: cannot happen in a valid catalog
	// (conflict), but keep a deterministic tiebreak on name.
	return a.Name < b.Name
}

// Match finds the best route for method+path. It returns
// *MethodNotAllowedError when the path exists under other methods, and
// *NotFoundError when no route matches the path at all.
func (c *Catalog) Match(method, path string) (*MatchResult, error) {
	segs, err := splitRequestPath(path)
	if err != nil {
		return nil, err
	}
	method = strings.ToUpper(method)
	var best *candidate
	var otherMethods []string
	seenMethod := map[string]bool{}
	for _, r := range c.routes {
		values, ok := matchRoute(r, segs)
		if !ok {
			continue
		}
		if r.Method != method {
			if !seenMethod[r.Method] {
				seenMethod[r.Method] = true
				otherMethods = append(otherMethods, r.Method)
			}
			continue
		}
		if best == nil || less(r, best.route) {
			best = &candidate{route: r, values: values}
		}
	}
	if best != nil {
		return &MatchResult{Route: best.route.Name, Values: best.values}, nil
	}
	if len(otherMethods) > 0 {
		sort.Strings(otherMethods)
		return nil, &MethodNotAllowedError{Path: path, Methods: otherMethods}
	}
	return nil, &NotFoundError{Path: path}
}
