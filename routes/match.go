package routes

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

// MatchResult is the outcome of a successful match.
type MatchResult struct {
	Route  string
	Values map[string][]string // single-segment params hold exactly one element
}

// MatchError describes why a request could not be dispatched.
type MatchError struct {
	Reason  string
	Detail  string
	Methods []string
}

func (e *MatchError) Error() string {
	if e.Reason == "method_not_allowed" {
		return fmt.Sprintf("method not allowed; allowed: %s", strings.Join(e.Methods, ", "))
	}
	if e.Detail != "" {
		return e.Reason + ": " + e.Detail
	}
	return e.Reason
}

// splitRequestPath validates and decodes a request path into segments.
func splitRequestPath(rawPath string) ([]string, *MatchError) {
	if !strings.HasPrefix(rawPath, "/") {
		return nil, &MatchError{Reason: "bad_request", Detail: "path must start with '/'"}
	}
	if i := strings.IndexByte(rawPath, '?'); i >= 0 {
		rawPath = rawPath[:i]
	}
	body := strings.TrimSuffix(rawPath, "/")
	if body == "" {
		return nil, nil // root
	}
	parts := strings.Split(body[1:], "/")
	segs := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			return nil, &MatchError{Reason: "bad_request", Detail: "path contains consecutive slashes"}
		}
		dec, err := url.PathUnescape(p)
		if err != nil {
			return nil, &MatchError{Reason: "bad_request", Detail: fmt.Sprintf("invalid percent-encoding in segment %q", p)}
		}
		if !utf8.ValidString(dec) {
			return nil, &MatchError{Reason: "bad_request", Detail: fmt.Sprintf("segment %q is not valid UTF-8", p)}
		}
		if strings.ContainsAny(dec, "/\\") {
			return nil, &MatchError{Reason: "bad_request", Detail: fmt.Sprintf("segment %q decodes to a path separator", p)}
		}
		if dec == "." || dec == ".." {
			return nil, &MatchError{Reason: "bad_request", Detail: fmt.Sprintf("segment %q decodes to a dot segment", p)}
		}
		segs = append(segs, dec)
	}
	return segs, nil
}

// matchRoute reports whether segs matches r, and extracts values.
func matchRoute(r *Route, segs []string) (map[string][]string, bool) {
	vals := map[string][]string{}
	i := 0
	for _, s := range r.segs {
		switch s.kind {
		case segStatic:
			if i >= len(segs) || segs[i] != s.text {
				return nil, false
			}
			i++
		case segParam:
			if i >= len(segs) {
				return nil, false
			}
			vals[s.param] = []string{segs[i]}
			i++
		case segWild:
			rest := make([]string, len(segs)-i)
			copy(rest, segs[i:])
			vals[s.param] = rest
			i = len(segs)
		}
	}
	if i != len(segs) {
		return nil, false
	}
	return vals, true
}

func kindRank(k int) int {
	switch k {
	case segStatic:
		return 0
	case segParam:
		return 1
	}
	return 2
}

// better reports whether route a is strictly preferred over route b,
// comparing at the first differing segment: static < param < wildcard,
// and a route that ends exactly beats an empty wildcard capture.
func better(a, b *Route) bool {
	n := len(a.segs)
	if len(b.segs) < n {
		n = len(b.segs)
	}
	for i := 0; i < n; i++ {
		ra, rb := kindRank(a.segs[i].kind), kindRank(b.segs[i].kind)
		if ra != rb {
			return ra < rb
		}
	}
	return len(a.segs) < len(b.segs)
}

// Match finds the best route for method and rawPath. The result is
// independent of catalog load order.
func (c *Catalog) Match(method, rawPath string) (*MatchResult, *MatchError) {
	segs, merr := splitRequestPath(rawPath)
	if merr != nil {
		return nil, merr
	}
	method = strings.ToUpper(method)
	var best *Route
	var bestVals map[string][]string
	allowed := map[string]bool{}
	for _, r := range c.routes {
		vals, ok := matchRoute(r, segs)
		if !ok {
			continue
		}
		if r.Method != method {
			allowed[r.Method] = true
			continue
		}
		if best == nil || better(r, best) {
			best, bestVals = r, vals
		}
	}
	if best != nil {
		return &MatchResult{Route: best.Name, Values: bestVals}, nil
	}
	if len(allowed) > 0 {
		methods := make([]string, 0, len(allowed))
		for m := range allowed {
			methods = append(methods, m)
		}
		sort.Strings(methods)
		return nil, &MatchError{Reason: "method_not_allowed", Methods: methods}
	}
	return nil, &MatchError{Reason: "not_found"}
}
