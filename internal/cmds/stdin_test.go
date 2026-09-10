package cmds

import (
	"strings"
	"testing"

	"forge/internal/cli"
)

func TestReadDashInputDashReadsStdin(t *testing.T) {
	got, err := readDashInput("-", strings.NewReader("piped body"))
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if string(got) != "piped body" {
		t.Fatalf("got %q; want %q", got, "piped body")
	}
}

func TestReadDashInputNonDashPassesThrough(t *testing.T) {
	got, err := readDashInput("hello world", nil)
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("got %q; want %q", got, "hello world")
	}
}

func TestReadDashInputDashWithNilStdinIsUsageError(t *testing.T) {
	_, err := readDashInput("-", nil)
	if err == nil {
		t.Fatal("err = nil; want usage error")
	}
	e, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %T; want *cli.Error", err)
	}
	if e.Code != cli.ExitUsage {
		t.Fatalf("code = %d; want %d", e.Code, cli.ExitUsage)
	}
}
