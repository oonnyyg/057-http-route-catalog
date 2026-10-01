package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runSession(t *testing.T, lines ...string) []response {
	t.Helper()
	var out bytes.Buffer
	if err := run(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var resps []response
	sc := bufio.NewScanner(&out)
	for sc.Scan() {
		var r response
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("bad response JSON %q: %v", sc.Text(), err)
		}
		resps = append(resps, r)
	}
	return resps
}

func TestCLILoadMatchLink(t *testing.T) {
	resps := runSession(t,
		`{"op":"load","catalog":"u GET /u/{user}\nf GET /f/*rest"}`,
		`{"op":"match","method":"GET","path":"/u/bob"}`,
		`{"op":"match","method":"POST","path":"/u/bob"}`,
		`{"op":"match","method":"GET","path":"/nope"}`,
		`{"op":"link","name":"u","params":{"user":"a b"},"query":{"x":["1","2"]}}`,
	)
	if !resps[0].OK || resps[0].Routes != 2 {
		t.Errorf("load: %+v", resps[0])
	}
	if !resps[1].OK || resps[1].Route != "u" || resps[1].Values["user"] != "bob" {
		t.Errorf("match: %+v", resps[1])
	}
	if resps[2].OK || len(resps[2].Methods) != 1 || resps[2].Methods[0] != "GET" {
		t.Errorf("method not allowed: %+v", resps[2])
	}
	if resps[3].OK {
		t.Errorf("not found: %+v", resps[3])
	}
	if !resps[4].OK || resps[4].URL != "/u/a%20b?x=1&x=2" {
		t.Errorf("link: %+v", resps[4])
	}
}

func TestCLIInvalidLoadKeepsCatalog(t *testing.T) {
	resps := runSession(t,
		`{"op":"load","catalog":"a GET /a"}`,
		`{"op":"load","catalog":"b GET /x//y"}`,
		`{"op":"match","method":"GET","path":"/a"}`,
		`{"op":"check","catalog":"c GET /p\nc POST /p"}`,
	)
	if !resps[0].OK {
		t.Fatalf("first load failed: %+v", resps[0])
	}
	if resps[1].OK || len(resps[1].Issues) == 0 {
		t.Fatalf("invalid load should fail with issues: %+v", resps[1])
	}
	if !resps[2].OK || resps[2].Route != "a" {
		t.Errorf("catalog was replaced by invalid load: %+v", resps[2])
	}
	if resps[3].OK {
		t.Errorf("check should report conflict: %+v", resps[3])
	}
}

func TestCLIBadRequests(t *testing.T) {
	resps := runSession(t,
		`not json`,
		`{"op":"bogus"}`,
		`{"op":"match","method":"GET","path":"/a"}`,
	)
	for i, r := range resps {
		if r.OK || r.Error == "" {
			t.Errorf("response %d should be an error: %+v", i, r)
		}
	}
}
