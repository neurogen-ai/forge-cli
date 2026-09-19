package cmds

import (
	"bytes"
	"encoding/json"
	"forge/internal/cli"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// apiCmd is the authenticated raw-API escape hatch: one path joined below the
// client's /api/v1 base, one request through DoRaw, and the response body
// rendered as received. It exists so CLI omissions never block a script.
type apiCmd struct{}

func (apiCmd) Name() string      { return "api" }
func (apiCmd) Summary() string   { return "call the Forgejo API directly and print the raw response" }
func (apiCmd) RequiresAPI() bool { return true }

func (apiCmd) HelpPage() string {
	return `use: forge api <path> [--method M] [--input @file|JSON|-] [--q k=v]... [--jq Q]

Send one authenticated request to <host>/api/v1<path> and print the response
body. The path is relative to the API root; a leading slash is optional.
Absolute http(s) URLs are rejected: the request always goes to the configured
host, so the token is never sent anywhere you did not point forge at.

--method defaults to GET and must be a valid HTTP method token. --q is
repeatable; repeated keys are sent as repeated query parameters. --input
takes inline JSON, @file, or - for stdin. With --jq the JSON response is
piped through the system jq with your filter as a single argument; the shell
is never involved, and jq's own errors are passed through.

The request carries your forge token. Only send paths you mean to send it to.
`
}

// APICommand returns the top-level forge api passthrough command.
func APICommand() cli.Command { return apiCmd{} }

func (c apiCmd) Run(args []string, ctx *cli.Ctx) error {
	method, args, err := c.stringFlag(args, "--method")
	if err != nil {
		return err
	}
	if method == "" {
		method = "GET"
	}
	input, args, err := c.stringFlag(args, "--input")
	if err != nil {
		return err
	}
	queries := flagValues(args, "--q")
	jqFilter, args, err := c.stringFlag(args, "--jq")
	if err != nil {
		return err
	}
	rest := stripFlags(args, "--method", "--input", "--q", "--jq")
	if len(rest) != 1 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "api requires exactly one path argument",
			Hint: "use: forge api <path> [--method M] [--input @file|JSON|-] [--q k=v]...",
		}
	}
	path, err := apiPath(rest[0])
	if err != nil {
		return err
	}

	var body []byte
	if input != "" {
		if body, err = apiInput(input, ctx.Stdin); err != nil {
			return err
		}
	}

	query := url.Values{}
	for _, kv := range queries {
		eq := strings.Index(kv, "=")
		if eq <= 0 {
			return &cli.Error{
				Code: cli.ExitUsage,
				Msg:  "--q expects k=v, got " + kv,
				Hint: "quote the whole pair if the shell splits it: --q \"k=v\"",
			}
		}
		query.Add(kv[:eq], kv[eq+1:])
	}

	method = strings.ToUpper(method)
	if !httpMethodToken(method) {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "--method: \"" + method + "\" is not a valid HTTP method",
			Hint: "use an HTTP method token such as GET, POST, PUT, PATCH, or DELETE",
		}
	}

	resp, err := ctx.API.DoRaw(method, path, query, body)
	if err != nil {
		return mapErr(err)
	}
	if jqFilter != "" {
		return runJQ(ctx.Stdout, ctx.Stderr, jqFilter, resp.Body)
	}
	return writeAPIResponse(ctx.Stdout, resp.ContentType, resp.Body)
}

// httpMethodToken reports whether s is a valid HTTP method token per RFC 7230
// (token characters only); it keeps an invalid --method a usage error instead
// of surfacing later as a request-build runtime error.
func httpMethodToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}

// stringFlag reads the single occurrence of a value flag, rejecting repeats.
// The returned args have the flag and its value removed.
func (apiCmd) stringFlag(args []string, name string) (string, []string, error) {
	values := flagValues(args, name)
	switch len(values) {
	case 0:
		return "", stripFlags(args, name), nil
	case 1:
		return values[0], stripFlags(args, name), nil
	default:
		return "", args, &cli.Error{Code: cli.ExitUsage, Msg: name + " given more than once"}
	}
}

// apiPath validates the request path: exactly one relative path below the
// client's /api/v1 base, leading slash optional, never an absolute URL.
func apiPath(path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return "", &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "api takes a path below /api/v1, not an absolute URL",
			Hint: "requests always go to the configured host; use --q for query parameters",
		}
	}
	if path == "" || path == "/" {
		return "", &cli.Error{Code: cli.ExitUsage, Msg: "api requires a non-empty path"}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path, nil
}

// apiInput resolves --input: "@" reads a file, "-" reads stdin through
// readDashInput, anything else is the literal body.
func apiInput(value string, stdin io.Reader) ([]byte, error) {
	if value == "-" {
		return readDashInput(value, stdin)
	}
	if strings.HasPrefix(value, "@") {
		b, err := os.ReadFile(value[1:])
		if err != nil {
			return nil, &cli.Error{Code: cli.ExitUsage, Msg: "--input: " + err.Error()}
		}
		return b, nil
	}
	return []byte(value), nil
}

// flagValues returns the values of every "--name value" pair in args,
// preserving order. Repeatable flags read through this helper.
func flagValues(args []string, name string) []string {
	var out []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			out = append(out, args[i+1])
		}
	}
	return out
}

// runJQ pipes body through the system jq with filter as one argv element;
// there is no shell anywhere in this path. A missing jq binary is a usage
// error with an install hint. A non-zero jq exit is a runtime error whose
// hint preserves jq's stderr; jq's stdout is not written on failure.
func runJQ(out, errOut io.Writer, filter string, body []byte) error {
	jq, err := exec.LookPath("jq")
	if err != nil {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "--jq requires the jq binary, which was not found on PATH",
			Hint: "install jq (https://jqlang.github.io/jq/download/), or drop --jq to print the raw response",
		}
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(jq, filter) // filter is one argv element; no shell
	cmd.Stdin = bytes.NewReader(body)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_, _ = errOut.Write(stderr.Bytes())
		if _, ok := err.(*exec.ExitError); ok {
			return &cli.Error{
				Code: cli.ExitRuntime,
				Msg:  "--jq: filter failed",
				Hint: strings.TrimRight(stderr.String(), "\n"),
			}
		}
		return &cli.Error{Code: cli.ExitRuntime, Msg: "--jq: " + err.Error()}
	}
	_, err = out.Write(stdout.Bytes())
	return err
}

// isJSONMediaType reports whether a Content-Type header names JSON, tolerating
// parameters such as charset and the structured-syntax +json suffix.
func isJSONMediaType(contentType string) bool {
	base := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return base == "application/json" || (base != "" && strings.HasSuffix(base, "+json"))
}

// writeAPIResponse renders a passthrough body: JSON content types are decoded
// and pretty-printed through writeJSON, everything else passes through
// byte-exact. An empty body writes nothing. A JSON content type whose body
// fails to decode also passes through byte-exact rather than erroring.
func writeAPIResponse(w io.Writer, contentType string, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	if isJSONMediaType(contentType) {
		var v any
		if err := json.Unmarshal(body, &v); err == nil {
			return writeJSON(w, v)
		}
	}
	_, err := w.Write(body)
	return err
}
