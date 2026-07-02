#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-daily}"
MCP_URL="${MCP_URL:-http://localhost:8082/mcp}"
WIKA_API_BASE="${WIKA_API_BASE:-http://localhost:8080/api/v1}"

TEMP_TOKEN_IDS=()
CREATED_TOKEN=""
DAILY_PAT_VALUE=""
ADMIN_PAT_VALUE=""

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing command: $1"
}

future_iso() {
  python3 -c 'from datetime import datetime, timezone, timedelta; print((datetime.now(timezone.utc) + timedelta(days=7)).isoformat(timespec="seconds").replace("+00:00", "Z"))'
}

inspector() {
  npx -y @modelcontextprotocol/inspector --cli "$MCP_URL" --transport http "$@"
}

create_token() {
  local name="$1"
  local scopes_json="$2"
  local expires_at body response code response_body token token_id prefix

  [[ -n "${WIKA_JWT:-}" ]] || fail "creating test PAT requires WIKA_JWT"
  expires_at="$(future_iso)"
  body="$(jq -n --arg name "$name" --arg expires_at "$expires_at" --argjson scopes "$scopes_json" \
    '{name: $name, scopes: $scopes, expires_at: $expires_at}')"
  response="$(curl -sS -w '\n%{http_code}' "$WIKA_API_BASE/wika/tokens" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $WIKA_JWT" \
    -d "$body")"
  code="$(printf '%s' "$response" | tail -n1)"
  response_body="$(printf '%s' "$response" | sed '$d')"
  [[ "$code" == "201" ]] || fail "create token failed, http=$code, body=$response_body"

  token="$(printf '%s' "$response_body" | jq -r '.token // empty')"
  token_id="$(printf '%s' "$response_body" | jq -r '.id // empty')"
  prefix="$(printf '%s' "$response_body" | jq -r '.token_prefix // empty')"
  [[ -n "$token" && -n "$token_id" ]] || fail "create token response missing token or id"
  TEMP_TOKEN_IDS+=("$token_id")
  CREATED_TOKEN="$token"
  echo "created temporary PAT id=$token_id prefix=$prefix" >&2
}

cleanup() {
  [[ -n "${WIKA_JWT:-}" ]] || return 0
  [[ "${KEEP_TEST_TOKENS:-false}" != "true" ]] || return 0
  [[ "${#TEMP_TOKEN_IDS[@]}" -gt 0 ]] || return 0
  for token_id in "${TEMP_TOKEN_IDS[@]}"; do
    curl -sS -o /dev/null -X DELETE "$WIKA_API_BASE/wika/tokens/$token_id" \
      -H "Authorization: Bearer $WIKA_JWT" || true
    echo "revoked temporary PAT id=$token_id" >&2
  done
}
trap cleanup EXIT

ensure_daily_pat() {
  if [[ -n "${WIKA_DAILY_PAT:-}" ]]; then
    DAILY_PAT_VALUE="$WIKA_DAILY_PAT"
    return
  fi
  create_token "MCP Inspector daily smoke" '["knowledge:push","knowledge:search","knowledge:read","suggestion:create"]'
  DAILY_PAT_VALUE="$CREATED_TOKEN"
}

ensure_admin_pat() {
  if [[ -n "${WIKA_ADMIN_PAT:-}" ]]; then
    ADMIN_PAT_VALUE="$WIKA_ADMIN_PAT"
    return
  fi
  create_token "MCP Inspector admin smoke" '["knowledge:push","knowledge:search","knowledge:read","suggestion:create","mcp:admin"]'
  ADMIN_PAT_VALUE="$CREATED_TOKEN"
}

expect_permission_denied() {
  local description="$1"
  shift
  local output code
  set +e
  output="$("$@" 2>&1)"
  code=$?
  set -e
  [[ "$code" -ne 0 ]] || fail "$description should fail, but exited 0"
  grep -q "Permission denied" <<<"$output" || fail "$description failed without Permission denied: $output"
  echo "ok: $description -> Permission denied"
}

verify_daily_tools() {
  local pat="$1"
  local output
  output="$(inspector --method tools/list --header "Authorization: Bearer $pat")"
  printf '%s' "$output" | jq -e '
    (.tools | length) == 5 and
    ([.tools[].name] | index("push_knowledge") != null) and
    ([.tools[].name] | index("search_knowledge") != null) and
    ([.tools[].name] | index("expand_knowledge_result") != null) and
    ([.tools[].name] | index("get_my_knowledge") != null) and
    ([.tools[].name] | index("suggest_to_team") != null) and
    ([.tools[].name] | index("list_knowledge_bases") == null)
  ' >/dev/null
  echo "ok: daily tools/list only exposes 5 daily tools"
}

run_daily() {
  local pat unique push_output push_text knowledge_id get_output search_output expand_output

  ensure_daily_pat
  pat="$DAILY_PAT_VALUE"
  verify_daily_tools "$pat"

  unique="mcp-inspector-daily-$(date +%Y%m%d%H%M%S)"
  push_output="$(inspector \
    --method tools/call \
    --tool-name push_knowledge \
    --tool-arg "title=MCP Inspector daily smoke" \
    --tool-arg "content=这是一条通过 MCP Inspector daily PAT 写入的验收知识，唯一标识 ${unique}。" \
    --tool-arg "source=inspector-cli" \
    --tool-arg "idempotency_key=$unique" \
    --header "Authorization: Bearer $pat")"
  push_text="$(printf '%s' "$push_output" | jq -r '.content[0].text')"
  knowledge_id="$(printf '%s' "$push_text" | jq -r '.knowledge_id // empty')"
  [[ -n "$knowledge_id" ]] || fail "push_knowledge did not return knowledge_id: $push_text"
  echo "ok: push_knowledge created knowledge_id=$knowledge_id"

  get_output="$(inspector --method tools/call --tool-name get_my_knowledge --tool-arg limit=5 --header "Authorization: Bearer $pat")"
  printf '%s' "$get_output" | jq -e --arg id "$knowledge_id" '(.content[0].text | fromjson | .results | map(.knowledge_id) | index($id)) != null' >/dev/null
  echo "ok: get_my_knowledge can read created knowledge"

  search_output="$(inspector \
    --method tools/call \
    --tool-name search_knowledge \
    --tool-arg "query=$unique" \
    --tool-arg limit=5 \
    --tool-arg include_team=false \
    --header "Authorization: Bearer $pat")"
  printf '%s' "$search_output" | jq -e --arg id "$knowledge_id" '(.content[0].text | fromjson | .results | map(.knowledge_id) | index($id)) != null' >/dev/null
  echo "ok: search_knowledge can find created knowledge"

  expand_output="$(inspector \
    --method tools/call \
    --tool-name expand_knowledge_result \
    --tool-arg "ids=[\"$knowledge_id\"]" \
    --header "Authorization: Bearer $pat")"
  printf '%s' "$expand_output" | jq -e --arg id "$knowledge_id" '(.content[0].text | fromjson | .results | map(.knowledge_id) | index($id)) != null' >/dev/null
  echo "ok: expand_knowledge_result can expand created knowledge"
}

run_admin() {
  local daily admin list_without_header admin_list admin_call direct_call

  ensure_daily_pat
  ensure_admin_pat
  daily="$DAILY_PAT_VALUE"
  admin="$ADMIN_PAT_VALUE"

  list_without_header="$(inspector --method tools/list --header "Authorization: Bearer $admin")"
  printf '%s' "$list_without_header" | jq -e '
    (.tools | length) == 5 and
    ([.tools[].name] | index("list_knowledge_bases") == null) and
    ([.tools[].name] | index("push_knowledge") != null)
  ' >/dev/null
  echo "ok: admin PAT without admin header still gets daily toolset"

  expect_permission_denied "admin header without token" \
    inspector --method tools/list --header "X-Wika-MCP-Toolset: admin"
  expect_permission_denied "admin header with invalid PAT" \
    inspector --method tools/list --header "Authorization: Bearer wika_pat_invalid" --header "X-Wika-MCP-Toolset: admin"
  expect_permission_denied "admin header with daily PAT missing mcp:admin" \
    inspector --method tools/list --header "Authorization: Bearer $daily" --header "X-Wika-MCP-Toolset: admin"

  admin_list="$(inspector \
    --method tools/list \
    --header "Authorization: Bearer $admin" \
    --header "X-Wika-MCP-Toolset: admin")"
  printf '%s' "$admin_list" | jq -e '
    (.tools | length) > 5 and
    ([.tools[].name] | index("list_knowledge_bases") != null) and
    ([.tools[].name] | index("push_knowledge") != null)
  ' >/dev/null
  echo "ok: authorized admin PAT exposes admin toolset"

  admin_call="$(inspector \
    --method tools/call \
    --tool-name list_knowledge_bases \
    --header "Authorization: Bearer $admin" \
    --header "X-Wika-MCP-Toolset: admin")"
  printf '%s' "$admin_call" | jq -e '(.content[0].text | fromjson | .success) == true' >/dev/null
  echo "ok: authorized admin PAT can call list_knowledge_bases"

  direct_call="$(inspector \
    --method tools/call \
    --tool-name list_knowledge_bases \
    --header "Authorization: Bearer $admin")"
  printf '%s' "$direct_call" | jq -e '(.content[0].text | contains("not available in the daily MCP toolset"))' >/dev/null
  echo "ok: admin tool cannot be called without admin header"
}

need_cmd curl
need_cmd jq
need_cmd npx
need_cmd python3

case "$MODE" in
  daily)
    run_daily
    ;;
  admin)
    run_admin
    ;;
  all)
    run_daily
    run_admin
    ;;
  *)
    fail "usage: $0 daily|admin|all"
    ;;
esac

echo "MCP Inspector smoke passed: mode=$MODE mcp_url=$MCP_URL"
