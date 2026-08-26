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
pr-conversation = "~/forge-cache/prs"
issue           = "~/forge-cache/issues"

[api]
timeout_seconds = 30 # seconds; 30 when absent everywhere
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
