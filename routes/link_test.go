package routes

import (
	"strings"
	"testing"
)

func TestLinkBasic(t *testing.T) {
	c := mustCompile(t,
		Decl{Name: "user", Method: "GET", Path: "/u/{name}"},
		Decl{Name: "files", Method: "GET", Path: "/f/*rest"},
		Decl{Name: "root", Method: "GET", Path: "/"},
	)
	u, err := c.Link("user", map[string][]string{"name": {"a b"}}, nil)
	if err != nil || u != "/u/a%20b" {
		t.Fatalf("got %q, %v", u, err)
	}
	u, err = c.Link("files", map[string][]string{"rest": {"x", "中"}}, nil)
	if err != nil || u != "/f/x/%E4%B8%AD" {
		t.Fatalf("got %q, %v", u, err)
	}
	// Empty wildcard list is allowed.
	u, err = c.Link("files", map[string][]string{"rest": {}}, nil)
	if err != nil || u != "/f" {
		t.Fatalf("got %q, %v", u, err)
	}
	u, err = c.Link("root", nil, nil)
	if err != nil || u != "/" {
		t.Fatalf("got %q, %v", u, err)
	}
}

func TestLinkQuery(t *testing.T) {
	c := mustCompile(t, Decl{Name: "s", Method: "GET", Path: "/s"})
	u, err := c.Link("s", nil, map[string][]string{
		"b": {"2"},
		"a": {"x y", "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if u != "/s?a=x+y&a=1&b=2" {
		t.Fatalf("query not sorted/order-preserving: %q", u)
	}
}

func TestLinkRejects(t *testing.T) {
	c := mustCompile(t,
		Decl{Name: "user", Method: "GET", Path: "/u/{name}"},
		Decl{Name: "files", Method: "GET", Path: "/f/*rest"},
	)
	cases := []struct {
		name   string
		route  string
		params map[string][]string
		substr string
	}{
		{"unknown", "nope", nil, "unknown route"},
		{"missing", "user", nil, "missing parameter"},
		{"extra", "user", map[string][]string{"name": {"a"}, "x": {"1"}}, "unexpected parameter"},
		{"empty", "user", map[string][]string{"name": {""}}, "non-empty"},
		{"slash", "user", map[string][]string{"name": {"a/b"}}, "separator"},
		{"backslash", "user", map[string][]string{"name": {"a\\b"}}, "separator"},
		{"dot", "user", map[string][]string{"name": {".."}}, "dot segment"},
		{"multi-single", "user", map[string][]string{"name": {"a", "b"}}, "exactly one"},
		{"wild-empty-item", "files", map[string][]string{"rest": {"a", ""}}, "non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Link(tc.route, tc.params, nil)
			if err == nil || !strings.Contains(err.Error(), tc.substr) {
				t.Fatalf("expected error containing %q, got %v", tc.substr, err)
			}
		})
	}
}

func TestLinkRoundTripHijack(t *testing.T) {
	// A static route shadows the parameter route; linking to the param
	// route with the shadowed value must be refused.
	c := mustCompile(t,
		Decl{Name: "static", Method: "GET", Path: "/u/me"},
		Decl{Name: "user", Method: "GET", Path: "/u/{name}"},
	)
	_, err := c.Link("user", map[string][]string{"name": {"me"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "claimed by route") {
		t.Fatalf("expected hijack refusal, got %v", err)
	}
	u, err := c.Link("user", map[string][]string{"name": {"you"}}, nil)
	if err != nil || u != "/u/you" {
		t.Fatalf("got %q, %v", u, err)
	}
}

func TestLinkRoundTripValues(t *testing.T) {
	c := mustCompile(t, Decl{Name: "f", Method: "GET", Path: "/f/{a}/*rest"})
	u, err := c.Link("f", map[string][]string{"a": {"x y"}, "rest": {"p", "q"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, merr := c.Match("GET", u)
	if merr != nil {
		t.Fatal(merr)
	}
	if res.Route != "f" || res.Values["a"][0] != "x y" ||
		res.Values["rest"][0] != "p" || res.Values["rest"][1] != "q" {
		t.Fatalf("round-trip mismatch: %+v", res)
	}
}
