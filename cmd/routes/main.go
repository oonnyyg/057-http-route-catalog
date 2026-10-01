// Command routes is a line-delimited JSON tool over the route catalog.
// It reads one JSON request per line from stdin and writes one JSON
// response per line to stdout. It never starts an HTTP server.
//
// Requests:
//
//	{"op":"load","catalog":"NAME METHOD PATH\n..."}   compile & swap catalog
//	{"op":"check","catalog":"..."}                     validate without swap
//	{"op":"match","method":"GET","path":"/a/b?x=1"}    match a request
//	{"op":"link","name":"r","params":{...},"query":{...}}  build a link
//
// Every response has an "ok" boolean; failures carry an "error" string
// and, where applicable, structured "issues" or "methods" fields.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"example.com/http-route-catalog/routes"
)

type request struct {
	Op      string              `json:"op"`
	Catalog string              `json:"catalog"`
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Name    string              `json:"name"`
	Params  map[string]any      `json:"params"`
	Query   map[string][]string `json:"query"`
}

type response struct {
	OK      bool           `json:"ok"`
	Error   string         `json:"error,omitempty"`
	Issues  []routes.Issue `json:"issues,omitempty"`
	Routes  int            `json:"routes,omitempty"`
	Route   string         `json:"route,omitempty"`
	Values  map[string]any `json:"values,omitempty"`
	Methods []string       `json:"methods,omitempty"`
	URL     string         `json:"url,omitempty"`
}

type server struct {
	catalog *routes.Catalog
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "routes:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	s := &server{}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := enc.Encode(response{OK: false, Error: "invalid JSON request: " + err.Error()}); err != nil {
				return err
			}
			continue
		}
		if err := enc.Encode(s.handle(&req)); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *server) handle(req *request) response {
	switch req.Op {
	case "load":
		c, err := routes.Compile(req.Catalog)
		if err != nil {
			// The invalid catalog is rejected; the current one stays.
			return response{OK: false, Error: err.Error(), Issues: routes.Check(req.Catalog)}
		}
		s.catalog = c
		return response{OK: true, Routes: c.Len()}
	case "check":
		issues := routes.Check(req.Catalog)
		if len(issues) > 0 {
			return response{OK: false, Error: "catalog has validation issues", Issues: issues}
		}
		return response{OK: true}
	case "match":
		if s.catalog == nil {
			return response{OK: false, Error: "no catalog loaded"}
		}
		m, err := s.catalog.Match(req.Method, req.Path)
		if err != nil {
			resp := response{OK: false, Error: err.Error()}
			var mna *routes.MethodNotAllowedError
			if errors.As(err, &mna) {
				resp.Methods = mna.Methods
			}
			return resp
		}
		return response{OK: true, Route: m.Route, Values: m.Values}
	case "link":
		if s.catalog == nil {
			return response{OK: false, Error: "no catalog loaded"}
		}
		u, err := s.catalog.Link(req.Name, req.Params, req.Query)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return response{OK: true, URL: u}
	default:
		return response{OK: false, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
}
