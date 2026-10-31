#!/usr/bin/env bash
# Record docs/demo/i2a-config.gif with VHS against the local test Mattermost server.
#
# Needs: vhs (brew install vhs), the e2e server
#   (docker compose -f test/e2e/mattermost/compose.yaml up -d), and bin/i2a (task build).
# The first user on that server must be e2e-admin: run the e2e suite one time first.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

URL="${MM_URL:-http://localhost:18065}"
API="$URL/api/v4"
PASS='Passw0rd!e2e'
json() { python3 -I -c "import json,sys;print(json.load(sys.stdin)$1)"; }
login() { curl -s -i "$API/users/login" -H 'Content-Type: application/json' \
  -d "{\"login_id\":\"$1\",\"password\":\"$PASS\"}" | sed -n 's/^[Tt]oken: *//p' | tr -d '\r'; }
call() { curl -sf -X "$2" "$API$3" -H "Authorization: Bearer $1" -H 'Content-Type: application/json' ${4:+-d "$4"}; }

S=$(date +%s | tail -c 5)
for u in alice bob; do
  curl -s "$API/users" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$u$S@example.com\",\"username\":\"$u$S\",\"password\":\"$PASS\"}" >/dev/null
done
ADMIN=$(login e2e-admin); A=$(login "alice$S"); B=$(login "bob$S")
AID=$(call "$A" GET /users/me | json '["id"]'); BID=$(call "$B" GET /users/me | json '["id"]')
call "$ADMIN" PUT "/users/$AID/roles" '{"roles":"system_user system_user_access_token"}' >/dev/null
TEAM=$(call "$A" POST /teams "{\"name\":\"acme$S\",\"display_name\":\"Acme\",\"type\":\"O\"}" | json '["id"]')
call "$A" POST "/teams/$TEAM/members" "{\"team_id\":\"$TEAM\",\"user_id\":\"$BID\"}" >/dev/null
for c in releases design support; do
  id=$(call "$A" POST /channels "{\"team_id\":\"$TEAM\",\"name\":\"$c\",\"display_name\":\"$c\",\"type\":\"O\"}" | json '["id"]')
  call "$A" POST "/channels/$id/members" "{\"user_id\":\"$BID\"}" >/dev/null
  [[ $c == releases ]] && REL=$id
done
TOKEN=$(call "$A" POST "/users/$AID/tokens" '{"description":"demo"}' | json '["token"]')
call "$B" POST /posts "{\"channel_id\":\"$REL\",\"message\":\"v2.3 is tagged. Can you check the release notes before 5pm?\"}" >/dev/null
DM=$(call "$B" POST /channels/direct "[\"$BID\",\"$AID\"]" | json '["id"]')
call "$B" POST /posts "{\"channel_id\":\"$DM\",\"message\":\"Lunch at 12:30?\"}" >/dev/null

TAPE=$(mktemp -t i2a-tape).tape
cat > "$TAPE" <<TAPE
Output docs/demo/i2a-config.gif
Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 680
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms
Env PATH "$PWD/bin:$(dirname "$(command -v ttyd)"):/usr/bin:/bin"
Env I2A_HOME "/tmp/i2a-demo"
Env I2A_SECRET_BACKEND "file"
Env PS1 "$ "
Hide
Type "rm -rf /tmp/i2a-demo && clear" Enter
Show
Type "i2a config" Sleep 400ms Enter Sleep 2.5s
Type "3" Sleep 1.5s
Enter Sleep 800ms
Tab Sleep 300ms Tab Sleep 300ms Type "Acme" Sleep 300ms
Tab Sleep 300ms Type "$URL" Sleep 300ms Tab Sleep 300ms
Hide
Type "$TOKEN"
Show
Sleep 500ms Enter Sleep 3s
Type "4" Sleep 2s
Type "/" Sleep 300ms Type "rel" Sleep 600ms Enter Sleep 1s
Type "m" Sleep 1s Down Sleep 600ms Enter Sleep 800ms
Type "Releases" Sleep 500ms Enter Sleep 2s
Type "5" Sleep 1s Down Sleep 3s
Type "7" Sleep 2.5s
Type "q" Sleep 1s
Type "i2a run --once" Sleep 400ms Enter Sleep 2.5s
Type "head -12 /tmp/i2a-demo/digests/*.md" Sleep 400ms Enter Sleep 4s
TAPE
vhs "$TAPE"
rm -f "$TAPE"
echo "wrote docs/demo/i2a-config.gif"
