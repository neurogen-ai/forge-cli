# forge-cli — PRD (v0.1)

A small, observable Go CLI that wraps the Forgejo HTTP API (`/api/v1`). Think "lite `gh`": one binary that creates pull requests, reads PR conversations, and manages issues — designed so both humans and agents can drive it.

## 1. Goals

1. **Replace the bash workflow** in `~/.bashrc` (`forge-create-pr`, `forge-fetch-prc`) with a single binary that is more observable and **fails loudly** (structured errors, non-zero exits, request logging).
2. **Zero-config auth**: reuse Git credential manager via `git credential fill` (protocol + host) — same mechanism the bash workflow uses today. Token = `password=` line; sent as `Authorization: token <TOKEN>`.
3. **Repo-aware defaults**: run inside a git repo and host/owner/repo are inferred from `git remote get-url origin`. Every inferred default is overridable by flag, env, or config.
4. **Fetch ≠ Save**: API-facing commands return data to stdout as JSON. Writing to disk is always an explicit separate step (`--save` / `save` command). This keeps fetch functions pure feeders for agents/scripts.
5. **Modular & extensible**: a thin core (client, auth, config, git context) plus a command registry so future utility interactions (labels, releases, comments, review approve) bolt on without touching the core.

## 2. Non-goals (v0.1)

- No merge/rebase/close of PRs beyond what issues endpoints require.
- No release management, attachment upload/download, wiki, webhooks, actions.
- No interactive TUI; no daemon/caching server (only simple file saving).
- No GitHub support; Forgejo/Gitea-compatible API only.

## 3. Personas

- **Human developer**: `forge pr create -t "fix: x"`, pipes output to `jq`, wants clear errors with the server's message surfaced verbatim.
- **Agent/script**: calls `forge pr conversation 42 --json` (default), parses stdout, never triggers side effects implicitly; uses explicit exit codes to branch.

## 4. Command surface (sketch)

```
forge pr create --title T [--head B] [--base B] [--body TEXT]      # POST /repos/{o}/{r}/pulls
forge pr get N                                                     # GET /pulls/{index}
forge pr list [--state open|closed|all] [--page N] [--limit M]     # GET /pulls
forge pr conversation N                                            # unified view:
                                                                   #   issue comments + reviews + review comments
                                                                   #   (--format flat|grouped)
forge issue create --title T [--body TEXT] [--label l]... [--assignee u]...
forge issue list [--state ...] [--page N] [--limit M]
forge issue get N

# save layer (explicit, never implicit):
forge save pr-conversation N [--dir override]                      # pulls then writes JSON file
forge save issue N [--dir override]                                # pulls then writes JSON file
forge cache flush                                                  # empties configured download dirs
forge cache path                                                   # prints resolved dirs

Global flags: --host, --owner, --repo, --token, --config, --verbose (-v), --timeout (seconds)

`pr conversation` output shapes: `--format flat` = one JSON array of all items
(comments, reviews, review comments) sorted by `created_at`, each tagged with a
`kind` field; `--format grouped` (default) =
`{"comments": [...], "reviews": [{"<review fields>", "comments": [...]}]}`
mirroring the proven bash workflow's flattened view.
```

All output is JSON on stdout by default (`--format table` available later). Errors go to stderr as `{"error": "...", "hint": "...", "status": N}`.

## 5. Auth resolution order (first match wins)

1. `--token` flag
2. `FORGE_TOKEN` env
3. token in repo-local config (discouraged but allowed for CI dirs)
4. token in global config
5. `git credential fill` with `protocol=https\nhost=<resolved-host>` → `password=`

If credential fill fails or returns empty: exit code 4 with a loud, actionable error ("no token found: check your git credential helper stores a token for this host, or pass --token"). Never fall back silently to unauthenticated requests.

## 6. Repo-aware defaults

- Host/owner/repo parsed from `git remote get-url origin`; supports `https://host/o/r(.git)`, `git@host:o/r.git`, `ssh://git@host/o/r`.
- Outside a repo (or no remote): required flags must be supplied or we fail loudly with which values are missing.
- Precedence for any contextual value: **flag > env > repo-local config > global config > derived-from-git-remote**.

## 7. Config schema

Two layers, deep-ish merged (repo-local wins):

```toml
# global: ~/.config/forge/config.toml
# repo-local: ./.forge/config.toml (commit-friendly; secrets do NOT belong here)
[defaults]
host    = ""   # optional hard override of remote-derived host
owner   = ""
repo    = ""
base    = ""   # default PR base branch; if unset, falls back to server-reported
               # default branch (GET /repos/{o}/{r}), else --base required

[savedir]                    # per-item-type download directories
pr-conversation = ".forge/prs"
issue           = ".forge/issues"

[api]
timeout_seconds = 30
```

- `forge cache flush` deletes the *contents* of every configured savedir, printing each removed path. If any resolved path lies outside the current repo root, it aborts without `--yes`.
- Saving layout: `<savedir>/<repo>-<N>.json` (savedirs are already per-item-type; `-<ts>` suffix added when overwriting would clobber).

## 8. Fetch vs save contract

- `internal/api` exposes typed methods returning decoded structs / raw JSON — **no filesystem access, no stdout writes**.
- Commands render to stdout only. The `save` family calls the same api functions then serializes to disk.
- Agents consume the CLI over stdout JSON (stable, versioned shape), or shell out — parse ambiguity-free. Note `internal/api` cannot be imported outside this module (Go `internal/` rule); if in-process embedding is ever needed, promote that package out of `internal/` then.

## 9. Extensibility model

```go
type Command interface {
    Name() string                       // "pr conversation"
    Run(ctx *Ctx, args []string) error  // Ctx carries client, cfg, flags
}
// registry.Register(cmd) in cmd/forge/main.go; new features = new file + Register call.
```

Core packages stay dumb; all Forgejo knowledge lives in feature files.

## 10. Observability & error contract

- Exit codes: `0` ok · `1` runtime/API error · `2` usage/flag error · `3` not-in-repo/context error · `4` auth error · `5` network/timeout.
- `--verbose`: logs method, URL (redacted headers), status, latency to stderr.
- Server JSON errors (`{"message":"..."}`) surfaced verbatim plus HTTP status.
- Loud failures: no swallowed errors anywhere; every non-zero path explains itself.

## 11. Success criteria

- Bash workflows reproduced: create PR against `Neurogenesis/ngen-weave` at `git.ngenesis.co.uk`; fetch full conversation incl. per-review comments — using zero stored credentials beyond git credential manager.
- Fresh clone → `forge pr list` works with no setup.
- `forge cache flush` leaves working tree clean except intentionally deleted cache dirs.
