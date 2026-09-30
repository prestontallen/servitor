#!/usr/bin/env bash
# Verify install.sh's Cursor registration without network, Go, or npm.
#
# What this asserts:
#   - cursor_hook install creates ~/.cursor/hooks.json when there is none,
#     as {version:1, hooks:{sessionStart:[{command, timeout}]}} running
#     `servitor hook --cursor` by absolute path, and is idempotent
#   - an existing hooks.json keeps its other keys, other events and other
#     sessionStart entries; ours is appended
#   - the sad paths refuse and change nothing: unparsable JSON, a schema
#     version other than 1, hooks: not an object
#   - cursor_hook check distinguishes registered from not, and fails on the
#     sad paths (this is the call site --check uses for drift)
#   - ensure_cursor_skill_root creates ~/.cursor/skills when ~/.cursor exists
#     and link_skill then links into it; check_skill_links reports drift
#     before the link and none after
#   - on a host that also has ~/.claude and ~/.hermes, settings.json and
#     config.yaml are byte-identical before/after the Cursor work
#
# Method: install.sh is sourced as a library (SERVITOR_INSTALL_LIB=1) with a
# fake HOME holding ~/.cursor.
#
#   ./tests/install-cursor.sh
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

FAKE_HOME="${WORK}/home"
mkdir -p "${FAKE_HOME}/.cursor"
HOOKS="${FAKE_HOME}/.cursor/hooks.json"
CMD="${FAKE_HOME}/.local/bin/servitor hook --cursor"

fail=0
ok()   { printf '  ok   %s\n' "$1"; }
bad()  { printf '  FAIL %s\n' "$1"; fail=1; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want [$3], got [$2])"; fi; }
has()   { if printf '%s' "$2" | grep -q -- "$3"; then ok "$1"; else bad "$1 (missing: $3)"; fi; }
hasnt() { if printf '%s' "$2" | grep -q -- "$3"; then bad "$1 (found: $3)"; else ok "$1"; fi; }

# shellcheck disable=SC1090
run() { HOME="${FAKE_HOME}" SERVITOR_INSTALL_LIB=1 \
  bash -c ". '${REPO}/install.sh'; $1"; }
# jq-free JSON probes via python3, which install.sh itself requires
probe() { python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print($2)" "${HOOKS}"; }

echo "== install with no hooks.json =="
run 'cursor_hook check' >/dev/null 2>&1 && bad "check fails before install" || ok "check fails before install"
out="$(run 'install_hook_cursor' 2>&1)" && ok "install succeeds" || { bad "install succeeds"; printf '%s\n' "$out"; }
has "install announces the registration" "${out}" "Cursor sessionStart hook registered"
[ -f "${HOOKS}" ] && ok "hooks.json created" || bad "hooks.json created"
check "schema version 1" "$(probe "${HOOKS}" 'd["version"]')" "1"
check "one sessionStart entry" "$(probe "${HOOKS}" 'len(d["hooks"]["sessionStart"])')" "1"
check "entry runs the hook by absolute path" "$(probe "${HOOKS}" 'd["hooks"]["sessionStart"][0]["command"]')" "${CMD}"
check "entry has a timeout" "$(probe "${HOOKS}" 'd["hooks"]["sessionStart"][0]["timeout"]')" "10"
run 'cursor_hook check' >/dev/null 2>&1 && ok "check passes after install" || bad "check passes after install"

before="$(cat "${HOOKS}")"
out="$(run 'install_hook_cursor' 2>&1)" && ok "idempotent re-run succeeds" || bad "idempotent re-run succeeds"
check "idempotent re-run changes nothing" "${before}" "$(cat "${HOOKS}")"
hasnt "re-run when registered is silent" "${out}" "registered"

echo
echo "== install into an existing hooks.json =="
cat > "${HOOKS}" <<'EOF'
{
  "version": 1,
  "hooks": {
    "sessionStart": [{"command": "~/.cursor/hooks/other.sh", "timeout": 5}],
    "afterFileEdit": [{"command": "~/.cursor/hooks/format.sh"}]
  },
  "extra": {"keep": true}
}
EOF
run 'cursor_hook install' >/dev/null 2>&1 && ok "install with existing hooks" || bad "install with existing hooks"
check "sibling sessionStart entry kept" "$(probe "${HOOKS}" 'd["hooks"]["sessionStart"][0]["command"]')" "~/.cursor/hooks/other.sh"
check "ours appended after it" "$(probe "${HOOKS}" 'd["hooks"]["sessionStart"][1]["command"]')" "${CMD}"
check "other event kept" "$(probe "${HOOKS}" 'd["hooks"]["afterFileEdit"][0]["command"]')" "~/.cursor/hooks/format.sh"
check "unknown top-level key kept" "$(probe "${HOOKS}" 'd["extra"]["keep"]')" "True"

echo
echo "== sad paths refuse and change nothing =="
printf '{"version": 1, "hooks": {\n' > "${HOOKS}"
before="$(cat "${HOOKS}")"
if out="$(run 'cursor_hook install' 2>&1)"; then bad "unparsable json must refuse"; else ok "unparsable json refuses"; fi
has "unparsable message says does not parse" "${out}" "does not parse"
check "unparsable file untouched" "${before}" "$(cat "${HOOKS}")"
run 'cursor_hook check' >/dev/null 2>&1 && bad "check fails on unparsable" || ok "check fails on unparsable"

printf '{"version": 2, "hooks": {}}\n' > "${HOOKS}"
before="$(cat "${HOOKS}")"
if out="$(run 'cursor_hook install' 2>&1)"; then bad "version 2 must refuse"; else ok "version 2 refuses"; fi
has "version message names the version" "${out}" "schema version 2"
check "version 2 file untouched" "${before}" "$(cat "${HOOKS}")"

printf '{"version": 1, "hooks": []}\n' > "${HOOKS}"
before="$(cat "${HOOKS}")"
if out="$(run 'cursor_hook install' 2>&1)"; then bad "non-object hooks must refuse"; else ok "non-object hooks refuses"; fi
has "non-object message names hooks" "${out}" "not an object"
check "non-object file untouched" "${before}" "$(cat "${HOOKS}")"
run 'cursor_hook check' >/dev/null 2>&1 && bad "check fails on the sad paths" || ok "check fails on the sad paths"

echo
echo "== skills root: created, linked, drift before and after =="
rm -rf "${FAKE_HOME}/.cursor/skills"
run 'ensure_cursor_skill_root' && ok "ensure_cursor_skill_root runs" || bad "ensure_cursor_skill_root runs"
[ -d "${FAKE_HOME}/.cursor/skills" ] && ok "~/.cursor/skills created" || bad "~/.cursor/skills created"
run 'check_skill_links' >/dev/null 2>&1 && bad "drift reported before linking" || ok "drift reported before linking"
run 'for s in "${SKILLS[@]}"; do link_skill "$s"; done' >/dev/null 2>&1 && ok "link_skill runs" || bad "link_skill runs"
check "servitor skill linked into the Cursor root" "$(readlink "${FAKE_HOME}/.cursor/skills/servitor")" "${REPO}/skills/servitor"
run 'check_skill_links' >/dev/null 2>&1 && ok "no drift after linking" || bad "no drift after linking"

rm -rf "${FAKE_HOME}/.cursor"
run 'ensure_cursor_skill_root' && ok "no ~/.cursor: ensure is a no-op" || bad "no ~/.cursor: ensure is a no-op"
[ ! -e "${FAKE_HOME}/.cursor" ] && ok "no ~/.cursor: nothing created" || bad "no ~/.cursor: nothing created"
run 'install_hook_cursor' >/dev/null 2>&1 && ok "no ~/.cursor: install_hook_cursor is a no-op" || bad "no ~/.cursor: install_hook_cursor is a no-op"
[ ! -e "${HOOKS}" ] && ok "no ~/.cursor: no hooks.json written" || bad "no ~/.cursor: no hooks.json written"

echo
echo "== seam: Claude and Hermes files untouched =="
mkdir -p "${FAKE_HOME}/.cursor" "${FAKE_HOME}/.claude" "${FAKE_HOME}/.hermes"
printf '{"model":"claude"}\n' > "${FAKE_HOME}/.claude/settings.json"
printf 'model: test-model\n' > "${FAKE_HOME}/.hermes/config.yaml"
before_settings="$(cat "${FAKE_HOME}/.claude/settings.json")"
before_hermes="$(cat "${FAKE_HOME}/.hermes/config.yaml")"
run 'ensure_cursor_skill_root; install_hook_cursor' >/dev/null 2>&1 && ok "cursor work runs on the dual host" || bad "cursor work runs on the dual host"
check "settings.json byte-identical" "${before_settings}" "$(cat "${FAKE_HOME}/.claude/settings.json")"
check "config.yaml byte-identical" "${before_hermes}" "$(cat "${FAKE_HOME}/.hermes/config.yaml")"

if [ "${fail}" -eq 0 ]; then echo "ALL PASS"; else echo "FAILURES"; exit 1; fi
