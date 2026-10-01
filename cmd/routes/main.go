// Command routes is a local route-catalog compiler. It reads one JSON
// request per line from stdin and writes one JSON response per line to
// stdout. It never starts an HTTP server. Supported operations:
//
//	{"op":"load","routes":[{"name":"n","method":"get","path":"/x/{id}"}]}
//	{"op":"check","routes":[...]}   // validate without replacing
//	{"op":"check"}                  // report on the current catalog
//	{"op":"match","method":"GET","path":"/x/1?a=b"}
//	{"op":"link","name":"n","params":{"id":["1"]},"query":{"q":["a","b"]}}
//
// A failed load never replaces the current catalog.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"example.com/http-route-catalog/routes"
)

type request struct {
	Op     string              `json:"op"`
	Routes []routes.Decl       `json:"routes"`
	Method string              `json:"method"`
	Path   string              `json:"path"`
	Name   string              `json:"name"`
	Params map[string][]string `json:"params"`
	Query  map[string][]string `json:"query"`
}

type routeInfo struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
}

type response struct {
	OK      bool                `json:"ok"`
	Error   string              `json:"error,omitempty"`
	Reason  string              `json:"reason,omitempty"`
	Methods []string            `json:"methods,omitempty"`
	Problems []routes.Problem   `json:"problems,omitempty"`
	Routes  []routeInfo         `json:"routes,omitempty"`
	Route   string              `json:"route,omitempty"`
	Values  map[string][]string `json:"values,omitempty"`
	URL     string              `json:"url,omitempty"`
}

func catalogInfo(c *routes.Catalog) []routeInfo {
	var out []routeInfo
	for _, r := range c.Routes() {
		out = append(out, routeInfo{Name: r.Name, Method: r.Method, Path: r.Path, Line: r.Line})
	}
	return out
}

func handle(current *routes.Catalog, req *request) (*routes.Catalog, response) {
	switch req.Op {
	case "load":
		c, problems := routes.Compile(req.Routes)
		if problems != nil {
			return current, response{OK: false, Error: "invalid catalog; current catalog unchanged", Problems: problems}
		}
		return c, response{OK: true, Routes: catalogInfo(c)}
	case "check":
		if req.Routes != nil {
			_, problems := routes.Compile(req.Routes)
			if problems != nil {
				return current, response{OK: false, Error: "invalid catalog", Problems: problems}
			}
			return current, response{OK: true}
		}
		if current == nil {
			return current, response{OK: false, Error: "no catalog loaded"}
		}
		return current, response{OK: true, Routes: catalogInfo(current)}
	case "match":
		if current == nil {
			return current, response{OK: false, Error: "no catalog loaded"}
		}
		res, merr := current.Match(req.Method, req.Path)
		if merr != nil {
			return current, response{OK: false, Error: merr.Error(), Reason: merr.Reason, Methods: merr.Methods}
		}
		return current, response{OK: true, Route: res.Route, Values: res.Values}
	case "link":
		if current == nil {
			return current, response{OK: false, Error: "no catalog loaded"}
		}
		u, err := current.Link(req.Name, req.Params, req.Query)
		if err != nil {
			return current, response{OK: false, Error: err.Error()}
		}
		return current, response{OK: true, URL: u}
	default:
		return current, response{OK: false, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
}

func main() {
	var catalog *routes.Catalog
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		var resp response
		if err := json.Unmarshal(line, &req); err != nil {
			resp = response{OK: false, Error: "invalid JSON request: " + err.Error()}
		} else {
			catalog, resp = handle(catalog, &req)
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "routes: encode response:", err)
			os.Exit(1)
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "routes: read stdin:", err)
		os.Exit(1)
	}
}
