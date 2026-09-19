# forge-cli

forge is a gh-style CLI for Forgejo. The command surface follows gh's
design — `pr view`, `pr comment`, `pr review --approve`, `pr checkout`, `api`
— so a developer who knows gh can use forge without reading the manual, and
existing spellings (`pr get`, `pr comment add`, `pr review submit --state`)
stay as compatibility aliases with unchanged receipts. Reads repo context
from your git remote, auth from your git credential helper, and writes JSON to
stdout so scripts can parse it.

## gh to forge

| gh | forge |
|---|---|
| `gh pr list` | `forge pr list` |
| `gh pr view N` | `forge pr view N` (`pr get` is the compatibility alias) |
| `gh pr diff N` | `forge pr diff N` |
| `gh pr comment N --body T` | `forge pr comment N --body T` (`pr comment add` stays registered) |
| `gh pr review N --approve` / `--request-changes` / `--comment` | `forge pr review N --approve\|--request-changes\|--comment [--body T]` (`pr review submit --state S` stays registered) |
| `gh pr merge N --squash` | `forge pr merge N --squash` |
| `gh pr checkout N` | `forge pr checkout N [--branch B]` |
| `gh api PATH` | `forge api PATH` |

Index placement follows gh too: `forge pr merge 11 --merge` and
`forge pr merge --merge 11` both work on every command that takes a PR
number.

`forge api <path> [--method M] [--input @file\|JSON\|-] [--q k=v]... [--jq Q]`
is the escape hatch when a command is missing: it sends the authenticated
request to the configured host's API and prints the response, JSON
pretty-printed when the response is JSON, byte-exact otherwise. The auth
header goes wherever you send the request, so scope paths accordingly.

### An agent skill for forge

Six commands cover the review lifecycle, and they behave the same from a
fresh clone with only `FORGE_TOKEN` set: `pr list` to find a PR, `pr view`
to read it, `pr diff` for the patch, `pr comment` to post (including
anchored inline comments with `--file`/`--line`/`--side`), `pr review` to
approve or request changes, and `pr merge` to land it. Everything prints
JSON when stdout is not a terminal, errors carry an actionable hint on
stderr, and no step prompts.

```sh
forge pr list --state open
forge pr view 7
forge pr diff 7
forge pr comment 7 --body "checked, one nit below"
forge pr comment 7 --file main.go --line 42 --side new --body "this leaks the conn"
forge pr review 7 --request-changes --body "nits on line 42"
forge pr merge 7 --squash
```

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
# releases        = ".forge/cache/releases"

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
list`, `issue list`, `pr review list`, `label list`, `release list`) show
human-readable tables when stdout
is an interactive terminal, and emit JSON otherwise.

Two global flags override the default for any command:

- `--json` forces JSON output regardless of TTY state.
- `--table` / `-t` forces table rendering where the command supports it;
  it fails with a usage error (exit 2) on commands that only have one output
  form.

Passing both `--json` and `-t` together is a conflict and exits 2 with a usage
error.

Receipts — the JSON objects printed by mutating commands like `pr comment
resolve` and `pr pull` — are always JSON, whatever flags are passed. The
full contract: JSON goes to stdout, diagnostics and errors go to stderr,
and receipts are always JSON on stdout even when the command also prints a
table in a terminal.

### Help

`forge -h` lists the command families; `forge pr -h` lists one family's
subcommands; `forge pr create -h` shows a single command's full page. Any
usage error reprints the misused command's page under its `use:` line.
Help goes to stdout and exits 0; errors keep stderr and their exit code.

### Pull requests

```
forge pr create --title "T" [--head B] [--base B] [--body TEXT]
forge pr view N
forge pr list [--state open|closed|all] [--page N] [--limit M]
forge pr conv N [--all] [--min-unresolved N]
forge pr create-batch PATTERN [--base B] [--body TEXT] [--yes]
forge pr edit N [--title T] [--body B]
forge pr browse N [--open]
forge pr review submit N --state approve|request-changes|comment [--body T]
forge pr review N --approve|--request-changes|--comment [--body T]
forge pr comment add N --body T
forge pr comment N --body T [--file P --line L [--side old|new]]
forge pr checkout N [--branch B]
forge pr close N
forge pr reopen N
forge pr ready N
forge pr diff N [--patch] [--out]
forge pr merge N --merge|--squash|--rebase [--subject S] [--body T] [--delete]
```

`pr view` is the canonical spelling of the single-PR read; `pr get` is the
compatibility alias and prints the same JSON. `pr comment` and `pr review`
with flags are the gh-spelled forms; the older `comment add` and
`review submit --state` spellings stay and print the same receipts.

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

The plan is ordered oldest tip first by committer date, so stack parents
post before the branches that contain them; equal dates fall through to
ancestry (stack parent first), then lexical. Branches that cannot be
planned are skipped with a note on stderr:
`skipped: <branch> (no commit subject)` for a tip without a commit
subject, `skipped: <branch> (already in base)` for a tip already
contained in the resolved base. If nothing remains after the skips, the
command exits 2. The ordering and the containment preflight answer the
empty-parent-PR failure v0.3.1 shipped with; see
`plans/releases/v0.4.1.md` for the root cause on record.

With `--yes`, after each POST the client waits for the PR page to become
reachable before posting the next branch — bounded retries, about 5
attempts 500ms apart, unauthenticated — and a timeout only prints
`note: <branch> page not confirmed available before continuing`; the
batch continues. A duplicate-PR server response is also a skip, not a
failure: `skipped: <branch> (PR already exists)`, and the remaining
branches still post. The matcher on the server's message is provisional
until confirmed against the live Forgejo instance.

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

### Edit

`pr edit N [--title T] [--body B]` and `issue edit N [--title T] [--body B]`
patch title and body and print the updated object as JSON. At least one flag
must be non-empty; that check runs before any request. Empty or absent flags
leave the field untouched, so an edit never clears a value.

### Diff

`pr diff N` prints the server's raw `.diff` bytes on stdout exactly as
received — no trailing newline, no JSON wrapper. `--patch` selects the
`.patch` representation instead. `--out` writes the same bytes below the
`pr-conversation` savedir as `<repo>-N.diff` (or `<repo>-N.patch` with
`--patch`), replacing any previous copy for that PR and format, and prints a
receipt instead of the diff:

```json
{"path": "/abs/path/to/repo/.forge/cache/prs/r-42.diff", "bytes": 1873}
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

Future work, not shipped: `forge search`
(v0.6.0) and an auto-sync service (v0.8.0) — see `plans/releases/`.
`forge api PATH [--method M] [--input @file|JSON|-] [--q k=v]... [--jq Q]`
is shipped: it sends the path under the configured host's `/api/v1` base and
prints the response, JSON pretty-printed, anything else byte-exact.

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
forge issue edit N [--title T] [--body B]
forge issue label add N --label name...
forge issue label remove N --label name...
forge issue browse N [--open]
```

`--label` takes label names and resolves them to ids via the labels API.
`issue close` and `issue open` patch the issue state server-side and print
the updated issue JSON.

`label list` prints a NAME/COLOR/ID table on a TTY and JSON otherwise.
`issue label add` resolves names to ids with one labels-list request
(exact match, input order and duplicates preserved) and sends a single POST
carrying every id. `issue label remove` deletes one id at a time in the given
order and stops on the first failure. Both print a receipt naming the labels
you asked for, only after the mutation succeeded.

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

### Browse

```
forge pr browse N [--open]
forge issue browse N [--open]
forge repo browse [--open]
```

These print the web URL the server returned (`html_url`). With `--open` the
URL is passed as the final argument to `$BROWSER` (split on whitespace, no
shell), and the URL still prints when opening. Nothing opens without
`--open`.

### Releases

```
forge release list
forge release get TAG
forge release download TAG [--asset NAME]...
```

`release list` prints a TAG/NAME/DRAFT/PRERELEASE/PUBLISHED table on a TTY
and JSON otherwise, in server order. `release get TAG` prints the release
JSON. `release download TAG` writes each asset into the `.forge/cache/releases`
savedir under its exact name, overwriting any existing file, and prints one
receipt only after every selected asset succeeded:

```json
{"tag": "v1.2.0", "files": [{"name": "forge-linux", "path": ".forge/cache/releases/forge-linux", "bytes": 9342976}]}
```

A mid-batch failure leaves earlier files in place and prints no receipt.
Asset URLs on a different host than the API are rejected before the token is
sent. Scope decisions for this release live in `plans/releases/v0.4.2.md`.

### Other

```
forge version
```

`forge version` prints `forge-cli v0.4.2` from the `forge.Version` constant
in repo-root `version.go`. That one line is where version numbers change.

## Three ways to ask "what's going on"

These three answer different questions, at different ranges:

- `forge pr sync-status [BRANCH]` answers one question about one branch: does
  the open pull request for this branch carry my local head SHA? One request,
  one answer (`in-sync`, `out-of-sync`, or `no-pr`), exit 0 either way. CI
  polling is the caller's job.
- `forge status` answers "what in this repository needs me right now?": review
  requests made of you, pull requests assigned to you, open issues. One repo
  per invocation; it never enumerates repositories.
- Setup triage is the error path itself: when a command fails, forge probes
  the host, token, owner, and repository layers and names the one that is
  wrong (see Errors and diagnosis below). There is no separate diagnose
  command; run any command and read the appended diagnosis.

## Errors and diagnosis

API commands proceed naively; no wiring checks run before the request.
When a request fails, forge runs its checks (server reachable, token valid,
owner exists, repository exists) and appends what it found below the original
error, after a blank line. If every layer verifies OK the output ends with
"failed to diagnose", meaning the failure belongs to the request itself.
Exit code always comes from the original error.

## Exit codes

Matched line for line to the constants in `internal/cli/exit.go`
(`ExitOK`, `ExitRuntime`, `ExitUsage`, `ExitContext`, `ExitAuth`,
`ExitNetwork`):

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | API/runtime error |
| 2 | bad flags / missing value |
| 3 | not in repo / no remote |
| 4 | no token / rejected |
| 5 | timeout / conn failure |

Errors go to stderr with an actionable hint. Server error messages are surfaced
verbatim with their HTTP status.
