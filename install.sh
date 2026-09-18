#!/usr/bin/env bash
# Build servitor from this checkout and redeploy it locally.
#
# Installs the binaries to ~/.local/bin, restarts the servitord systemd
# user unit, and links the servitor skill into every detected agent skill
# directory (Hermes: ~/.hermes/skills, Claude: ~/.claude/skills), so skill
# edits are live immediately and binary edits take effect after restart.
#
# Usage: ./install.sh [--check]
#   --check  report drift and exit 1 if the deployed state differs
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

build() {
  echo "==> building from ${REPO}"
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
  systemctl --user is-active --quiet "${UNIT}" \
    || { echo "drift: ${UNIT} is not running"; drift=1; }
  exit "${drift}"
}

mkdir -p "${BIN_DIR}"

if [ "${1:-}" = "--check" ]; then
  check
fi

build
link_skills
restart
echo "done — binaries in ${BIN_DIR}, skills linked: $(skill_dirs | tr '\n' ' ')"
