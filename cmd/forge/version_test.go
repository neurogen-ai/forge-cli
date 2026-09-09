package main

import (
	"bytes"
	"strings"
	"testing"

	"forge/internal/cli"

	"forge"
)

// forge version must read the repo-root constant, so the output tracks any
// release bump without this test changing. Pinning the literal here would
// put the version number in a second place.
func TestVersionCmdPrintsConstant(t *testing.T) {
	out := &bytes.Buffer{}
	ctx := &cli.Ctx{Stdout: out, Stderr: &bytes.Buffer{}}
	if err := (versionCmd{}).Run(nil, ctx); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "forge-cli v") {
		t.Fatalf("output = %q, want prefix \"forge-cli v\"", got)
	}
	if want := "forge-cli v" + forge.Version + "\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if forge.Version == "" {
		t.Fatal("forge.Version is empty")
	}
}
