#!/usr/bin/env sh
# Probes a live Forgejo instance for the version-dependent endpoints that
# v0.6.0's probe-gated transports commit to (probe-first rule from
# plans/releases/v0.6.0.md; see plans/implementation/v0.6.0.md branches A,
# E, G, H). Env required:
#   FORGE_PROBE_HOST   e.g. git.ngenesis.co.uk
#   FORGE_PROBE_TOKEN  token with repo write access
#   FORGE_PROBE_REPO   owner/name of a test repo
# Env optional:
#   FORGE_PROBE_REF    ref for statuses/Actions (default: main)
#   FORGE_PROBE_PR     a PR number used for the reviewer-request probe; the
#                      probe is skipped when unset
# Probes: Actions runs listing for a ref, issue lock POST/DELETE (against a
# scratch issue, deleted afterward), notifications list and mark-read (the
# mark-read probe uses a nonexistent id so nothing visible is written), and
# PR reviewer request/removal. Prints which encodings the server accepted as
# labeled lines. A server rejection is a finding, not an error; a transport
# failure sets TRANSPORT_ERR. Exits non-zero only on transport-level errors.
set -u

HOST="${FORGE_PROBE_HOST:?set FORGE_PROBE_HOST}"
TOK="${FORGE_PROBE_TOKEN:?set FORGE_PROBE_TOKEN}"
REPO="${FORGE_PROBE_REPO:?set FORGE_PROBE_REPO}"
REF="${FORGE_PROBE_REF:-main}"
API="https://$HOST/api/v1/repos/$REPO"
ROOT="https://$HOST/api/v1"
H="Authorization: token $TOK"
CT="Content-Type: application/json"
TRANSPORT_ERR=0

# http METHOD URL [JSON]
http() {
	method="$1"; url="$2"; payload="${3:-}"
	if [ -n "$payload" ]; then
		curl -sS -X "$method" -H "$H" -H "$CT" -d "$payload" -w '\n%{http_code}' "$url" 2>&1
	else
		curl -sS -X "$method" -H "$H" -w '\n%{http_code}' "$url" 2>&1
	fi
}

finding() {
	label="$1"
	rest="$2"
	printf '%s: %s\n' "$label" "$rest"
}

# probe LABEL METHOD URL [JSON]
probe() {
	label="$1"; method="$2"; url="$3"; payload="${4:-}"
	resp=$(http "$method" "$url" "$payload"); curl_rc=$?
	if [ "$curl_rc" -ne 0 ]; then
		finding "$label" "TRANSPORT-ERROR $resp"
		TRANSPORT_ERR=1
		return 0
	fi
	status=$(printf '%s' "$resp" | tail -n 1)
	body=$(printf '%s' "$resp" | sed '$d')
	case "$status" in
		2*|302) verdict=accepted ;;
		*) verdict=rejected ;;
	esac
	excerpt=$(printf '%s' "$body" | head -c 200 | tr -d '\n')
	finding "$label" "$status $verdict $excerpt"
	LAST_STATUS="$status"
	LAST_BODY="$body"
	return 0
}

echo "== v0.6.0 endpoint probe against $HOST $REPO =="
echo "-- commit statuses for ref $REF (baseline, not probe-gated) --"
probe "statuses GET" GET "$API/commits/$REF/statuses"
echo "-- Actions runs listing for a ref --"
probe "actions runs GET" GET "$ROOT/repos/$REPO/actions/runs"
probe "actions tasks GET" GET "$ROOT/repos/$REPO/actions/tasks"
probe "actions runs-ref GET" GET "$API/actions/runs?ref=$REF"

echo "-- issue lock (scratch issue, deleted afterward) --"
SCRATCH_ISSUE=""
resp=$(http POST "$API/issues" '{"title":"probe-v0.6.0 scratch issue (safe to delete)"}')
status=$(printf '%s' "$resp" | tail -n 1)
body=$(printf '%s' "$resp" | sed '$d')
case "$status" in
	2*) SCRATCH_ISSUE=$(printf '%s' "$body" | grep -o '"number":[0-9]*' | head -n1 | cut -d: -f2) ;;
esac
if [ -n "${SCRATCH_ISSUE:-}" ]; then
	finding "scratch-issue" "created #$SCRATCH_ISSUE"
	probe "lock POST" POST "$API/issues/$SCRATCH_ISSUE/lock" '{"reason":"off-topic"}'
	probe "unlock DELETE" DELETE "$API/issues/$SCRATCH_ISSUE/lock"
	resp=$(http DELETE "$API/issues/$SCRATCH_ISSUE")
	case "$(printf '%s' "$resp" | tail -n 1)" in
		2*) finding "scratch-issue cleanup" "deleted" ;;
		*) finding "scratch-issue cleanup" "DELETE FAILED (delete it manually)" ;;
	esac
else
	finding "scratch-issue" "could not create ($status); lock probes skipped"
	TRANSPORT_ERR=1
fi

echo "-- notifications (mark-read probe uses a nonexistent id; nothing visible is written) --"
probe "notifications GET" GET "$ROOT/notifications"
probe "notifications all GET" GET "$ROOT/notifications?all=true"
probe "notifications mark-one PUT" PUT "$ROOT/notifications/999999999"
probe "notifications mark-all PUT" PUT "$ROOT/notifications" '{}'
probe "notifications mark-all POST" POST "$ROOT/notifications"

if [ -n "${FORGE_PROBE_PR:-}" ]; then
	echo "-- PR reviewer request/removal on PR $FORGE_PROBE_PR --"
	probe "reviewers POST" POST "$API/pulls/$FORGE_PROBE_PR/requested_reviewers" '{"reviewers":["probe-v0.6.0-nonexistent-user"]}'
	probe "reviewers DELETE" DELETE "$API/pulls/$FORGE_PROBE_PR/requested_reviewers" '{"reviewers":["probe-v0.6.0-nonexistent-user"]}'
else
	finding "reviewers" "SKIPPED: FORGE_PROBE_PR unset"
fi

echo "== probe done =="
[ "$TRANSPORT_ERR" -eq 0 ] || echo "TRANSPORT ERRORS occurred; findings above are partial"
exit $TRANSPORT_ERR
