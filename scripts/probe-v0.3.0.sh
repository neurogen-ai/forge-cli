#!/usr/bin/env sh
# Probes a live Forgejo instance for the three v0.3.0 unknowns from
# plans/releases/v0.3.0.md section 8. Env required:
#   FORGE_PROBE_HOST   e.g. git.ngenesis.co.uk
#   FORGE_PROBE_TOKEN  token with repo access
#   FORGE_PROBE_REPO   owner/name of a PR-heavy test repo, e.g. Neurogenesis/ngen-weave
#   FORGE_PROBE_PR     a PR number on that repo known to carry review comments
# Prints findings as labeled lines; exits non-zero if any probe errors at the
# transport level (not merely "endpoint missing", which is itself a finding).
set -u

HOST="${FORGE_PROBE_HOST:?set FORGE_PROBE_HOST}"
TOK="${FORGE_PROBE_TOKEN:?set FORGE_PROBE_TOKEN}"
REPO="${FORGE_PROBE_REPO:?set FORGE_PROBE_REPO}"
PR="${FORGE_PROBE_PR:?set FORGE_PROBE_PR}"
API="https://$HOST/api/v1/repos/$REPO"
H="Authorization: token $TOK"

echo "== pagination caps =="
for ep in pulls/$PR/reviews "pulls/$PR/comments"; do
  curl -sSI -H "$H" "$API/$ep?limit=50" | grep -i '^link:' || echo "(no Link header => single page)"
done

echo "== resolved field in list payloads =="
curl -sS -H "$H" "$API/pulls/$PR/comments" | head -c 2000; echo

echo "== resolution endpoint candidates =="
CID=$(curl -sS -H "$H" "$API/pulls/$PR/comments" | jq -r '.[0].id // empty')
[ -n "$CID" ] || { echo "no comments to probe against; set FORGE_PROBE_PR to a PR with review comments"; exit 0; }
echo "-- candidate A: PATCH .../pulls/comments/$CID/resolve --"
curl -sS -X PATCH -H "$H" -d '{"resolved":true}' -w '\nHTTP %{http_code}\n' "$API/pulls/comments/$CID/resolve"
echo "-- candidate B: POST .../pulls/comments/$CID/resolve --"
curl -sS -X POST -H "$H" -w '\nHTTP %{http_code}\n' "$API/pulls/comments/$CID/resolve"
echo "-- candidate C: PATCH .../pulls/$PR/comments/$CID/resolve --"
curl -sS -X PATCH -H "$H" -d '{"resolved":true}' -w '\nHTTP %{http_code}\n' "$API/pulls/$PR/comments/$CID/resolve"

echo "== idempotency check on winning candidate =="
curl -sS -X PATCH -H "$H" -d '{"resolved":true}' -w '\nHTTP %{http_code}\n' "$API/pulls/comments/$CID/resolve"
