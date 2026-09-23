#!/usr/bin/env bash
# Verify install.sh's release (prebuilt-download) mode without network, Go,
# or npm.
#
# What this asserts:
#   - platform_id maps uname output to release-asset naming, and errors
#     with the supported list on unknown OS/arch (exit nonzero)
#   - verify_checksum passes a genuine sha256 and refuses a tampered one
#     (nonzero exit, clear message)
#   - fetch sends the bearer token only when one is set
#   - download_release unpacks a tarball, installs binaries, and repoints
#     REPO at the release tree
#   - record_mode + check() distinguish release vs source installs
#   - link_skill refuses to link a skill the tree does not carry, drops a
#     dangling link left by an earlier install, and leaves a real directory
#     alone; check_skill_links reports a dangling link as drift
#
# Method: install.sh is sourced as a library (SERVITOR_INSTALL_LIB=1) with a
# stub PATH, a fake HOME, and a stub curl that serves a real tarball built
# here from fixtures, so the unpack/verify path is exercised for real.
#
#   ./tests/install-release.sh
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

STUB="${WORK}/stub"
FAKE_HOME="${WORK}/home"
mkdir -p "${STUB}" "${FAKE_HOME}"

# ---- release fixture: what the workflow packs for linux_amd64 ----
FIXTURE="${WORK}/fixture"
mkdir -p "${FIXTURE}/deploy" "${FIXTURE}/skills/servitor"
printf '#!/bin/sh\necho fake-servitor\n'  > "${FIXTURE}/servitor"
printf '#!/bin/sh\necho fake-servitord\n' > "${FIXTURE}/servitord"
printf '#!/bin/sh\necho fake-mcp\n'      > "${FIXTURE}/servitor-mcp"
printf 'ExecStart=stubbed\n'             > "${FIXTURE}/deploy/servitord.service"
printf 'servitor skill\n'                > "${FIXTURE}/skills/servitor/SKILL.md"
chmod +x "${FIXTURE}/servitor" "${FIXTURE}/servitord" "${FIXTURE}/servitor-mcp"
tar -czf "${WORK}/servitor_linux_amd64.tar.gz" -C "${FIXTURE}" \
  servitor servitord servitor-mcp deploy skills
( cd "${WORK}" && sha256sum servitor_linux_amd64.tar.gz > checksums.txt )

# ---- stub curl: serves the fixture from "releases" ----
SERVE="${WORK}/serve"
mkdir -p "${SERVE}"
cp "${WORK}/servitor_linux_amd64.tar.gz" "${WORK}/checksums.txt" "${SERVE}/"
cat > "${STUB}/curl" <<'EOF'
#!/bin/sh
# record the call; serve the asked-for file to the last -o argument
echo "curl $*" >> "${CALLS}"
out=""
prev=""
for a in "$@"; do
  [ "$prev" = "-o" ] && out="$a"
  prev="$a"
done
case "$*" in
  *servitor_linux_amd64.tar.gz*) cp "$SERVE/servitor_linux_amd64.tar.gz" "$out" ;;
  *checksums.txt*)                cp "$SERVE/checksums.txt" "$out" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "${STUB}/curl"
export CALLS="${WORK}/calls" SERVE
: > "${CALLS}"

fail=0
ok()   { printf '  ok   %s\n' "$1"; }
bad()  { printf '  FAIL %s\n' "$1"; fail=1; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want [$3], got [$2])"; fi; }
has()  { if printf '%s' "$2" | grep -q -- "$3"; then ok "$1"; else bad "$1 (missing: $3)"; fi; }
hasnt(){ if printf '%s' "$2" | grep -q -- "$3"; then bad "$1 (found: $3)"; else ok "$1"; fi; }

# shellcheck disable=SC1090
run() { PATH="${STUB}:${PATH}" HOME="${FAKE_HOME}" SERVITOR_INSTALL_LIB=1 \
  bash -c ". '${REPO}/install.sh'; $1"; }

echo "== platform detection =="
out="$(run 'platform_id')"
check "linux x86_64 maps to linux_amd64" "${out}" "linux_amd64"

out="$(run 'OS=FreeBSD; platform_id' 2>&1)" && bad "unknown OS must fail" || ok "unknown OS fails"
has "unknown OS names the supported list" "${out}" "Release assets exist for Linux and macOS"

out="$(run 'uname() { echo riscv64; }; platform_id' 2>&1)" \
  && bad "unknown arch must fail" || ok "unknown arch fails"
has "unknown arch names the supported list" "${out}" "amd64 and arm64"

echo
echo "== checksum verification =="
if run "verify_checksum '${WORK}/servitor_linux_amd64.tar.gz' '${WORK}/checksums.txt'" >/dev/null 2>&1; then
  ok "genuine checksum passes"
else
  bad "genuine checksum passes"
fi

# tamper: flip a byte inside the tarball, re-check under its checksummed name
mkdir -p "${WORK}/tamperdir"
cp "${WORK}/servitor_linux_amd64.tar.gz" "${WORK}/tamperdir/"
printf 'x' | dd of="${WORK}/tamperdir/servitor_linux_amd64.tar.gz" bs=1 seek=100 conv=notrunc 2>/dev/null
if out="$(run "verify_checksum '${WORK}/tamperdir/servitor_linux_amd64.tar.gz' '${WORK}/checksums.txt'" 2>&1)"; then
  bad "tampered tarball must fail"
else
  ok "tampered tarball fails"
fi
check "tamper message is explicit" "${out}" "ERROR: checksum mismatch for servitor_linux_amd64.tar.gz"

# unknown entry in checksums
printf 'deadbeef  other.tar.gz\n' > "${WORK}/only-other.txt"
if out="$(run "verify_checksum '${WORK}/servitor_linux_amd64.tar.gz' '${WORK}/only-other.txt'" 2>&1)"; then
  bad "unlisted tarball must fail"
else
  ok "unlisted tarball fails"
fi
has "unlisted message names the file" "${out}" "not listed"

echo
echo "== fetch: token only when set =="
: > "${CALLS}"
run "fetch \"\${RELEASES_URL}/latest/download/checksums.txt\" '${WORK}/f1'" >/dev/null
hasnt "no token header without a token" "$(cat "${CALLS}")" "Authorization"

: > "${CALLS}"
GH_TOKEN=stubb run "fetch \"\${RELEASES_URL}/latest/download/checksums.txt\" '${WORK}/f2'" >/dev/null
has "token sent when set" "$(cat "${CALLS}")" "Authorization: Bearer stubb"

: > "${CALLS}"
GITHUB_TOKEN=stubb run "fetch \"\${RELEASES_URL}/latest/download/checksums.txt\" '${WORK}/f3'" >/dev/null
has "GITHUB_TOKEN also honored" "$(cat "${CALLS}")" "Authorization: Bearer stubb"

echo
echo "== download_release: unpack, install, repoint REPO =="
out="$(run 'download_release; echo "REPO=${REPO} TAG=${RELEASE_TAG}"')"
has "REPO is the release tree"   "${out}" "REPO=${FAKE_HOME}/.config/servitor/release"
has "tag defaults to latest"     "${out}" "TAG=latest"
for b in servitor servitord servitor-mcp; do
  [ -x "${FAKE_HOME}/.local/bin/${b}" ] && ok "installed ${b}" || bad "installed ${b}"
done
[ -f "${FAKE_HOME}/.config/servitor/release/deploy/servitord.service" ] \
  && ok "release tree carries deploy/" || bad "release tree carries deploy/"
[ -f "${FAKE_HOME}/.config/servitor/release/skills/servitor/SKILL.md" ] \
  && ok "release tree carries skills/" || bad "release tree carries skills/"

: > "${CALLS}"
out="$(run 'WANT_VERSION=v9.9.9; download_release; echo "TAG=${RELEASE_TAG}"')"
has "--version pins the release" "${out}" "TAG=v9.9.9"
has "pinned URL hits the tag"    "$(cat "${CALLS}")" "download/v9.9.9/servitor_linux_amd64.tar.gz"

echo
echo "== install mode marker =="
run 'record_mode release' >/dev/null
check "marker says release" "$(cat "${FAKE_HOME}/.config/servitor/install-mode")" "release"
check "install_mode reads release" "$(run 'install_mode')" "release"
run 'record_mode source' >/dev/null
check "marker says source" "$(cat "${FAKE_HOME}/.config/servitor/install-mode")" "source"
check "install_mode reads source" "$(run 'install_mode')" "source"
# a pre-release-pipeline install (no marker) reads as source
rm -f "${FAKE_HOME}/.config/servitor/install-mode"
check "no marker reads as source" "$(run 'install_mode')" "source"

echo
echo "== skill links: a name the tree does not carry =="
# The fixture ships skills/servitor and nothing else — the exact shape that
# produced this bug: an install.sh newer than the release, naming skills the
# tarball does not have.
TREE="${FAKE_HOME}/.config/servitor/release"
run 'download_release' >/dev/null
ROOT="${FAKE_HOME}/.claude/skills"
rm -rf "${ROOT}"; mkdir -p "${ROOT}"

out="$(run "REPO='${TREE}'; link_skill servitor; link_skill servitor-review" 2>&1)"
[ -d "${ROOT}/servitor" ] \
  && ok "a skill in the tree is linked" || bad "a skill in the tree is linked"
[ -L "${ROOT}/servitor-review" ] \
  && bad "a skill not in the tree must not be linked" \
  || ok "a skill not in the tree is not linked"
has "the warning names the skill and the tree" "${out}" \
  "no servitor-review skill in ${TREE}/skills"

# a link an earlier install left behind now points at nothing
ln -sfn "${TREE}/skills/servitor-review" "${ROOT}/servitor-review"
out="$(run "REPO='${TREE}'; link_skill servitor-review" 2>&1)"
[ -L "${ROOT}/servitor-review" ] \
  && bad "a dangling link must be removed" || ok "a dangling link is removed"
has "removing the dangling link is announced" "${out}" "removing dangling servitor-review link"

# a link pointing somewhere real is not ours to delete
mkdir -p "${WORK}/elsewhere/servitor-plan"
ln -sfn "${WORK}/elsewhere/servitor-plan" "${ROOT}/servitor-plan"
run "REPO='${TREE}'; link_skill servitor-plan" >/dev/null 2>&1
check "a link with a live target is left alone" \
  "$(readlink "${ROOT}/servitor-plan")" "${WORK}/elsewhere/servitor-plan"

# a real directory an operator put there is never removed
rm -f "${ROOT}/servitor-plan"
mkdir -p "${ROOT}/servitor-plan"
printf 'operator own skill\n' > "${ROOT}/servitor-plan/SKILL.md"
run "REPO='${TREE}'; link_skill servitor-plan" >/dev/null 2>&1
[ -f "${ROOT}/servitor-plan/SKILL.md" ] \
  && ok "a real directory is left alone" || bad "a real directory is left alone"
rm -rf "${ROOT}/servitor-plan"

echo
echo "== the skipped-skill report =="
out="$(run 'SKIPPED_SKILLS=(servitor-plan servitor-review); report_skipped_skills' 2>&1)" \
  && ok "the report does not fail the install" || bad "the report does not fail the install"
has "the report names each skipped skill" "${out}" "servitor-review"
has "the report says what to do"          "${out}" "install.sh --from-source"
check "nothing skipped, nothing said" "$(run 'report_skipped_skills' 2>&1)" ""

echo
echo "== --check sees a dangling link =="
# right readlink, no target: the drift the old readlink-only test could not see
rm -rf "${ROOT}"; mkdir -p "${ROOT}"
for b in servitor servitor-dev ticket-flow servitor-plan servitor-review; do
  ln -sfn "${TREE}/skills/${b}" "${ROOT}/${b}"
done
out="$(run "REPO='${TREE}'; check_skill_links" 2>&1)" \
  && bad "a dangling link must be drift" || ok "a dangling link is drift"
has "dangling drift says the target is not there" "${out}" \
  "${ROOT}/servitor-review links to ${TREE}/skills/servitor-review, which is not there"
hasnt "a dangling link is not reported as unlinked" "${out}" \
  "${ROOT}/servitor-review is not linked"
hasnt "the one real skill is not drift" "${out}" "${ROOT}/servitor is not"

# a wrong readlink still reports as unlinked
ln -sfn /nowhere/servitor "${ROOT}/servitor"
out="$(run "REPO='${TREE}'; check_skill_links" 2>&1)" || true
has "a wrong target reports as not linked" "${out}" "${ROOT}/servitor is not linked to this checkout"

echo
if [ "${fail}" -eq 0 ]; then
  echo "PASS — stubbed releases only. A live end-to-end run happens on the first real tag."
else
  echo "FAIL"
fi
exit "${fail}"
