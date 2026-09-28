#!/usr/bin/env bash
# Verify install.sh's Hermes hook registration without network, Go, or npm.
#
# What this asserts:
#   - hermes_hook install registers `servitor hook --hermes` as a
#     pre_llm_call shell hook in the Hermes profile config.yaml, preserving
#     an existing commented config (textual append when there is no hooks:
#     key), and is idempotent (second run changes nothing)
#   - a config with an existing hooks: mapping gains the entry via the YAML
#     editor, keeping the other keys, with the original left at .bak
#   - the sad paths refuse and change nothing: config.yaml missing,
#     unparsable, hooks: not a mapping
#   - hermes_hook check distinguishes registered from not
#   - on a dual-agent host (also ~/.claude), hook_json/install_hook are
#     untouched: settings.json is byte-identical before/after hermes work
#   - --check reports the missing hook as drift (via the check() function's
#     hook_json-style call site; here asserted through hermes_hook check)
#
# Method: install.sh is sourced as a library (SERVITOR_INSTALL_LIB=1) with a
# fake HOME holding a stub Hermes venv python (bin/python -> the system
# python3, which needs pyyaml; installed on the fly when missing).
#
#   ./tests/install-hermes.sh
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

FAKE_HOME="${WORK}/home"
mkdir -p "${FAKE_HOME}/.hermes/hermes-agent/venv/bin"
# stub the Hermes venv python: same interpreter, whatever pyyaml it has
ln -sf "$(command -v python3)" "${FAKE_HOME}/.hermes/hermes-agent/venv/bin/python"
if ! python3 -c "import yaml" 2>/dev/null; then
  python3 -m pip install --quiet --user pyyaml
fi

HERMES_CONFIG="${FAKE_HOME}/.hermes/config.yaml"

fail=0
ok()   { printf '  ok   %s\n' "$1"; }
bad()  { printf '  FAIL %s\n' "$1"; fail=1; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want [$3], got [$2])"; fi; }
hasnt() { if printf '%s' "$2" | grep -q -- "$3"; then bad "$1 (found: $3)"; else ok "$1"; fi; }

# shellcheck disable=SC1090
run() { HOME="${FAKE_HOME}" SERVITOR_INSTALL_LIB=1 \
  bash -c ". '${REPO}/install.sh'; $1"; }

echo "== install into a commented config with no hooks key =="
cat > "${HERMES_CONFIG}" <<'EOF'
# my hand-edited hermes config
# keep this comment
model: test-model
EOF
out="$(run 'hermes_hook install' 2>&1)" && ok "install succeeds" || { bad "install succeeds"; printf '%s\n' "$out"; }
grep -q "# keep this comment" "${HERMES_CONFIG}" && ok "comments survive" || bad "comments survive"
grep -q "model: test-model" "${HERMES_CONFIG}" && ok "existing keys survive" || bad "existing keys survive"
grep -q "command: ${FAKE_HOME}/.local/bin/servitor hook --hermes" "${HERMES_CONFIG}" \
  && ok "hook registered with absolute path" || bad "hook registered with absolute path"
run 'hermes_hook check' >/dev/null 2>&1 && ok "check passes after install" || bad "check passes after install"

before="$(cat "${HERMES_CONFIG}")"
run 'hermes_hook install' >/dev/null 2>&1 && ok "idempotent re-run succeeds" || bad "idempotent re-run succeeds"
check "idempotent re-run changes nothing" "${before}" "$(cat "${HERMES_CONFIG}")"
out="$(run 'install_hook_hermes' 2>&1)"
hasnt "re-run when registered is silent (no note)" "${out}" "note:"
[ ! -e "${HERMES_CONFIG}.bak" ] && ok "no .bak for a lossless edit" || bad "no .bak for a lossless edit"

echo
echo "== install into an existing hooks mapping =="
cat > "${HERMES_CONFIG}" <<'EOF'
model: test-model
hooks:
  pre_llm_call:
    - command: ~/.hermes/agent-hooks/other.sh
      timeout: 5
EOF
run 'hermes_hook install' >/dev/null 2>&1 && ok "install with existing hooks key" || bad "install with existing hooks key"
grep -q "other.sh" "${HERMES_CONFIG}" && ok "sibling hook kept" || bad "sibling hook kept"
grep -q "servitor hook --hermes" "${HERMES_CONFIG}" && ok "servitor hook added" || bad "servitor hook added"
[ -e "${HERMES_CONFIG}.bak" ] && ok "original kept at .bak" || bad "original kept at .bak"

echo
echo "== sad paths refuse and change nothing =="
rm -f "${HERMES_CONFIG}"
if out="$(run 'hermes_hook install' 2>&1)"; then bad "missing config must refuse"; else ok "missing config refuses"; fi
has() { if printf '%s' "$2" | grep -q -- "$3"; then ok "$1"; else bad "$1 (missing: $3)"; fi; }
has "missing-config message names the fix" "${out}" "start hermes once"
[ ! -e "${HERMES_CONFIG}" ] && ok "missing config stays missing" || bad "missing config stays missing"

printf 'model: [unclosed\n' > "${HERMES_CONFIG}"
before="$(cat "${HERMES_CONFIG}")"
if out="$(run 'hermes_hook install' 2>&1)"; then bad "unparsable config must refuse"; else ok "unparsable config refuses"; fi
has "unparsable message says does not parse" "${out}" "does not parse"
check "unparsable config untouched" "${before}" "$(cat "${HERMES_CONFIG}")"

printf 'hooks: not-a-mapping\n' > "${HERMES_CONFIG}"
before="$(cat "${HERMES_CONFIG}")"
if out="$(run 'hermes_hook install' 2>&1)"; then bad "non-mapping hooks must refuse"; else ok "non-mapping hooks refuses"; fi
has "non-mapping message names hooks:" "${out}" "not a mapping"
check "non-mapping config untouched" "${before}" "$(cat "${HERMES_CONFIG}")"
run 'hermes_hook check' >/dev/null 2>&1 && bad "check fails on the sad paths" || ok "check fails on the sad paths"

echo
echo "== dual-agent seam: settings.json untouched =="
mkdir -p "${FAKE_HOME}/.claude"
printf '{"model":"claude"}\n' > "${FAKE_HOME}/.claude/settings.json"
printf 'model: test-model\n' > "${HERMES_CONFIG}"
before_settings="$(cat "${FAKE_HOME}/.claude/settings.json")"
run 'install_hook_hermes' >/dev/null 2>&1 && ok "install_hook_hermes runs" || bad "install_hook_hermes runs"
check "settings.json byte-identical" "${before_settings}" "$(cat "${FAKE_HOME}/.claude/settings.json")"

if [ "${fail}" -eq 0 ]; then echo "ALL PASS"; else echo "FAILURES"; exit 1; fi
