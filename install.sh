#!/usr/bin/env bash
# Build servitor from this checkout and redeploy it locally.
#
# Installs the binaries to ~/.local/bin, installs deploy/servitord.service
# into ~/.config/systemd/user (and restarts the servitord user unit), and links the servitor skill into every detected agent skill
# directory (Hermes: ~/.hermes/skills, Claude: ~/.claude/skills), so skill
# edits are live immediately and binary edits take effect after restart.
#
# Usage: ./install.sh [--check] [--tone|--no-tone]
#   --check    report drift and exit 1 if the deployed state differs
#   --tone     link the optional servitor-tone skill (terse procedural
#              reporting register) into every detected agent skill directory
#   --no-tone  skip the tone-skill prompt (non-interactive installs)
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${HOME}/.local/bin"
UNIT="servitord.service"

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
for arg in "$@"; do
  case "${arg}" in
    --tone) WANT_TONE=1 ;;
    --no-tone) WANT_TONE=0; TONE_ASKED=1 ;;
    --check) WANT_CHECK=1 ;;
  esac
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
    found=1
  done
  [ "${found}" -eq 1 ] || echo "warning: no agent skill directory found (looked in ${SKILL_TARGETS[*]})" >&2
}

build() {
  echo "==> building from ${REPO}"
  echo "==> building GUI (web/ -> internal/api/static)"
  (cd "${REPO}/web" && npm run build)
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

if ! unit_installed; then
  install_unit
fi

build
link_skills
[ "${WANT_TONE}" -eq 1 ] && link_tone
restart
echo "done — binaries in ${BIN_DIR}, skills linked: $(skill_dirs | tr '\n' ' ')"
