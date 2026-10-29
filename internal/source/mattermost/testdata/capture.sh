#!/usr/bin/env bash
# Capture real Mattermost API responses for the unit tests.
#
# 1. Start a new server: docker compose -f test/e2e/mattermost/compose.yaml down -v && \
#    docker compose -f test/e2e/mattermost/compose.yaml up -d
# 2. Run this script from the repository root:
#    internal/source/mattermost/testdata/capture.sh
#
# The script makes users alice and bob, a team "fx", the channels "selected" and
# "other", and a direct channel. Then bob writes the posts that the tests need.
# It saves each response that the source reads, and manifest.json, which maps
# each request to its file. Then alice sends two replies, as i2a does, and the
# script saves the responses of the send requests (reply_*.json, not in the
# manifest). Needs: curl, python3.
set -euo pipefail

BASE="${MM_URL:-http://localhost:18065}/api/v4"
PASS='Passw0rd!e2e'
OUT="$(cd "$(dirname "$0")" && pwd)"

until curl -sf "$BASE/system/ping" >/dev/null; do sleep 2; done

json() { python3 -I -c "import json,sys;print(json.load(sys.stdin)$1)"; }
api() { # api <token> <method> <path> [body]
  local t=$1 m=$2 p=$3 b=${4:-}
  if [[ -n $b ]]; then
    curl -sf -X "$m" "$BASE$p" -H "Authorization: Bearer $t" -H 'Content-Type: application/json' -d "$b"
  else
    curl -sf -X "$m" "$BASE$p" -H "Authorization: Bearer $t"
  fi
}
mkuser() { curl -s "$BASE/users" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$1@example.com\",\"username\":\"$1\",\"password\":\"$PASS\"}" >/dev/null; }
login() { curl -s -i "$BASE/users/login" -H 'Content-Type: application/json' \
  -d "{\"login_id\":\"$1\",\"password\":\"$PASS\"}" | sed -n 's/^[Tt]oken: *//p' | tr -d '\r'; }

mkuser alice; mkuser bob
A=$(login alice); B=$(login bob)
AID=$(api "$A" GET /users/me | json '["id"]')
BID=$(api "$B" GET /users/me | json '["id"]')

TEAM=$(api "$A" POST /teams '{"name":"fx","display_name":"FX","type":"O"}' | json '["id"]')
api "$A" POST "/teams/$TEAM/members" "{\"team_id\":\"$TEAM\",\"user_id\":\"$BID\"}" >/dev/null
mkch() { api "$A" POST /channels "{\"team_id\":\"$TEAM\",\"name\":\"$1\",\"display_name\":\"$1\",\"type\":\"O\"}" | json '["id"]'; }
SEL=$(mkch selected); OTH=$(mkch other)
for c in $SEL $OTH; do api "$A" POST "/channels/$c/members" "{\"user_id\":\"$BID\"}" >/dev/null; done
DM=$(api "$B" POST /channels/direct "[\"$BID\",\"$AID\"]" | json '["id"]')

post() { api "$1" POST /posts "{\"channel_id\":\"$2\",\"message\":\"$3\"$4}" | json '["id"]'; sleep 0.05; }
# selected: 4 posts by bob, 1 own post by alice, 1 deleted post by bob
post "$B" "$SEL" "selected one" "" >/dev/null
post "$A" "$SEL" "my own post" "" >/dev/null
DEL=$(post "$B" "$SEL" "this post is deleted" "")
api "$B" DELETE "/posts/$DEL" >/dev/null
post "$B" "$SEL" "selected two" "" >/dev/null
post "$B" "$SEL" "selected three" "" >/dev/null
post "$B" "$SEL" "selected four" "" >/dev/null
# other: one mention, three posts that are not mentions
post "$B" "$OTH" "noise in other" "" >/dev/null
post "$B" "$OTH" "@alice please check the report." "" >/dev/null
post "$B" "$OTH" "mail me at x@alice.example" "" >/dev/null
post "$B" "$OTH" "@alicex is someone else" "" >/dev/null
# direct: one post and one reply in its thread
ROOT=$(post "$B" "$DM" "direct hi alice" "")
RID=$(post "$B" "$DM" "reply in thread" ",\"root_id\":\"$ROOT\"")

PAT=$(api "$A" POST "/users/$AID/tokens" '{"description":"capture"}' | json '["token"]')

manifest="{"
save() { # save <name> <method> <path> [body]
  api "$PAT" "$2" "$3" "${4:-}" | python3 -I -m json.tool > "$OUT/$1.json"
  manifest+="\"$2 $3\":\"$1.json\","
}
save me GET /users/me
save teams GET /users/me/teams
save channels GET "/users/me/teams/$TEAM/channels"
# Save the posts of every member channel: with mentions on, the source reads all of them.
CHANNELS=$(python3 -I -c 'import json,sys
for c in json.load(open(sys.argv[1])):
    print(("direct" if c["type"] == "D" else c["name"]) + ":" + c["id"])' "$OUT/channels.json")
for ch in $CHANNELS; do
  name=${ch%%:*}; id=${ch#*:}
  save "posts_${name}_200_p0" GET "/channels/$id/posts?page=0&per_page=200"
  for p in 0 1 2 3; do save "posts_${name}_2_p$p" GET "/channels/$id/posts?page=$p&per_page=2"; done
done
save users_ids POST /users/ids "[\"$BID\",\"$AID\"]"
save post_root GET "/posts/$ROOT"
save post_reply GET "/posts/$RID"
api "$PAT" GET /users/me/teams/unread | python3 -I -m json.tool > "$OUT/unread.json"
# The send requests of i2a. They run after the saves above, so the post lists
# above do not contain the replies.
keep() { python3 -I -m json.tool > "$OUT/$1.json"; }
curl -s "$BASE/posts/$DEL" -H "Authorization: Bearer $PAT" | keep reply_get_deleted
api "$PAT" POST "/posts?set_online=false" \
  "{\"channel_id\":\"$DM\",\"message\":\"Thanks Bob, noted.\",\"root_id\":\"$ROOT\",\"pending_post_id\":\"$AID:1700000000000\"}" | keep reply_create_thread
api "$PAT" POST "/posts?set_online=false" \
  "{\"channel_id\":\"$SEL\",\"message\":\"Noted, thanks.\",\"pending_post_id\":\"$AID:1700000000001\"}" | keep reply_create_channel
api "$PAT" GET "/channels/$DM/posts?page=0&per_page=200" | keep reply_posts_direct_after
manifest="${manifest%,}}"
printf '%s\n' "$manifest" | python3 -I -m json.tool > "$OUT/manifest.json"
echo "captured $(ls "$OUT"/*.json | wc -l | tr -d ' ') files in $OUT"
