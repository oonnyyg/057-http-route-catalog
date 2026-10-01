package routes

import (
	"reflect"
	"testing"
)

func match(t *testing.T, c *Catalog, method, path string) *MatchResult {
	t.Helper()
	res, merr := c.Match(method, path)
	if merr != nil {
		t.Fatalf("Match(%s %s): %v", method, path, merr)
	}
	return res
}

func TestMatchPriority(t *testing.T) {
	c := mustCompile(t,
		Decl{Name: "wild", Method: "GET", Path: "/a/*rest"},
		Decl{Name: "param", Method: "GET", Path: "/a/{x}"},
		Decl{Name: "static", Method: "GET", Path: "/a/b"},
		Decl{Name: "exact", Method: "GET", Path: "/a"},
	)
	if r := match(t, c, "GET", "/a/b"); r.Route != "static" {
		t.Fatalf("static should win, got %q", r.Route)
	}
	if r := match(t, c, "GET", "/a/c"); r.Route != "param" {
		t.Fatalf("param should win, got %q", r.Route)
	}
	if r := match(t, c, "GET", "/a/b/c"); r.Route != "wild" {
		t.Fatalf("wild should win, got %q", r.Route)
	}
	// Exact end beats empty wildcard capture.
	if r := match(t, c, "GET", "/a"); r.Route != "exact" {
		t.Fatalf("exact end should win over empty wildcard, got %q", r.Route)
	}
}

func TestMatchWildcardZeroSegments(t *testing.T) {
	c := mustCompile(t, Decl{Name: "wild", Method: "GET", Path: "/f/*rest"})
	r := match(t, c, "GET", "/f")
	if r.Route != "wild" {
		t.Fatalf("got %q", r.Route)
	}
	if got := r.Values["rest"]; len(got) != 0 {
		t.Fatalf("expected empty capture, got %v", got)
	}
	r = match(t, c, "GET", "/f/x/y")
	if !reflect.DeepEqual(r.Values["rest"], []string{"x", "y"}) {
		t.Fatalf("ordered capture wrong: %v", r.Values["rest"])
	}
}

func TestMatchOrderIndependent(t *testing.T) {
	decls := []Decl{
		{Name: "wild", Method: "GET", Path: "/a/*rest"},
		{Name: "static", Method: "GET", Path: "/a/b"},
		{Name: "param", Method: "GET", Path: "/a/{x}"},
	}
	rev := []Decl{decls[2], decls[1], decls[0]}
	for _, ds := range [][]Decl{decls, rev} {
		c := mustCompile(t, ds...)
		if r := match(t, c, "GET", "/a/b"); r.Route != "static" {
			t.Fatalf("order-dependent result: %q", r.Route)
		}
	}
}

func TestMatchTrailingSlashAndQuery(t *testing.T) {
	c := mustCompile(t,
		Decl{Name: "root", Method: "GET", Path: "/"},
		Decl{Name: "x", Method: "GET", Path: "/x/"},
	)
	if r := match(t, c, "GET", "/x/"); r.Route != "x" {
		t.Fatalf("trailing slash not ignored: %q", r.Route)
	}
	if r := match(t, c, "GET", "/x?a=b&c=d"); r.Route != "x" {
		t.Fatalf("query not stripped: %q", r.Route)
	}
	if r := match(t, c, "GET", "/"); r.Route != "root" {
		t.Fatalf("root: %q", r.Route)
	}
}

func TestMatchDecoding(t *testing.T) {
	c := mustCompile(t, Decl{Name: "u", Method: "GET", Path: "/u/{name}"})
	r := match(t, c, "GET", "/u/a%20b")
	if got := r.Values["name"]; !reflect.DeepEqual(got, []string{"a b"}) {
		t.Fatalf("decoded value wrong: %v", got)
	}
	r = match(t, c, "GET", "/u/%E4%B8%AD")
	if got := r.Values["name"]; !reflect.DeepEqual(got, []string{"中"}) {
		t.Fatalf("UTF-8 decode wrong: %v", got)
	}
}

func TestMatchBadRequests(t *testing.T) {
	c := mustCompile(t, Decl{Name: "u", Method: "GET", Path: "/u/{name}"})
	cases := []struct {
		path   string
		detail string
	}{
		{"u/x", "start with '/'"},
		{"/u//x", "consecutive slashes"},
		{"/u/%zz", "percent-encoding"},
		{"/u/%2F", "path separator"},
		{"/u/%5C", "path separator"},
		{"/u/%2e%2e", "dot segment"},
		{"/u/..", "dot segment"},
		{"/u/%ff", "UTF-8"},
	}
	for _, tc := range cases {
		_, merr := c.Match("GET", tc.path)
		if merr == nil || merr.Reason != "bad_request" {
			t.Fatalf("path %q: expected bad_request, got %v", tc.path, merr)
		}
	}
}

func TestMatchNotFoundVsMethodNotAllowed(t *testing.T) {
	c := mustCompile(t,
		Decl{Name: "g", Method: "GET", Path: "/u/{id}"},
		Decl{Name: "p", Method: "POST", Path: "/u/{id}"},
		Decl{Name: "p2", Method: "post", Path: "/other"},
	)
	_, merr := c.Match("DELETE", "/u/1")
	if merr == nil || merr.Reason != "method_not_allowed" {
		t.Fatalf("expected method_not_allowed, got %v", merr)
	}
	if !reflect.DeepEqual(merr.Methods, []string{"GET", "POST"}) {
		t.Fatalf("allowed methods wrong: %v", merr.Methods)
	}
	_, merr = c.Match("GET", "/nope")
	if merr == nil || merr.Reason != "not_found" {
		t.Fatalf("expected not_found, got %v", merr)
	}
}

func TestMatchCaseSensitivePath(t *testing.T) {
	c := mustCompile(t, Decl{Name: "x", Method: "GET", Path: "/Ab"})
	if _, merr := c.Match("GET", "/ab"); merr == nil || merr.Reason != "not_found" {
		t.Fatalf("path matching must be case-sensitive: %v", merr)
	}
	if r := match(t, c, "get", "/Ab"); r.Route != "x" {
		t.Fatalf("method should be case-insensitive input: %q", r.Route)
	}
}
