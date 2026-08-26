package cmds

import (
	"errors"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

// A token rejected by the server (401) must hard-fail with ExitAuth no matter
// whether a token was supplied locally.
func TestMapErr401IsAuthExit(t *testing.T) {
	err := mapErr(&api.APIError{Status: 401, Message: "unauthorized"})
	var cerr *cli.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("mapErr returned %T, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitAuth {
		t.Fatalf("code = %d, want ExitAuth (%d)", cerr.Code, cli.ExitAuth)
	}
	if !errors.Is(err, cerr) || cerr.Msg != "401: unauthorized" {
		t.Fatalf("server message not carried through: %q", cerr.Msg)
	}
	if cerr.Hint == "" {
		t.Fatal("auth rejection must carry a hint naming the fix")
	}
}

func TestMapErr403IsAuthExit(t *testing.T) {
	err := mapErr(&api.APIError{Status: 403, Message: "forbidden"})
	var cerr *cli.Error
	errors.As(err, &cerr)
	if cerr.Code != cli.ExitAuth {
		t.Fatalf("403 code = %d, want ExitAuth", cerr.Code)
	}
}

func TestMapErrOtherStatusesStayRuntime(t *testing.T) {
	for _, status := range []int{400, 404, 422, 500} {
		err := mapErr(&api.APIError{Status: status, Message: "x"})
		var cerr *cli.Error
		errors.As(err, &cerr)
		if cerr.Code != cli.ExitRuntime {
			t.Fatalf("%d mapped to %d, want ExitRuntime", status, cerr.Code)
		}
	}
}
