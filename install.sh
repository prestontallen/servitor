#!/usr/bin/env bash
# Build servitor from this checkout and redeploy it locally.
#
# Installs the binaries to ~/.local/bin, installs deploy/servitord.service
# into ~/.config/systemd/user (and restarts the servitord user unit), and
# links the servitor, servitor-dev and ticket-flow skills into every
# detected agent skill directory (Hermes: ~/.hermes/skills, Claude:
# ~/.claude/skills), so skill edits are live immediately and binary edits
# take effect after restart. On hosts with Claude Code it also registers
# the SessionStart hook (`servitor hook`) in ~/.claude/settings.json.
#
# Usage: ./install.sh [--check] [--tone|--no-tone] [--dsn URL] [--token TOKEN]
#   --check    report drift and exit 1 if the deployed state differs
#   --tone     link the optional servitor-tone skill (terse procedural
#              reporting register) into every detected agent skill directory
#   --no-tone  skip the tone-skill prompt (non-interactive installs)
#   --dsn URL  write SERVITOR_DSN into ~/.config/servitor/servitord.env
#              (mode 0600) and run `servitord apply-schema` against it
#              before restarting. Never committed to the repo.
#   --token T  write SERVITOR_TOKEN into the same env file (API auth).
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${HOME}/.local/bin"
UNIT="servitord.service"
ENV_FILE="${HOME}/.config/servitor/servitord.env"
ENV_DIR="${HOME}/.config/servitor"
CLAUDE_SETTINGS="${HOME}/.claude/settings.json"
HOOK_CMD="servitor hook"

BINARIES=(servitor servitord servitor-mcp)

# agent skill roots to try; each skill links to <root>/<name>
SKILL_TARGETS=(
  "${HOME}/.hermes/skills"
  "${HOME}/.claude/skills"
)

skill_roots() {
  local root
  for root in "${SKILL_TARGETS[@]}"; do
    [ -d "${root}" ] && echo "${root}"
  done
  return 0
}

unit_installed() {
  # the deployed unit matches the one in deploy/
  [ -f "${HOME}/.config/systemd/user/${UNIT}" ] \
    && cmp -s "${REPO}/deploy/${UNIT}" "${HOME}/.config/systemd/user/${UNIT}"
}

install_unit() {
  echo "==> installing ${UNIT} to ~/.config/systemd/user"
  mkdir -p "${HOME}/.config/systemd/user"
  cp "${REPO}/deploy/${UNIT}" "${HOME}/.config/systemd/user/${UNIT}"
  systemctl --user daemon-reload
  systemctl --user enable --quiet "${UNIT}" 2>/dev/null || true
}

WANT_TONE=0
WANT_DSN=""
WANT_TOKEN=""
while [ $# -gt 0 ]; do
  case "$1" in
    --tone) WANT_TONE=1 ;;
    --no-tone) WANT_TONE=0; TONE_ASKED=1 ;;
    --check) WANT_CHECK=1 ;;
    --dsn) WANT_DSN="${2:-}"; shift ;;
    --dsn=*) WANT_DSN="${1#*=}" ;;
    --token) WANT_TOKEN="${2:-}"; shift ;;
    --token=*) WANT_TOKEN="${1#*=}" ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

# prompt for the optional tone skill unless the choice was made by flag
# or we're not interactive (non-tty defaults to no)
if [ "${WANT_TONE}" -eq 0 ] && [ "${TONE_ASKED:-0}" -eq 0 ] && [ -t 0 ]; then
  printf "link the optional servitor-tone skill? [y/N] "
  read -r answer
  case "${answer}" in
    [yY]|[yY][eE][sS]) WANT_TONE=1 ;;
  esac
fi
TONE_ASKED=1

link_skill() {
  # link_skill NAME: symlink skills/NAME into every detected agent skill root
  local name="$1" root found=0
  for root in $(skill_roots); do
    echo "==> linking ${name} skill into ${root}"
    # older installs made <root>/servitor a real directory of file links
    [ -d "${root}/${name}" ] && [ ! -L "${root}/${name}" ] && rm -rf "${root}/${name}"
    ln -sfn "${REPO}/skills/${name}" "${root}/${name}"
    found=1
  done
  [ "${found}" -eq 1 ] || echo "warning: no agent skill directory found (looked in ${SKILL_TARGETS[*]})" >&2
}

hook_json() {
  # hook_json check|install: is `servitor hook` a SessionStart hook in
  # settings.json (exit 0/1); install adds it, keeping other keys and hooks,
  # and leaves the file untouched when it is already there.
  python3 - "$1" "${CLAUDE_SETTINGS}" "${HOOK_CMD}" <<'PY'
import json, os, sys
mode, path, cmd = sys.argv[1:4]
s = json.load(open(path)) if os.path.exists(path) else {}
entries = s.setdefault("hooks", {}).setdefault("SessionStart", [])
have = any(h.get("command") == cmd for e in entries for h in e.get("hooks", []))
if mode == "check" or have:
    sys.exit(0 if have else 1)
entries.append({"matcher": "startup|resume|clear|compact",
                "hooks": [{"type": "command", "command": cmd, "timeout": 10}]})
with open(path, "w") as f:
    json.dump(s, f, indent=2)
    f.write("\n")
PY
}

install_hook() {
  # Claude Code hosts only: every new session starts with `servitor hook`
  [ -d "${HOME}/.claude" ] || return 0
  hook_json install && echo "==> SessionStart hook registered in ${CLAUDE_SETTINGS}"
}

build() {
  echo "==> building from ${REPO}"
  echo "==> building GUI (web/ -> internal/api/static)"
  # npm ci installs exactly what package-lock.json pins (fresh checkouts
  # have no node_modules; npx would fetch unpinned vite instead)
  (cd "${REPO}/web" && { [ -x node_modules/.bin/vite ] || npm ci; } && npm run build)
  (cd "${REPO}" && go build -o "${BIN_DIR}" ./cmd/servitor ./cmd/servitord ./cmd/servitor-mcp)
}

restart() {
  echo "==> restarting ${UNIT}"
  systemctl --user restart "${UNIT}"
  systemctl --user is-active --quiet "${UNIT}" \
    || { echo "servitord failed to start" >&2; exit 1; }
  echo "servitord active"
}

# env line helpers: write_env_file installs a fresh env file; ensure_env_file
# creates it only when missing (reruns without --dsn preserve what's there).
env_value() {
  # env_value FILE KEY -> value after KEY= (values may contain '=')
  sed -n "s/^${1}=//p" "$2" | head -1
}

current_unit_dsn() {
  # reuse the DSN from an already-installed unit, if any (migration path)
  local installed="${HOME}/.config/systemd/user/${UNIT}" dsn
  [ -f "${installed}" ] || return 1
  dsn="$(env_value "Environment=SERVITOR_DSN" "${installed}")"
  [ -n "${dsn}" ] || return 1
  printf '%s\n' "${dsn}"
}

write_env_file() {
  local dsn="$1" token="$2"
  mkdir -p "${ENV_DIR}"
  {
    echo "SERVITOR_DSN=${dsn}"
    [ -n "${token}" ] && echo "SERVITOR_TOKEN=${token}"
  } > "${ENV_FILE}"
  chmod 0600 "${ENV_FILE}"
}

ensure_env_file() {
  local dsn token="" existing_token=""
  if [ -f "${ENV_FILE}" ]; then
    if [ -z "${WANT_DSN}" ] && [ -z "${WANT_TOKEN}" ]; then
      echo "==> keeping existing ${ENV_FILE} (pass --dsn to change it)"
      return
    fi
    # rewrite, preserving whatever the caller didn't override
    existing_token="$(env_value SERVITOR_TOKEN "${ENV_FILE}")"
  fi
  if [ -n "${WANT_DSN}" ]; then
    dsn="${WANT_DSN}"
  elif [ -f "${ENV_FILE}" ]; then
    dsn="$(env_value SERVITOR_DSN "${ENV_FILE}")"
  elif dsn="$(current_unit_dsn)"; then
    echo "==> migrating existing unit DSN into ${ENV_FILE}"
  else
    dsn="postgres://localhost:5432/servitor?sslmode=disable"
  fi
  if [ -n "${WANT_TOKEN}" ]; then token="${WANT_TOKEN}"
  elif [ -n "${existing_token}" ]; then token="${existing_token}"; fi
  write_env_file "${dsn}" "${token}"
  echo "==> wrote ${ENV_FILE} (mode 0600)"
}

apply_schema() {
  local dsn="$1"
  echo "==> applying schema"
  if ! SERVITOR_DSN="${dsn}" "${BIN_DIR}/servitord" apply-schema; then
    echo "ERROR: apply-schema failed against ${dsn%%:*}://…" >&2
    echo "       not restarting into a broken schema; fix the DSN and rerun." >&2
    exit 1
  fi
}

check() {
  local drift=0 b dest
  for b in "${BINARIES[@]}"; do
    # binary drift: rebuild to a temp file and compare
    if (cd "${REPO}" && go build -o "/tmp/servitor-check-${b}" ./cmd/${b}); then
      cmp -s "/tmp/servitor-check-${b}" "${BIN_DIR}/${b}" \
        || { echo "drift: ${BIN_DIR}/${b} differs from a fresh build"; drift=1; }
      rm -f "/tmp/servitor-check-${b}"
    fi
  done
  for dest in $(skill_roots); do
    for b in servitor servitor-dev ticket-flow; do
      [ "$(readlink "${dest}/${b}" 2>/dev/null)" = "${REPO}/skills/${b}" ] \
        || { echo "drift: ${dest}/${b} is not linked to this checkout"; drift=1; }
    done
  done
  unit_installed || { echo "drift: ${HOME}/.config/systemd/user/${UNIT} differs from deploy/${UNIT}"; drift=1; }
  if [ -d "${HOME}/.claude" ] && ! hook_json check; then
    echo "drift: no SessionStart hook running '${HOOK_CMD}' in ${CLAUDE_SETTINGS} (run install.sh)"
    drift=1
  fi
  if [ -f "${ENV_FILE}" ]; then
    local mode
    mode="$(stat -c '%a' "${ENV_FILE}")"
    [ "${mode}" = "600" ] || { echo "drift: ${ENV_FILE} mode is ${mode}, want 600"; drift=1; }
  else
    echo "drift: ${ENV_FILE} does not exist (run install.sh, optionally --dsn)"
    drift=1
  fi
  systemctl --user is-active --quiet "${UNIT}" \
    || { echo "drift: ${UNIT} is not running"; drift=1; }
  exit "${drift}"
}

mkdir -p "${BIN_DIR}"

if [ "${WANT_CHECK:-0}" -eq 1 ]; then
  check
fi

# env file first: it may migrate the DSN out of an installed unit, so it
# must run before install_unit replaces that unit with the template
ensure_env_file

if ! unit_installed; then
  install_unit
fi

build
# schema must be current before the daemon restarts onto it; use the file's
# DSN (may have just been written by --dsn)
apply_schema "$(env_value SERVITOR_DSN "${ENV_FILE}")"
for s in servitor servitor-dev ticket-flow; do link_skill "${s}"; done
[ "${WANT_TONE}" -eq 1 ] && link_skill servitor-tone
install_hook
restart
echo "done — binaries in ${BIN_DIR}, skills linked into: $(skill_roots | tr '\n' ' ')"
echo "servitor-mcp: source ${ENV_FILE} for SERVITOR_DSN$( [ -f "${ENV_FILE}" ] && grep -q SERVITOR_TOKEN "${ENV_FILE}" && echo '/SERVITOR_TOKEN' )"
