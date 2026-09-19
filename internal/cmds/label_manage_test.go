package cmds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

// labelManageServer serves the label CRUD endpoints plus the list endpoint
// resolveLabelIDs uses; it records method/path/body per request.
type labelManageServer struct {
	ts     *httptest.Server
	bodies map[string]map[string]any
}

func newLabelManageServer(t *testing.T) *labelManageServer {
	s := &labelManageServer{bodies: map[string]map[string]any{}}
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		switch key {
		case "GET /api/v1/repos/o/r/labels":
			json.NewEncoder(w).Encode([]api.Label{{ID: 1, Name: "bug", Color: "#ff0000"}})
			return
		case "POST /api/v1/repos/o/r/labels":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			s.bodies[key] = body
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(api.Label{ID: 2, Name: "triage", Color: "#00aabb"})
			return
		case "PATCH /api/v1/repos/o/r/labels/1":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			s.bodies[key] = body
			json.NewEncoder(w).Encode(api.Label{ID: 1, Name: "bug", Color: "#00ff00"})
			return
		case "DELETE /api/v1/repos/o/r/labels/1":
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(500)
	}))
	t.Cleanup(s.ts.Close)
	return s
}

func TestLabelCreateCmd(t *testing.T) {
	s := newLabelManageServer(t)
	out := &strings.Builder{}
	ctx := testCtx(s.ts)
	ctx.Stdout = out
	if err := (labelCreateCmd{}).Run([]string{"triage", "--color", "#00aabb"}, ctx); err != nil {
		t.Fatal(err)
	}
	body := s.bodies["POST /api/v1/repos/o/r/labels"]
	if body["name"] != "triage" || body["color"] != "#00aabb" {
		t.Errorf("body = %v", body)
	}
	var lbl api.Label
	if err := json.Unmarshal([]byte(out.String()), &lbl); err != nil {
		t.Fatalf("receipt: %v (%s)", err, out.String())
	}
	if lbl.ID != 2 || lbl.Name != "triage" {
		t.Errorf("receipt = %+v", lbl)
	}
}

func TestLabelCreateRequiresName(t *testing.T) {
	s := newLabelManageServer(t)
	err := (labelCreateCmd{}).Run(nil, testCtx(s.ts))
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitUsage {
		t.Fatalf("err = %v, want usage error", err)
	}
}

func TestLabelEditCmdResolvesByName(t *testing.T) {
	s := newLabelManageServer(t)
	out := &strings.Builder{}
	ctx := testCtx(s.ts)
	ctx.Stdout = out
	if err := (labelEditCmd{}).Run([]string{"bug", "--color", "#00ff00"}, ctx); err != nil {
		t.Fatal(err)
	}
	body := s.bodies["PATCH /api/v1/repos/o/r/labels/1"]
	if body["color"] != "#00ff00" {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["name"]; ok {
		t.Errorf("absent --name was sent: %v", body)
	}
	var lbl api.Label
	if err := json.Unmarshal([]byte(out.String()), &lbl); err != nil {
		t.Fatal(err)
	}
	if lbl.Color != "#00ff00" {
		t.Errorf("receipt = %+v", lbl)
	}
}

func TestLabelEditUnknownName(t *testing.T) {
	s := newLabelManageServer(t)
	err := (labelEditCmd{}).Run([]string{"nope", "--color", "#ffffff"}, testCtx(s.ts))
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitRuntime {
		t.Fatalf("err = %v, want runtime error", err)
	}
	if !strings.Contains(cliErr.Msg, "nope") {
		t.Errorf("msg = %q, want the unknown label named", cliErr.Msg)
	}
}

func TestLabelDeleteCmd(t *testing.T) {
	s := newLabelManageServer(t)
	out := &strings.Builder{}
	ctx := testCtx(s.ts)
	ctx.Stdout = out
	if err := (labelDeleteCmd{}).Run([]string{"bug"}, ctx); err != nil {
		t.Fatal(err)
	}
	var rcpt labelManageReceipt
	if err := json.Unmarshal([]byte(out.String()), &rcpt); err != nil {
		t.Fatal(err)
	}
	if rcpt.Name != "bug" || rcpt.Action != "delete" {
		t.Errorf("receipt = %+v", rcpt)
	}
}

func TestLabelDeleteUnknownNameNamesLabel(t *testing.T) {
	s := newLabelManageServer(t)
	err := (labelDeleteCmd{}).Run([]string{"ghost"}, testCtx(s.ts))
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitRuntime {
		t.Fatalf("err = %v, want runtime error", err)
	}
	if !strings.Contains(cliErr.Msg, "ghost") {
		t.Errorf("msg = %q, want the unknown label named", cliErr.Msg)
	}
}

func TestLabelManageCommandsRegistered(t *testing.T) {
	want := map[string]bool{"label create": false, "label edit": false, "label delete": false}
	for _, c := range LabelCommands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%s not registered in LabelCommands", name)
		}
	}
}
