#!/usr/bin/env bash
# Verify install.sh's macOS service handling without a Mac.
#
# The Mac that would run this for real is not the machine this repo is
# developed on, so the merge cannot be gated on it. What IS checkable here is
# our own logic: given uname reporting Darwin, does install.sh render the right
# plist, hand launchd the right subcommands, and keep its hands off systemctl?
# That is what this asserts. It is NOT a claim that servitord runs on macOS —
# that needs real hardware and has its own ticket.
#
# Method: install.sh is sourced as a library (SERVITOR_INSTALL_LIB=1) with a
# stub PATH in front, so svc_* can be called directly without building Go,
# running npm, or touching a real service manager.
#
#   ./tests/install-darwin.sh
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

STUB="${WORK}/stub"
FAKE_HOME="${WORK}/home"
CALLS="${WORK}/calls"
mkdir -p "${STUB}" "${FAKE_HOME}"
: > "${CALLS}"

# uname reports Darwin; launchctl and BSD stat record what they were asked.
cat > "${STUB}/uname" <<'EOF'
#!/bin/sh
echo Darwin
EOF

cat > "${STUB}/launchctl" <<EOF
#!/bin/sh
echo "launchctl \$*" >> "${CALLS}"
EOF

# systemctl must never be reached on Darwin; if it is, fail loudly rather than
# silently passing because the stub happened to be harmless.
cat > "${STUB}/systemctl" <<EOF
#!/bin/sh
echo "SYSTEMCTL CALLED ON DARWIN: \$*" >> "${CALLS}"
exit 1
EOF

# BSD stat: -f '%Lp'. GNU's -c '%a' must NOT appear.
cat > "${STUB}/stat" <<EOF
#!/bin/sh
if [ "\$1" = "-c" ]; then
  echo "GNU STAT FLAG USED ON DARWIN: \$*" >> "${CALLS}"
  exit 1
fi
echo "stat \$*" >> "${CALLS}"
echo 600
EOF

chmod +x "${STUB}"/*

fail=0
ok()   { printf '  ok   %s\n' "$1"; }
bad()  { printf '  FAIL %s\n' "$1"; fail=1; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want [$3], got [$2])"; fi; }
has()  { if printf '%s' "$2" | grep -q -- "$3"; then ok "$1"; else bad "$1 (missing: $3)"; fi; }
hasnt(){ if printf '%s' "$2" | grep -q -- "$3"; then bad "$1 (found: $3)"; else ok "$1"; fi; }

# shellcheck disable=SC1090
run() { PATH="${STUB}:${PATH}" HOME="${FAKE_HOME}" SERVITOR_INSTALL_LIB=1 bash -c ". '${REPO}/install.sh'; $1"; }

echo "== criterion 1: renders a plist and drives launchctl, not systemctl =="
out="$(run 'echo "OS=$OS"; echo "FILE=$(svc_file)"')"
has "uname Darwin is detected" "${out}" "OS=Darwin"
has "service file is the LaunchAgents plist" "${out}" "FILE=${FAKE_HOME}/Library/LaunchAgents/com.prestontallen.servitord.plist"

run 'svc_install' >/dev/null
PLIST_OUT="${FAKE_HOME}/Library/LaunchAgents/com.prestontallen.servitord.plist"
if [ -f "${PLIST_OUT}" ]; then ok "svc_install wrote the plist"; else bad "svc_install wrote the plist"; fi

calls="$(cat "${CALLS}")"
has "bootstrap was issued"        "${calls}" "launchctl bootstrap gui/"
has "bootout precedes bootstrap"  "${calls}" "launchctl bootout gui/"
hasnt "systemctl was never called" "${calls}" "SYSTEMCTL CALLED"

: > "${CALLS}"
run 'svc_restart' >/dev/null
has "restart uses kickstart -k" "$(cat "${CALLS}")" "launchctl kickstart -k gui/"

: > "${CALLS}"
run 'svc_is_active' >/dev/null 2>&1 || true
has "is_active uses launchctl print" "$(cat "${CALLS}")" "launchctl print gui/"

echo
echo "== criterion 2: no secret material in the plist =="
plist="$(cat "${PLIST_OUT}")"
# Strip XML comments first: the template explains at length why the DSN and an
# EnvironmentVariables block are NOT used, and those words must not be mistaken
# for the thing they warn about.
plist_cfg="$(sed '/<!--/,/-->/d' "${PLIST_OUT}")"
hasnt "no SERVITOR_DSN in the plist"  "${plist_cfg}" "SERVITOR_DSN"
hasnt "no postgres:// URL"            "${plist_cfg}" "postgres://"
hasnt "no EnvironmentVariables block" "${plist_cfg}" "EnvironmentVariables"
has   "sources the 0600 env file"     "${plist_cfg}" 'servitord.env'

echo
echo "== criterion 3: SERVITOR_ADDR precedence matches systemd =="
# The unit lists Environment=SERVITOR_ADDR after EnvironmentFile=, so the
# service manager wins. The wrapper must therefore assign it AFTER sourcing.
wrapper="$(printf '%s' "${plist}" | grep -F 'set -a')"
src_pos="$(awk '{print index($0, "servitord.env")}' <<<"${wrapper}")"
addr_pos="$(awk '{print index($0, "SERVITOR_ADDR=")}' <<<"${wrapper}")"
if [ "${addr_pos}" -gt "${src_pos}" ] && [ "${src_pos}" -gt 0 ]; then
  ok "SERVITOR_ADDR is assigned after the env file is sourced"
else
  bad "SERVITOR_ADDR precedence (src=${src_pos} addr=${addr_pos})"
fi

echo
echo "== criterion 4: templating and BSD stat =="
hasnt "no unexpanded placeholder left" "${plist}" "@HOME@"
hasnt "no literal \$HOME in path keys" "$(printf '%s' "${plist}" | grep -F '<string>/' || true)" '$HOME'
has   "log path is absolute and templated" "${plist}" "${FAKE_HOME}/Library/Logs/servitord.log"

: > "${CALLS}"
mkdir -p "${FAKE_HOME}/.config/servitor"
printf 'SERVITOR_DSN=postgres://servitor@example:5432/servitor?sslmode=disable\n' \
  > "${FAKE_HOME}/.config/servitor/servitord.env"
mode="$(run 'stat_mode "${ENV_FILE}"')"
check "stat_mode returns the octal mode" "${mode}" "600"
hasnt "GNU stat -c was not used" "$(cat "${CALLS}")" "GNU STAT FLAG USED"

echo
echo "== forward-compat: the health check targets the managed daemon =="
# When servitord eventually runs on the database host and this machine is only
# a client, SERVITOR_API will point at that remote daemon. install.sh restarts
# a LOCAL service, so its health check must ignore the ambient value or a
# healthy remote would vouch for a broken local install.
api="$(PATH="${STUB}:${PATH}" HOME="${FAKE_HOME}" SERVITOR_API=http://elsewhere.invalid:9999 \
  SERVITOR_INSTALL_LIB=1 bash -c ". '${REPO}/install.sh'; svc_local_api")"
check "ambient SERVITOR_API is ignored" "${api}" "http://127.0.0.1:8181"

echo
echo "== seam: Linux is unaffected =="
linux_file="$(HOME="${FAKE_HOME}" SERVITOR_INSTALL_LIB=1 bash -c ". '${REPO}/install.sh'; svc_file")"
check "Linux still resolves to the systemd unit" "${linux_file}" "${FAKE_HOME}/.config/systemd/user/servitord.service"

echo
if [ "${fail}" -eq 0 ]; then
  echo "PASS — stubbed Darwin only. Real-hardware verification is a separate ticket."
else
  echo "FAIL"
fi
exit "${fail}"
