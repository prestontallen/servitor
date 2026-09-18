#!/usr/bin/env bash
# Build servitor from this checkout and redeploy it locally.
#
# Installs the binaries to ~/.local/bin, installs deploy/servitord.service
# into ~/.config/systemd/user (and restarts the servitord user unit), and links the servitor skill into every detected agent skill
# directory (Hermes: ~/.hermes/skills, Claude: ~/.claude/skills), so skill
# edits are live immediately and binary edits take effect after restart.
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

BINARIES=(servitor servitord servitor-mcp)

# agent skill roots to try; the skill links into <root>/servitor
SKILL_TARGETS=(
  "${HOME}/.hermes/skills"
  "${HOME}/.claude/skills"
)

skill_dirs() {
  local root
  for root in "${SKILL_TARGETS[@]}"; do
    [ -d "${root}" ] && echo "${root}/servitor"
  done
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

tone_link_state() {
  # echo "linked" | "missing" | "absent" per agent skill root
  local dest
  for dest in $(skill_dirs); do
    if [ -L "${dest}/../servitor-tone/SKILL.md" ]; then
      echo "linked"
    else
      echo "absent"
    fi
  done
}

link_tone() {
  local dest found=0
  for dest in $(skill_dirs); do
    echo "==> linking servitor-tone skill into ${dest}"
    ln -sfn "${REPO}/skills/servitor-tone" "${dest}/../servitor-tone"
    ln -sfn "${REPO}/skills/servitor-dev" "${dest}/../servitor-dev"
    found=1
  done
  [ "${found}" -eq 1 ] || echo "warning: no agent skill directory found (looked in ${SKILL_TARGETS[*]})" >&2
}

build() {
  echo "==> building from ${REPO}"
  echo "==> building GUI (web/ -> internal/api/static)"
  # npm ci installs exactly what package-lock.json pins (fresh checkouts
  # have no node_modules; npx would fetch unpinned vite instead)
  (cd "${REPO}/web" && { [ -x node_modules/.bin/vite ] || npm ci; } && npm run build)
  (cd "${REPO}" && go build -o "${BIN_DIR}" ./cmd/servitor ./cmd/servitord ./cmd/servitor-mcp)
}

link_skills() {
  local dest found=0
  for dest in $(skill_dirs); do
    echo "==> linking skill into ${dest}"
    mkdir -p "${dest}"
    ln -sfn "${REPO}/skills/servitor/SKILL.md" "${dest}/SKILL.md"
    ln -sfn "${REPO}/skills/servitor/references" "${dest}/references"
    found=1
  done
  [ "${found}" -eq 1 ] || echo "warning: no agent skill directory found (looked in ${SKILL_TARGETS[*]})" >&2
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
  for dest in $(skill_dirs); do
    if [ -L "${dest}/SKILL.md" ] && [ "$(readlink "${dest}/SKILL.md")" = "${REPO}/skills/servitor/SKILL.md" ]; then
      :
    else
      echo "drift: ${dest}/SKILL.md is not linked to this checkout"
      drift=1
    fi
  done
  unit_installed || { echo "drift: ${HOME}/.config/systemd/user/${UNIT} differs from deploy/${UNIT}"; drift=1; }
  if [ "$(git -C "${REPO}" config core.hooksPath || true)" != "scripts/hooks" ]; then
    echo "drift: core.hooksPath is not scripts/hooks (run install.sh to wire the pre-commit guard)"
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
  # tone skill is optional: informational, not drift
  local st
  for st in $(tone_link_state); do
    echo "servitor-tone: ${st} (optional; install.sh --tone to enable)"
  done
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
link_skills
[ "${WANT_TONE}" -eq 1 ] && link_tone
# multi-agent isolation: wire the pre-commit guard into this clone
git -C "${REPO}" config core.hooksPath scripts/hooks
echo "==> pre-commit guard wired (core.hooksPath=scripts/hooks)"
restart
echo "done — binaries in ${BIN_DIR}, skills linked: $(skill_dirs | tr '\n' ' ')"
echo "servitor-mcp: source ${ENV_FILE} for SERVITOR_DSN$( [ -f "${ENV_FILE}" ] && grep -q SERVITOR_TOKEN "${ENV_FILE}" && echo '/SERVITOR_TOKEN' )"
