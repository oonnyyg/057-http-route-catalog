# http-route-catalog

A local route-catalog compiler for backend services. One catalog drives
both request dispatch and reverse link generation, so diagnostics always
match real routing behavior. Standard library only; no HTTP server is
started.

## Catalog format

One route per line, three whitespace-separated fields:

```
NAME METHOD PATH_TEMPLATE
```

- `NAME` — unique, non-empty route name.
- `METHOD` — HTTP method, normalized to uppercase.
- `PATH_TEMPLATE` — case-sensitive, must start with `/`. Segments:
  - literal: `users`
  - single-segment parameter: `{user}` (matches exactly one segment)
  - trailing multi-segment parameter: `*rest` (last segment only,
    matches zero or more segments)

Parameter names use letters, digits, and `_`, and must be non-empty and
unique within a route. Trailing slashes are ignored except for the root
path `/`.

Rejected: empty methods, duplicate route names, duplicate parameters in
one route, consecutive slashes, `%`, `\`, query strings, fragments, `.`
or `..` segments, and multi-segment parameters not at the end.

Two routes with the same method whose templates are structurally
identical (ignoring parameter names) conflict; the report names both
routes and their line positions.

## Matching

Request paths must start with `/`. The query string is stripped first,
then each segment is percent-decoded exactly once. Rejected: consecutive
slashes, invalid percent encodings, invalid UTF-8, decoded path
separators, and `.`/`..` segments.

Among all full matches, priority at the first differing segment is:
literal > single-segment parameter > multi-segment parameter; a route
that ends exactly beats one ending in an empty multi-segment capture.
Results do not depend on declaration order.

A successful match returns the route name and extracted values
(multi-segment values are ordered lists). A path that matches under
other methods yields a method-not-allowed error listing all matching
methods (sorted, de-duplicated); the method is never rewritten. A path
matching nothing yields not-found.

## Reverse links

`Link(name, params, query)` builds a URL from the same catalog:

- Single-segment values must be non-empty strings; multi-segment values
  are (possibly empty) lists of non-empty strings.
- Missing/extra parameters, unknown routes, separators, and `.`/`..`
  values are rejected.
- Each segment is percent-encoded individually; query values are encoded
  independently, sorted by name, with per-name value order preserved.
- The generated path is matched again with the route's own method. If it
  does not resolve back to the same route with the same extracted
  values, the link is refused rather than returning a URL that a
  higher-priority route would intercept.

## CLI

`cmd/routes` reads one JSON request per line on stdin and writes one
JSON response per line on stdout:

```
{"op":"load","catalog":"u GET /u/{user}\n"}
{"op":"check","catalog":"..."}
{"op":"match","method":"GET","path":"/u/bob?x=1"}
{"op":"link","name":"u","params":{"user":"bob"},"query":{"q":["1","2"]}}
```

Responses carry `ok`; failures include an `error` string plus structured
`issues` (validation, with line and route) or `methods` (method not
allowed). A failed `load` never replaces the current catalog.

## Development

```
go test ./...
```
