package routes

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

func checkSegmentValue(name, v string) error {
	if v == "" {
		return fmt.Errorf("parameter %q: value must be non-empty", name)
	}
	if strings.ContainsAny(v, "/\\") {
		return fmt.Errorf("parameter %q: value %q contains a path separator", name, v)
	}
	if v == "." || v == ".." {
		return fmt.Errorf("parameter %q: dot segment %q is not allowed", name, v)
	}
	return nil
}

// Link builds a URL for the named route. params maps parameter names to
// values: exactly one value for single-segment params, an ordered (possibly
// empty) list for wildcards. query is encoded independently, sorted by name,
// preserving the order of repeated values. The generated path is matched
// again with the route's method and must round-trip to the same route and
// values, otherwise an error is returned instead of a hijacked link.
func (c *Catalog) Link(name string, params map[string][]string, query map[string][]string) (string, error) {
	r, ok := c.byName[name]
	if !ok {
		return "", fmt.Errorf("unknown route %q", name)
	}
	want := map[string]bool{}
	for _, s := range r.segs {
		if s.kind != segStatic {
			want[s.param] = true
		}
	}
	for p := range params {
		if !want[p] {
			return "", fmt.Errorf("route %q: unexpected parameter %q", name, p)
		}
	}
	for p := range want {
		if _, ok := params[p]; !ok {
			return "", fmt.Errorf("route %q: missing parameter %q", name, p)
		}
	}
	var b strings.Builder
	for _, s := range r.segs {
		switch s.kind {
		case segStatic:
			b.WriteByte('/')
			b.WriteString(s.text)
		case segParam:
			vals := params[s.param]
			if len(vals) != 1 {
				return "", fmt.Errorf("route %q: parameter %q expects exactly one value", name, s.param)
			}
			if err := checkSegmentValue(s.param, vals[0]); err != nil {
				return "", err
			}
			b.WriteByte('/')
			b.WriteString(url.PathEscape(vals[0]))
		case segWild:
			for _, v := range params[s.param] {
				if err := checkSegmentValue(s.param, v); err != nil {
					return "", err
				}
				b.WriteByte('/')
				b.WriteString(url.PathEscape(v))
			}
		}
	}
	path := b.String()
	if path == "" {
		path = "/"
	}
	if len(query) > 0 {
		names := make([]string, 0, len(query))
		for n := range query {
			names = append(names, n)
		}
		sort.Strings(names)
		var q []string
		for _, n := range names {
			for _, v := range query[n] {
				q = append(q, url.QueryEscape(n)+"="+url.QueryEscape(v))
			}
		}
		path += "?" + strings.Join(q, "&")
	}
	res, merr := c.Match(r.Method, path)
	if merr != nil {
		return "", fmt.Errorf("route %q: generated path does not match: %s", name, merr)
	}
	if res.Route != name {
		return "", fmt.Errorf("route %q: generated path is claimed by route %q", name, res.Route)
	}
	for p := range want {
		got := res.Values[p]
		exp := params[p]
		if len(got) != len(exp) {
			return "", fmt.Errorf("route %q: round-trip mismatch for parameter %q", name, p)
		}
		for i := range exp {
			if got[i] != exp[i] {
				return "", fmt.Errorf("route %q: round-trip mismatch for parameter %q", name, p)
			}
		}
	}
	return path, nil
}
