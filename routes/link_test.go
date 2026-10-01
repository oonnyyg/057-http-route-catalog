package routes

import (
	"strings"
	"testing"
)

func TestLinkRoundTrip(t *testing.T) {
	c := mustCompile(t, "u GET /u/{user}/files/*rest")
	url, err := c.Link("u", map[string]any{
		"user": "bob",
		"rest": []string{"a b", "c&d"},
	}, map[string][]string{"q": {"x", "y"}, "page": {"2"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "/u/bob/files/a%20b/c&d?") {
		t.Errorf("url = %q", url)
	}
	if !strings.Contains(url, "page=2") || !strings.Contains(url, "q=x&q=y") {
		t.Errorf("query not sorted/ordered: %q", url)
	}
	if strings.Index(url, "page=2") > strings.Index(url, "q=x") {
		t.Errorf("query keys not sorted: %q", url)
	}
}

func TestLinkEmptyMulti(t *testing.T) {
	c := mustCompile(t, "r GET /a/*rest")
	url, err := c.Link("r", map[string]any{"rest": []string{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if url != "/a" {
		t.Errorf("url = %q, want /a", url)
	}
}

func TestLinkRejects(t *testing.T) {
	c := mustCompile(t, "u GET /u/{user}\nf GET /f/*rest")
	cases := []struct {
		name   string
		params map[string]any
	}{
		{"missing", map[string]any{}},
		{"unknown", map[string]any{"user": "a", "extra": "b"}},
		{"empty-single", map[string]any{"user": ""}},
		{"separator", map[string]any{"user": "a/b"}},
		{"backslash", map[string]any{"user": "a\\b"}},
		{"dot", map[string]any{"user": "."}},
		{"dotdot", map[string]any{"user": ".."}},
	}
	for _, tc := range cases {
		if _, err := c.Link("u", tc.params, nil); err == nil {
			t.Errorf("Link(%s) succeeded, want error", tc.name)
		}
	}
	if _, err := c.Link("f", map[string]any{"rest": []string{"a", ""}}, nil); err == nil {
		t.Error("Link with empty multi item succeeded, want error")
	}
	if _, err := c.Link("f", map[string]any{"rest": []string{"a/b"}}, nil); err == nil {
		t.Error("Link with separator in multi item succeeded, want error")
	}
	if _, err := c.Link("nope", nil, nil); err == nil {
		t.Error("Link with unknown route succeeded, want error")
	}
}

func TestLinkRefusesIntercepted(t *testing.T) {
	// /f/me is claimed by the literal route; a link for the {id} route
	// with id="me" would be intercepted and must be refused.
	c := mustCompile(t, "lit GET /f/me\nid GET /f/{id}")
	if _, err := c.Link("id", map[string]any{"id": "me"}, nil); err == nil {
		t.Error("Link succeeded for intercepted path, want error")
	}
	url, err := c.Link("id", map[string]any{"id": "you"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if url != "/f/you" {
		t.Errorf("url = %q", url)
	}
}

func TestLinkEncodingRoundTrip(t *testing.T) {
	c := mustCompile(t, "r GET /u/{name}")
	url, err := c.Link("r", map[string]any{"name": "你好 ?"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.Match("GET", url)
	if err != nil {
		t.Fatal(err)
	}
	if m.Route != "r" || m.Values["name"] != "你好 ?" {
		t.Errorf("round trip = %v %v", m.Route, m.Values)
	}
}
