package routes

import (
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
)

// Link builds a URL for the named route. params holds path parameters:
// a string for single-segment parameters and a []string (possibly empty)
// for multi-segment parameters. query holds independent query values,
// emitted sorted by name with per-name value order preserved.
//
// The generated path is matched again with the route's own method; if it
// does not resolve back to the same route with the same extracted values,
// an error is returned instead of a link that would be intercepted by a
// higher-priority route.
func (c *Catalog) Link(name string, params map[string]any, query map[string][]string) (string, error) {
	r, ok := c.byName[name]
	if !ok {
		return "", fmt.Errorf("unknown route %q", name)
	}
	want := map[string]bool{}
	for _, s := range r.segs {
		if s.kind != segLiteral {
			want[s.name] = true
		}
	}
	for k := range params {
		if !want[k] {
			return "", fmt.Errorf("route %q has no parameter %q", name, k)
		}
	}
	canonical := map[string]any{}
	var b strings.Builder
	for _, s := range r.segs {
		switch s.kind {
		case segLiteral:
			b.WriteByte('/')
			b.WriteString(url.PathEscape(s.text))
		case segSingle:
			v, err := stringParam(name, s.name, params)
			if err != nil {
				return "", err
			}
			canonical[s.name] = v
			b.WriteByte('/')
			b.WriteString(url.PathEscape(v))
		case segMulti:
			items, err := listParam(name, s.name, params)
			if err != nil {
				return "", err
			}
			canonical[s.name] = append([]string{}, items...)
			for _, item := range items {
				b.WriteByte('/')
				b.WriteString(url.PathEscape(item))
			}
		}
	}
	path := b.String()
	if path == "" {
		path = "/"
	}
	// Round-trip check: the link must dispatch back to this route.
	m, err := c.Match(r.Method, path)
	if err != nil {
		return "", fmt.Errorf("generated link %q does not match route %q: %v", path, name, err)
	}
	if m.Route != name {
		return "", fmt.Errorf("generated link %q is intercepted by route %q", path, m.Route)
	}
	for _, s := range r.segs {
		if s.kind == segLiteral {
			continue
		}
		if !reflect.DeepEqual(canonical[s.name], m.Values[s.name]) {
			return "", fmt.Errorf("generated link %q does not round-trip parameter %q", path, s.name)
		}
	}
	if len(query) > 0 {
		names := make([]string, 0, len(query))
		for k := range query {
			names = append(names, k)
		}
		sort.Strings(names)
		var q strings.Builder
		for _, k := range names {
			for _, v := range query[k] {
				if q.Len() == 0 {
					q.WriteByte('?')
				} else {
					q.WriteByte('&')
				}
				q.WriteString(url.QueryEscape(k))
				q.WriteByte('=')
				q.WriteString(url.QueryEscape(v))
			}
		}
		path += q.String()
	}
	return path, nil
}

func checkSegmentValue(route, param, v string) error {
	if v == "" {
		return fmt.Errorf("route %q parameter %q must not be empty", route, param)
	}
	if strings.ContainsAny(v, "/\\") {
		return fmt.Errorf("route %q parameter %q contains a path separator", route, param)
	}
	if v == "." || v == ".." {
		return fmt.Errorf("route %q parameter %q must not be %q", route, param, v)
	}
	return nil
}

func stringParam(route, param string, params map[string]any) (string, error) {
	v, ok := params[param]
	if !ok {
		return "", fmt.Errorf("route %q missing parameter %q", route, param)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("route %q parameter %q must be a string", route, param)
	}
	if err := checkSegmentValue(route, param, s); err != nil {
		return "", err
	}
	return s, nil
}

func listParam(route, param string, params map[string]any) ([]string, error) {
	v, ok := params[param]
	if !ok {
		return nil, fmt.Errorf("route %q missing parameter %q", route, param)
	}
	l, ok := v.([]string)
	if !ok {
		// JSON-decoded lists arrive as []any.
		anyList, isAny := v.([]any)
		if !isAny {
			return nil, fmt.Errorf("route %q parameter %q must be a list of strings", route, param)
		}
		l = make([]string, 0, len(anyList))
		for _, item := range anyList {
			s, isStr := item.(string)
			if !isStr {
				return nil, fmt.Errorf("route %q parameter %q must be a list of strings", route, param)
			}
			l = append(l, s)
		}
	}
	for _, item := range l {
		if err := checkSegmentValue(route, param, item); err != nil {
			return nil, err
		}
	}
	return l, nil
}
