package routes

import (
	"strings"
	"testing"
)

func mustCompile(t *testing.T, decls ...Decl) *Catalog {
	t.Helper()
	c, problems := Compile(decls)
	if problems != nil {
		t.Fatalf("unexpected problems: %+v", problems)
	}
	return c
}

func mustFail(t *testing.T, substr string, decls ...Decl) []Problem {
	t.Helper()
	c, problems := Compile(decls)
	if c != nil {
		t.Fatalf("expected failure, got catalog")
	}
	for _, p := range problems {
		if strings.Contains(p.Issue, substr) {
			return problems
		}
	}
	t.Fatalf("no problem containing %q in %+v", substr, problems)
	return nil
}

func TestCompileValid(t *testing.T) {
	c := mustCompile(t,
		Decl{Name: "root", Method: "get", Path: "/"},
		Decl{Name: "user", Method: "GET", Path: "/users/{user}"},
		Decl{Name: "files", Method: "GET", Path: "/files/*rest"},
	)
	if got := c.Routes(); got[0].Method != "GET" {
		t.Fatalf("method not upper-cased: %q", got[0].Method)
	}
	names := []string{c.Routes()[0].Name, c.Routes()[1].Name, c.Routes()[2].Name}
	if names[0] != "files" || names[1] != "root" || names[2] != "user" {
		t.Fatalf("routes not sorted by name: %v", names)
	}
}

func TestCompileRejects(t *testing.T) {
	cases := []struct {
		substr string
		decl   Decl
	}{
		{"route name", Decl{Name: "", Method: "GET", Path: "/a"}},
		{"method", Decl{Name: "a", Method: "", Path: "/a"}},
		{"start with '/'", Decl{Name: "a", Method: "GET", Path: "a"}},
		{"consecutive slashes", Decl{Name: "a", Method: "GET", Path: "/a//b"}},
		{"'%'", Decl{Name: "a", Method: "GET", Path: "/a%20"}},
		{"'\\'", Decl{Name: "a", Method: "GET", Path: "/a\\b"}},
		{"query string", Decl{Name: "a", Method: "GET", Path: "/a?x=1"}},
		{"fragment", Decl{Name: "a", Method: "GET", Path: "/a#x"}},
		{"dot segment", Decl{Name: "a", Method: "GET", Path: "/a/../b"}},
		{"malformed parameter", Decl{Name: "a", Method: "GET", Path: "/{}"}},
		{"parameter name", Decl{Name: "a", Method: "GET", Path: "/{bad-name}"}},
		{"duplicate parameter", Decl{Name: "a", Method: "GET", Path: "/{x}/{x}"}},
		{"must be the last segment", Decl{Name: "a", Method: "GET", Path: "/*rest/more"}},
		{"wildcard parameter name", Decl{Name: "a", Method: "GET", Path: "/*"}},
	}
	for _, tc := range cases {
		mustFail(t, tc.substr, tc.decl)
	}
}

func TestDuplicateName(t *testing.T) {
	problems := mustFail(t, "duplicate route name",
		Decl{Name: "a", Method: "GET", Path: "/x"},
		Decl{Name: "a", Method: "POST", Path: "/y"})
	found := false
	for _, p := range problems {
		if p.Line == 2 && strings.Contains(p.Issue, "line 1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected both positions reported: %+v", problems)
	}
}

func TestStructuralConflicts(t *testing.T) {
	// Identical templates and templates differing only in parameter names
	// conflict within the same method.
	problems := mustFail(t, "conflicts with route",
		Decl{Name: "a", Method: "GET", Path: "/u/{id}"},
		Decl{Name: "b", Method: "GET", Path: "/u/{id}"},
		Decl{Name: "c", Method: "GET", Path: "/u/{name}"},
		Decl{Name: "d", Method: "POST", Path: "/u/{id}"}, // different method: fine
		Decl{Name: "e", Method: "GET", Path: "/u/{id}/x"})
	count := 0
	for _, p := range problems {
		if strings.Contains(p.Issue, "conflicts with route") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 conflict reports, got %+v", problems)
	}
}

func TestTrailingSlashTemplatesEquivalent(t *testing.T) {
	mustFail(t, "conflicts with route",
		Decl{Name: "a", Method: "GET", Path: "/x"},
		Decl{Name: "b", Method: "GET", Path: "/x/"})
}

func TestProblemsSortedByName(t *testing.T) {
	_, problems := Compile([]Decl{
		{Name: "z", Method: "", Path: "/a"},
		{Name: "a", Method: "", Path: "/b"},
	})
	if len(problems) != 2 || problems[0].Route != "a" || problems[1].Route != "z" {
		t.Fatalf("problems not sorted by route name: %+v", problems)
	}
}
