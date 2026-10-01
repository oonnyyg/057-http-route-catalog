package routes

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func mustCompile(t *testing.T, src string) *Catalog {
	t.Helper()
	c, err := Compile(src)
	if err != nil {
		t.Fatalf("Compile(%q): %v", src, err)
	}
	return c
}

func TestValidationRejectsBadTemplates(t *testing.T) {
	bad := []string{
		"r GET",                  // wrong field count
		"r GET noslash",          // must start with /
		"r GET /a//b",            // consecutive slashes
		"r GET /a%20b",           // percent sign
		"r GET /a\\b",            // backslash
		"r GET /a?x=1",           // query string
		"r GET /a#frag",          // fragment
		"r GET /a/./b",           // dot segment
		"r GET /a/../b",          // dot-dot segment
		"r GET /a/*rest/b",       // multi not at end
		"r GET /a/*",             // empty multi name
		"r GET /a/*rest!",        // bad multi name
		"r GET /a/{}",            // empty param name
		"r GET /a/{x-y}",         // bad param name
		"r GET /a/{x}/{x}",       // duplicate param in route
		"r GET /a/{x",            // malformed parameter
		"r GET /a/x}",            // malformed parameter
		"dup GET /a\ndup GET /b", // duplicate route name
	}
	for _, src := range bad {
		if _, err := Compile(src); err == nil {
			t.Errorf("Compile(%q) succeeded, want error", src)
		}
	}
}

func TestConflictDetection(t *testing.T) {
	src := "a GET /u/{id}\nb GET /u/{name}\nc POST /u/{id}"
	issues := Check(src)
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %v", len(issues), issues)
	}
	is := issues[0]
	if is.Route != "b" || !strings.Contains(is.Message, `"a"`) {
		t.Errorf("issue should name both routes: %+v", is)
	}
	if is.Line != 2 || !strings.Contains(is.Message, "line 1") {
		t.Errorf("issue should carry both positions: %+v", is)
	}
}

func TestCheckReportSortedByRouteName(t *testing.T) {
	src := "z GET /a//b\na GET /c//d\nm GET /e//f"
	issues := Check(src)
	if len(issues) != 3 {
		t.Fatalf("got %d issues, want 3", len(issues))
	}
	if issues[0].Route != "a" || issues[1].Route != "m" || issues[2].Route != "z" {
		t.Errorf("issues not sorted by route name: %v", issues)
	}
}

func TestMatchPriority(t *testing.T) {
	c := mustCompile(t, strings.Join([]string{
		"multi GET /f/*rest",
		"single GET /f/{id}",
		"literal GET /f/me",
	}, "\n"))
	for path, want := range map[string]string{
		"/f/me":     "literal",
		"/f/you":    "single",
		"/f/a/b":    "multi",
		"/f":        "multi", // zero-segment multi capture
		"/f/":       "multi", // trailing slash ignored
		"/f/me?x=1": "literal",
	} {
		m, err := c.Match("GET", path)
		if err != nil {
			t.Fatalf("Match(%q): %v", path, err)
		}
		if m.Route != want {
			t.Errorf("Match(%q) = %s, want %s", path, m.Route, want)
		}
	}
	// Order independence: same catalog reversed.
	c2 := mustCompile(t, strings.Join([]string{
		"literal GET /f/me",
		"single GET /f/{id}",
		"multi GET /f/*rest",
	}, "\n"))
	m, _ := c2.Match("GET", "/f/me")
	if m.Route != "literal" {
		t.Errorf("reversed catalog: Match(/f/me) = %s, want literal", m.Route)
	}
}

func TestMatchCompleteBeatsEmptyMulti(t *testing.T) {
	c := mustCompile(t, "exact GET /a\nwild GET /a/*rest")
	m, err := c.Match("GET", "/a")
	if err != nil || m.Route != "exact" {
		t.Fatalf("Match(/a) = %v, %v; want exact", m, err)
	}
}

func TestMatchExtractsValues(t *testing.T) {
	c := mustCompile(t, "r GET /u/{user}/files/*rest")
	m, err := c.Match("GET", "/u/bob/files/a/b/c")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"user": "bob", "rest": []string{"a", "b", "c"}}
	if !reflect.DeepEqual(m.Values, want) {
		t.Errorf("values = %v, want %v", m.Values, want)
	}
	// Zero-segment multi capture.
	m, err = c.Match("GET", "/u/bob/files")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Values["rest"], []string{}) {
		t.Errorf("rest = %v, want empty list", m.Values["rest"])
	}
}

func TestRequestPathDecoding(t *testing.T) {
	c := mustCompile(t, "r GET /u/{name}")
	m, err := c.Match("GET", "/u/%E4%BD%A0%E5%A5%BD")
	if err != nil {
		t.Fatal(err)
	}
	if m.Values["name"] != "你好" {
		t.Errorf("name = %q", m.Values["name"])
	}
	bad := []string{
		"noslash",
		"/u//x",
		"/u/%zz",
		"/u/%2f", // decoded separator
		"/u/%5C", // decoded backslash
		"/u/%ff", // invalid UTF-8
		"/u/%2e", // decoded dot
		"/u/%2e%2e",
		"/u/.",
		"/u/..",
	}
	for _, p := range bad {
		if _, err := c.Match("GET", p); err == nil {
			t.Errorf("Match(%q) succeeded, want error", p)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	c := mustCompile(t, "g GET /a\np POST /a\np2 POST /a/{x}")
	_, err := c.Match("DELETE", "/a")
	var mna *MethodNotAllowedError
	if !errors.As(err, &mna) {
		t.Fatalf("err = %v, want MethodNotAllowedError", err)
	}
	if !reflect.DeepEqual(mna.Methods, []string{"GET", "POST"}) {
		t.Errorf("methods = %v", mna.Methods)
	}
	// Not found is distinct.
	_, err = c.Match("GET", "/nope")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestRootAndTrailingSlash(t *testing.T) {
	c := mustCompile(t, "root GET /\na GET /a/")
	if m, err := c.Match("GET", "/"); err != nil || m.Route != "root" {
		t.Errorf("Match(/) = %v, %v", m, err)
	}
	if m, err := c.Match("GET", "/a/"); err != nil || m.Route != "a" {
		t.Errorf("Match(/a/) = %v, %v", m, err)
	}
}
