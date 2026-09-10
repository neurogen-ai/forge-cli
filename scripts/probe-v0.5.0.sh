#!/usr/bin/env sh
# Probes a live Forgejo instance for the anchored-comment wire encoding that
# branch F of plans/implementation/v0.5.0.md commits to (probe-first rule from
# plans/releases/v0.5.0.md section 2). Env required:
#   FORGE_PROBE_HOST   e.g. git.ngenesis.co.uk
#   FORGE_PROBE_TOKEN  token with repo write access
#   FORGE_PROBE_REPO   owner/name of a test repo, e.g. Neurogenesis/ngen-weave
#   FORGE_PROBE_PR     a PR number whose diff has at least one hunk
# Posts candidate review-comment shapes (event COMMENT, one inline comment
# entry each) to POST /repos/{owner}/{repo}/pulls/{pr}/reviews and prints
# which encoding the server accepted, as labeled lines. Cleans up created
# reviews afterward; cleanup failures are findings, not fatal. Exits non-zero
# if any probe errors at the transport level; a server rejection of a shape
# is itself a finding, not an error.
set -u

HOST="${FORGE_PROBE_HOST:?set FORGE_PROBE_HOST}"
TOK="${FORGE_PROBE_TOKEN:?set FORGE_PROBE_TOKEN}"
REPO="${FORGE_PROBE_REPO:?set FORGE_PROBE_REPO}"
PR="${FORGE_PROBE_PR:?set FORGE_PROBE_PR}"
API="https://$HOST/api/v1/repos/$REPO"
H="Authorization: token $TOK"
CT="Content-Type: application/json"
TRANSPORT_ERR=0
ACCEPTED=""
RANGE_SUPPORT="unknown"

# post_shape LABEL JSON
# POSTs one candidate review shape, prints a labeled finding line, and
# remembers the review id for cleanup. Returns via global RV_REVIEW_ID.
post_shape() {
  label="$1"
  payload="$2"
  RV_REVIEW_ID=""
  resp=$(curl -sS -X POST -H "$H" -H "$CT" -d "$payload" \
    -w '\n%{http_code}' "$API/pulls/$PR/reviews" 2>&1)
  curl_rc=$?
  if [ $curl_rc -ne 0 ]; then
    echo "anchor-shape $label: TRANSPORT-ERROR $resp"
    TRANSPORT_ERR=1
    return 0
  fi
  status=$(printf '%s' "$resp" | tail -n 1)
  body=$(printf '%s' "$resp" | sed '$d')
  case "$status" in
    2*) verdict=accepted ;;
    *) verdict=rejected ;;
  esac
  excerpt=$(printf '%s' "$body" | head -c 200 | tr -d '\n')
  echo "anchor-shape $label: $status $verdict $excerpt"
  if [ "$verdict" = accepted ]; then
    ACCEPTED="$ACCEPTED $label"
    RV_REVIEW_ID=$(printf '%s' "$body" | jq -r '.id // empty' 2>/dev/null)
  fi
}

echo "== anchored-comment shape probe against $HOST $REPO PR $PR =="
echo "-- candidate 1: gh-shaped {path, body, line, side} --"
post_shape gh '{"event":"COMMENT","body":"probe: gh-shaped","comments":[{"path":"README.md","body":"probe: gh-shaped","line":1,"side":"new"}]}'
rv1=$RV_REVIEW_ID

echo "-- candidate 2: gh-shaped old-side {path, body, line, side: old} --"
post_shape gh-old '{"event":"COMMENT","body":"probe: gh-shaped old side","comments":[{"path":"README.md","body":"probe: gh-shaped old side","line":1,"side":"old"}]}'
rv2=$RV_REVIEW_ID

echo "-- candidate 3: contract skeleton {path, body, old_line_num, new_line_num} --"
post_shape line-num '{"event":"COMMENT","body":"probe: line-num shape","comments":[{"path":"README.md","body":"probe: line-num shape","old_line_num":1,"new_line_num":1}]}'
rv3=$RV_REVIEW_ID

echo "-- candidate 4: both encodings combined --"
post_shape both '{"event":"COMMENT","body":"probe: combined","comments":[{"path":"README.md","body":"probe: combined","line":1,"side":"new","old_line_num":1,"new_line_num":1}]}'
rv4=$RV_REVIEW_ID

echo "-- candidate 5: range-style fields (out of scope; finding only, for v0.6.0) --"
post_shape range '{"event":"COMMENT","body":"probe: range shape","comments":[{"path":"README.md","body":"probe: range shape","line":2,"side":"new","start_line":1,"start_side":"new"}]}'
rv5=$RV_REVIEW_ID
if [ -n "$rv5" ]; then
  RANGE_SUPPORT="supported"
else
  RANGE_SUPPORT="not-supported (or shape invalid)"
fi

echo "== cleanup =="
for rv in $rv1 $rv2 $rv3 $rv4 $rv5; do
  [ -n "$rv" ] || continue
  cstatus=$(curl -sS -o /dev/null -w '%{http_code}' -X DELETE -H "$H" \
    "$API/pulls/$PR/reviews/$rv" 2>&1)
  case "$cstatus" in
    2*) echo "cleanup review $rv: $cstatus ok" ;;
    *) echo "cleanup review $rv: $cstatus CLEANUP-FAILED (finding, not fatal)" ;;
  esac
done

echo "== summary =="
if [ -n "$ACCEPTED" ]; then
  echo "accepted encodings:$ACCEPTED"
else
  echo "accepted encodings: none (instance cannot anchor via the review-comment transport)"
fi
echo "range-support finding (v0.6.0): $RANGE_SUPPORT"

[ $TRANSPORT_ERR -eq 0 ] || exit 1
exit 0
