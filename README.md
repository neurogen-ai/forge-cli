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
and relative save paths resolve against the repo root.

`~/.config/forge/config.toml` (or `$XDG_CONFIG_HOME/forge/config.toml`):

```toml
[defaults]
host  = ""   # optional hard override of the remote-derived host
owner = ""
repo  = ""
base  = ""   # default PR base branch

[savedir]
# Defaults live under .forge/cache (see Pull below).
# Entries here are opt-ins that move them:
# pr-conversation = ".forge/cache/prs"
# issue           = ".forge/cache/issues"

[api]
timeout_seconds = 30 # seconds; 30 when absent everywhere
protocol = "http"    # or "https"; https when absent
```

Repo-local `<repo>/.forge/config.toml`:

```toml
[defaults]
base = "master"

[savedir]
pr-conversation = ".forge/cache/prs"
issue           = ".forge/cache/issues"
```

## Usage

Every value resolves through one chain: flag > environment variable
(`FORGE_HOST`, `FORGE_OWNER`, `FORGE_REPO`, `FORGE_BASE`) > repo-local config >
global config > derived from `git remote get-url origin`. Tokens resolve as:
`--token` > config token > `FORGE_TOKEN` > `git credential fill`.

Global flags on every command: `--host --owner --repo --token --config
--timeout N --verbose/-v --json --table/-t`.

`--host` accepts an embedded scheme (`--host http://127.0.0.1:3000`) as a quick
way to talk to a plain-http server, or you can set `[api] protocol`. Requests go
over https unless one of these says otherwise. Plain http to anything other than
localhost prints a one-line `warning: connecting over insecure http://<host>`
to stderr.

### Output contract

Rendering defaults are TTY-aware: listing commands that render tables (`pr
list`, `issue list`, `pr review list`) show human-readable tables when stdout
is an interactive terminal, and emit JSON otherwise.

Two global flags override the default for any command:

- `--json` forces JSON output regardless of TTY state.
- `--table` / `-t` forces table rendering where the command supports it;
  it fails with a usage error (exit 2) on commands that only have one output
  form.

Passing both `--json` and `-t` together is a conflict and exits 2 with a usage
error.

Receipts — the JSON objects printed by mutating commands like `pr comment
resolve` and `pr pull` — are always JSON, whatever flags are passed.

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
forge pr conv N [--all] [--min-unresolved N]
forge pr create-batch PATTERN [--base B] [--body TEXT] [--yes]
forge pr review submit N --state approve|request-changes|comment [--body T]
forge pr comment add N --body T
forge pr close N
forge pr reopen N
forge pr ready N
forge pr diff N [--patch] [--out]
forge pr merge N --merge|--squash|--rebase [--delete] [--subject S] [--body T]
```

`pr create` defaults `--head` to the current branch and `--base` to the
configured base. If no base can be determined it fails asking for `--base`.
With no `--title`, `pr create` infers it from the branch tip's commit
subject, which needs at least one commit unique to the head branch vs the
base. With no `--base` the base comes from `$FORGE_BASE`, then the
`[defaults].base` config, then `origin/HEAD`, then the server's default
branch. There is no hardcoded `main`. Output is JSON on stdout.

`forge pr` with no args, or `-h`, opens the `pr conv` page plus an index of
the other pr verbs.

`pr create-batch PATTERN` opens PRs for local branches matching the glob.
It is dry-run by default and prints a JSON plan; `--yes` posts. On the first
failed POST it stops and prints the partial receipt.

```
forge pr create-batch 'v0.4.0*'        # plan, posts nothing
forge pr create-batch 'v0.4.0*' --yes  # posts, stops on first failure
```

A 404 at create time is diagnosed: the command checks whether the base and
head branches exist and reports which one is missing, or says the repo does
not accept pull requests if both exist. The server's original message stays
in the hint.

`pr conv N` renders the review conversation of PR N with unresolved threads
first and unresolved comment counts per review. With `--all` it includes
resolved threads too; `--min-unresolved N` filters reviews to those with at
least N unresolved comments. Pass `-t` / `--table` for sectioned,
human-readable rendering (unresolved section first); without it the output is
JSON.

Note: the old `pr conversation` spelling was removed in v0.3.0 — use
`pr conv`.

### Review and comment writes

`pr review submit N --state S` posts exactly one review. `--state` accepts
`approve`, `request-changes`, or `comment`; `request-changes` requires
`--body` (missing body exits 2 before any request is sent), the other two
take an optional `--body`. The receipt is the created review's id and state:

```json
{"id": 41, "state": "APPROVED"}
```

`pr comment add N --body T` and `issue comment add N --body T` post one
comment each; Forgejo backs both with the same issue-comment endpoint, so
both spellings print the same receipt shape. A missing `--body` is a usage
error and sends no request:

```json
{"id": 123456, "html_url": "https://host/o/r/issues/6#issuecomment-123456"}
```

### PR state verbs

`pr close N`, `pr reopen N`, and `pr ready N` each send one PATCH
(`{"state":"closed"}`, `{"state":"open"}`, `{"draft":false}`) and print
the updated pull request JSON. No prompts, no confirmation, no retry. A
server that does not support draft changes surfaces its message through the
normal error path.

### Diff

`pr diff N` prints the server's raw `.diff` bytes on stdout exactly as
received — no trailing newline, no JSON wrapper. `--patch` selects the
`.patch` representation instead. `--out` writes the same bytes below the
`pr-conversation` savedir as `<repo>-N.diff` (or `<repo>-N.patch` with
`--patch`), replacing any previous copy for that PR and format, and prints a
receipt instead of the diff:

```json
{"path": ".forge/cache/prs/o-r-42.diff", "bytes": 1873}
```

Nothing is cached without `--out`; stdout requests write no files.

### Merge

`pr merge N` requires exactly one strategy flag: `--merge`, `--squash`, or
`--rebase`. Missing or multiple strategy flags exit 2 before any request.
`--subject` and `--body` map to the merge title and message. Conflicts, WIP
branches, and protection failures come back from the server verbatim with
exit 1; forge never retries, force-merges, or updates branches implicitly.

`--delete` first fetches the PR to capture its head ref, merges, and only
then deletes the head branch ref. A failed merge never triggers a delete; a
failed delete after a successful merge still prints the receipt with
`head_deleted: false` alongside the error, so the successful merge is not
hidden:

```json
{"index": 42, "action": "merge", "head_deleted": false}
```

Future work, not shipped: the `forge api` passthrough and `forge search`
(v0.5.0) and an auto-sync service (v0.6.0) — see `plans/releases/`.

### Resolution

Resolve or unresolve individual review comments by their **root** comment id:

```
forge pr comment resolve <rootCommentId>
forge pr comment unresolve <rootCommentId>
```

Each prints a receipt like:

```json
{"id": 123456, "action": "resolve"}
```

Only root comment ids may be targeted; passing a reply id fails with an error.
When the connected server does not offer a resolution endpoint, the command
fails loudly saying so rather than pretending success.

Resolve every unresolved thread in a PR:

```
forge pr resolve-all N          # dry-run: prints sorted array of candidate root ids
forge pr resolve-all N --yes    # resolves them, then prints summary
```

The dry-run prints e.g. `[421, 430, 512]`; nothing is sent. With `--yes` the
receipt reports what happened:

```json
{"requested": 3, "resolved": 2, "skipped": 0, "failed": 1}
```

Already-resolved threads are skipped, so rerunning resolve-all after a partial
failure is safe.

### Issues

```
forge issue create --title "T" [--body TEXT] [--label name]...
forge issue list [--state ...] [--page N] [--limit M]
forge issue get N
forge issue close N
forge issue open N
forge issue comment add N --body T
```

`--label` takes label names and resolves them to ids via the labels API.
`issue close` and `issue open` patch the issue state server-side and print
the updated issue JSON.

### Pull

Explicit snapshots; nothing else writes to disk.

```
forge pr pull N
forge issue pull N
```

Writes pretty-printed JSON dumps into `.forge/cache/<key>` one item per file,
overwriting any existing dump so repeated pulls stay fresh:
pull-request conversations land in `.forge/cache/prs/<repo>-N.json`, issues in
`.forge/cache/issues/<repo>-N.json`. Each pull prints a receipt naming what was
written:

```json
{"path": ".forge/cache/prs/ngenesis/ngen-weave-42.json", "items": 12, "reviews": 3, "unresolved": 5}
```

Cache dumps are throwaway local snapshots — add `.forge/cache/` to your
repository's `.gitignore` (`.forge/config.toml` itself stays committable).

The same config keys control location as before (`[savedir]
pr-conversation` / `issue`); they now seed defaults rooted at `.forge/cache`.

### Cache

```
forge cache path           # print resolved savedirs, one per line
forge cache flush [--yes]  # delete saved files; --yes allows dirs outside the repo
```

`cache flush` is non-recursive and refuses paths outside the repo root unless
`--yes` is given. Even under `--yes`, the flush guard refuses to touch anything
that contains or lives above a `config.toml`: files named `config.toml` and
directories that are parents of a config file are always protected.

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
| 2 | usage error (bad flags, missing required values, flag conflicts such as `--json` with `-t`) |
| 3 | context error (not in a repo, no remote) |
| 4 | auth error (no token, rejected token) |
| 5 | network error (timeout, connection failure) |

Errors go to stderr with an actionable hint. Server error messages are surfaced
verbatim with their HTTP status.
