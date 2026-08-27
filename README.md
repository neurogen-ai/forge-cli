# forge-cli

A command-line client for Forgejo pull requests and issues. Reads repo context
from your git remote, auth from your git credential helper, and writes JSON to
stdout so scripts can parse it.

## Install

```
go install ./cmd/forge
```

Requires Go 1.22+. No third-party dependencies.

## Configuration

Two TOML files are merged per key; the repo-local file wins. `~` is expanded,
and relative savedir paths resolve against the repo root.

`~/.config/forge/config.toml` (or `$XDG_CONFIG_HOME/forge/config.toml`):

```toml
[defaults]
host  = ""   # optional hard override of the remote-derived host
owner = ""
repo  = ""
base  = ""   # default PR base branch

[savedir]
# Defaults live in ~/.local/state/forge ($XDG_STATE_HOME/forge when set).
# Entries here are opt-ins that move them:
# pr-conversation = "~/.local/state/forge/prs"
# issue           = "~/.local/state/forge/issues"

[api]
timeout_seconds = 30 # seconds; 30 when absent everywhere
protocol = "http"    # or "https"; https when absent
```

Repo-local `<repo>/.forge/config.toml`:

```toml
[defaults]
base = "master"

[savedir]
pr-conversation = ".forge/prs"
issue           = ".forge/issues"
```

## Usage

Every value resolves through one chain: flag > environment variable
(`FORGE_HOST`, `FORGE_OWNER`, `FORGE_REPO`, `FORGE_BASE`) > repo-local config >
global config > derived from `git remote get-url origin`. Tokens resolve as:
`--token` > config token > `FORGE_TOKEN` > `git credential fill`.

Global flags on every command: `--host --owner --repo --token --config
--timeout N --verbose/-v`.

`--host` accepts an embedded scheme (`--host http://127.0.0.1:3000`) as a quick
way to talk to a plain-http server, or you can set `[api] protocol`. Requests go
over https unless one of these says otherwise. Plain http to anything other than
localhost prints a one-line `warning: connecting over insecure http://<host>`
to stderr.

### Help

`forge -h` lists the command families; `forge pr -h` lists one family's
subcommands; `forge pr create -h` shows a single command's full page. Any
usage error reprints the misused command's page under its `use:` line.
Help goes to stdout and exits 0; errors keep stderr and their exit code.

### Pull requests

```
forge pr create --title "T" [--head B] [--base B] [--body TEXT]
forge pr get N
forge pr list [--state open|closed|all] [--page N] [--limit M]
forge pr conversation N [--format flat|grouped]
```

`pr create` defaults `--head` to the current branch and `--base` to the
configured base. If no base can be determined it fails asking for `--base`.
Output is JSON on stdout.

A 404 at create time is diagnosed: the command checks whether the base and
head branches exist and reports which one is missing, or says the repo does
not accept pull requests if both exist. The server's original message stays
in the hint.

### Issues

```
forge issue create --title "T" [--body TEXT] [--label name]...
forge issue list [--state ...] [--page N] [--limit M]
forge issue get N
```

`--label` takes label names and resolves them to ids via the labels API.

### Save layer

Saving is explicit; nothing else writes to disk.

```
forge save pr-conversation N [--dir override]
forge save issue N        [--dir override]
```

Writes pretty-printed JSON into the configured savedir and prints the written
path to stdout.

### Cache

```
forge cache path           # print resolved savedirs, one per line
forge cache flush [--yes]  # delete saved files; --yes allows dirs outside the repo
```

`cache flush` is non-recursive and refuses paths outside the repo root unless
`--yes` is given.

### Other

```
forge version
```

## Errors and diagnosis

API commands proceed naively; no wiring checks run before the request.
When a request fails, forge runs its checks (server reachable, token valid,
owner exists, repository exists) and appends what it found below the original
error, after a blank line. If every layer verifies OK the output ends with
"failed to diagnose", meaning the failure belongs to the request itself.
Exit code always comes from the original error.

## Exit codes

See `plans/PRD.md` section 10 for details.

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | runtime / API error |
| 2 | usage error (bad flags, missing required values) |
| 3 | context error (not in a repo, no remote) |
| 4 | auth error (no token, rejected token) |
| 5 | network error (timeout, connection failure) |

Errors go to stderr with an actionable hint. Server error messages are surfaced
verbatim with their HTTP status.
