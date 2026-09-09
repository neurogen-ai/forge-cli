package cmds

import (
	"bytes"

	"testing"

	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/gitctx"
)

// savedirCtx builds a minimal Ctx; no API server is needed because
// resolution only reads args and config.
func savedirCtx(savedirs map[string]string) *cli.Ctx {
	return &cli.Ctx{
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		GlobalFlags: cli.GlobalFlags{Owner: "o", Repo: "r"},
		Cfg:         &config.Config{Savedirs: savedirs},
		Repo:        &gitctx.Repo{Root: "/tmp/fake-root"},
	}
}

func TestResolveSavedirDirFlagWinsOverConfig(t *testing.T) {
	ctx := savedirCtx(map[string]string{"pr-conversation": ".forge/prs"})
	dir, err := resolveSavedirDir([]string{"42", "--dir", "/tmp/elsewhere"}, "pr-conversation", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/tmp/elsewhere" {
		t.Errorf("dir = %q, want the --dir value", dir)
	}
}

func TestResolveSavedirDirFallsBackToConfig(t *testing.T) {
	ctx := savedirCtx(map[string]string{"pr-conversation": ".forge/prs"})
	dir, err := resolveSavedirDir([]string{"42"}, "pr-conversation", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dir != ".forge/prs" {
		t.Errorf("dir = %q, want the configured value", dir)
	}
}

func TestResolveSavedirDirMissingIsUsageError(t *testing.T) {
	ctx := savedirCtx(map[string]string{"issue": ".forge/issues"})
	_, err := resolveSavedirDir([]string{"42"}, "pr-conversation", ctx)
	cliErr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v (%T), want *cli.Error", err, err)
	}
	if cliErr.Code != cli.ExitUsage || cliErr.Msg != "no savedir for pr-conversation" {
		t.Errorf("error = %+v", cliErr)
	}
}

func TestResolveSavedirDirEmptyConfigValueIsMissing(t *testing.T) {
	ctx := savedirCtx(map[string]string{"pr-conversation": ""})
	_, err := resolveSavedirDir(nil, "pr-conversation", ctx)
	if err == nil {
		t.Fatal("empty configured savedir: want error, got nil")
	}
}

func TestResolveConfiguredSavedirMissingIsUsageError(t *testing.T) {
	ctx := savedirCtx(nil)
	_, err := resolveConfiguredSavedir(ctx, "releases")
	cliErr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v (%T), want *cli.Error", err, err)
	}
	if cliErr.Code != cli.ExitUsage || cliErr.Msg != "no savedir for releases" {
		t.Errorf("error = %+v", cliErr)
	}
}
