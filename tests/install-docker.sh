#!/usr/bin/env bash
# Verify install.sh's local docker TimescaleDB bootstrap without docker.
#
# The postgres entrypoint starts two servers on a fresh volume: a temporary
# one on the unix socket only, while initdb hooks run, then the real one on
# TCP. A readiness probe over the socket passes during the first, and the
# bootstrap psql lands in the gap when it shuts down ("the database system is
# shutting down" — a real install log). The stub docker below models exactly
# that: every `docker exec` is one tick; the socket answers pg_isready from
# tick one, TCP only from TCP_READY_AT; psql before that tick fails the way
# the real one did. What this asserts is our own ordering and our own
# failure handling, not the container.
#
# Method: install.sh is sourced as a library (SERVITOR_INSTALL_LIB=1) with a
# stub PATH in front; sleep is a no-op so the poll budget costs nothing.
#
#   ./tests/install-docker.sh
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

STUB="${WORK}/stub"
FAKE_HOME="${WORK}/home"
export CALLS="${WORK}/calls" TICKS="${WORK}/ticks"
mkdir -p "${STUB}" "${FAKE_HOME}"

cat > "${STUB}/docker" <<'EOF'
#!/bin/sh
echo "docker $*" >> "${CALLS}"
case "$1" in
  ps)  exit 0 ;;                       # no container named servitor-db yet
  run) echo "0123456789abcdef"; exit 0 ;;
  rm)  exit 0 ;;
  exec) ;;
  *)   echo "unexpected docker verb: $*" >&2; exit 1 ;;
esac
shift
while [ $# -gt 0 ]; do
  case "$1" in
    -i) shift ;;
    -e) shift 2 ;;
    servitor-db) shift; break ;;
    *) echo "unexpected docker exec flag: $1" >&2; exit 1 ;;
  esac
done
cmd="$1"; shift
n=$(cat "${TICKS}" 2>/dev/null || echo 0); n=$((n + 1)); echo "${n}" > "${TICKS}"
tcp=0
case " $* " in *" -h "*) tcp=1 ;; esac
case "${cmd}" in
  pg_isready)
    if [ "${tcp}" -eq 1 ]; then [ "${n}" -ge "${TCP_READY_AT}" ]; exit $?; fi
    exit 0 ;;                          # the init-phase server answers the socket at once
  psql)
    if [ "${n}" -lt "${TCP_READY_AT}" ]; then
      echo 'psql: error: connection to server on socket "/var/run/postgresql/.s.PGSQL.5432" failed: FATAL:  the database system is shutting down' >&2
      exit 2
    fi
    cat >/dev/null                     # the SQL on stdin, if any
    case "${PSQL_FAIL:-}:$*" in
      role:*)                         echo 'psql: error: stub refused CREATE ROLE' >&2; exit 1 ;;
      database:*"CREATE DATABASE"*)   echo 'psql: error: stub refused CREATE DATABASE' >&2; exit 1 ;;
    esac
    exit 0 ;;
  *) echo "unexpected docker exec command: ${cmd}" >&2; exit 1 ;;
esac
EOF

cat > "${STUB}/sleep" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod +x "${STUB}"/*

fail=0
ok()   { printf '  ok   %s\n' "$1"; }
bad()  { printf '  FAIL %s\n' "$1"; fail=1; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want [$3], got [$2])"; fi; }
has()  { if printf '%s' "$2" | grep -q -- "$3"; then ok "$1"; else bad "$1 (missing: $3)"; fi; }
hasnt(){ if printf '%s' "$2" | grep -q -- "$3"; then bad "$1 (found: $3)"; else ok "$1"; fi; }

reset() { : > "${CALLS}"; echo 0 > "${TICKS}"; export TCP_READY_AT="${1:-1}" PSQL_FAIL="${2:-}"; }
# shellcheck disable=SC1090
run() { PATH="${STUB}:${PATH}" HOME="${FAKE_HOME}" SERVITOR_INSTALL_LIB=1 \
  bash -c ". '${REPO}/install.sh'; $1"; }
# the env-file flow prompts only on a terminal; the test has none
run_tty() { run "interactive() { return 0; }; $1"; }

WANT="postgres://servitor:s3cret@localhost:5432/servitor?sslmode=disable"
ENV="${FAKE_HOME}/.config/servitor/servitord.env"

echo "== the init-phase race: socket answers first, TCP later =="
reset 5
out="$(printf 'y\ns3cret\n' | run 'ask_docker_dsn' 2>"${WORK}/err")"; rc=$?
check "bootstrap waits for TCP and succeeds" "${rc}" "0"
check "DSN is the servitor role at localhost" "${out}" "${WANT}"
has   "readiness probed over TCP" "$(cat "${CALLS}")" "pg_isready -h 127.0.0.1"
hasnt "no psql hit the shutting-down server" "$(cat "${WORK}/err")" "shutting down"
hasnt "nothing removed on success" "$(cat "${CALLS}")" "docker rm"

echo
echo "== TCP never comes up =="
reset 1000
out="$(printf 'y\ns3cret\n' | run 'ask_docker_dsn' 2>&1)"; rc=$?
check "bootstrap fails" "$( [ "${rc}" -ne 0 ] && echo nonzero )" "nonzero"
has   "says the container never became ready" "${out}" "container postgres never became ready"
has   "removes the container it started" "$(cat "${CALLS}")" "docker rm -f servitor-db"
hasnt "no psql was attempted" "$(cat "${CALLS}")" "psql"

echo
echo "== CREATE ROLE fails after the container is up =="
reset 1 role
out="$(printf 'y\ns3cret\n' | run 'ask_docker_dsn' 2>&1)"; rc=$?
check "a failed bootstrap returns 2, distinct from a decline" "${rc}" "2"
has   "removes the half-built container" "$(cat "${CALLS}")" "docker rm -f servitor-db"

echo
echo "== fresh install: a failed bootstrap aborts before the env file =="
rm -f "${ENV}"; reset 1 role
out="$(printf '\ny\ns3cret\n' | run_tty 'ensure_env_file' 2>&1)"; rc=$?
check "install.sh exits non-zero" "$( [ "${rc}" -ne 0 ] && echo nonzero )" "nonzero"
check "no env file written" "$( [ -e "${ENV}" ] && echo written || echo absent )" "absent"
has   "names the removed container" "${out}" "servitor-db container"
has   "names the kept volume" "${out}" "servitor-db-data"
hasnt "no localhost fallback" "${out}" "falling back"

echo
echo "== fresh install: declining docker keeps the localhost fallback =="
rm -f "${ENV}"; reset 1
out="$(printf '\nn\n' | run_tty 'ensure_env_file' 2>&1)"; rc=$?
check "install continues" "${rc}" "0"
has   "warns about the fallback" "${out}" "falling back to postgres://localhost:5432/servitor"
check "env file carries the fallback DSN" "$(grep '^SERVITOR_DSN=' "${ENV}")" "SERVITOR_DSN=postgres://localhost:5432/servitor?sslmode=disable"
hasnt "docker was not started" "$(cat "${CALLS}")" "docker run"

echo
echo "== fresh install: accepting docker writes the bootstrap DSN =="
rm -f "${ENV}"; reset 5
out="$(printf '\ny\ns3cret\n' | run_tty 'ensure_env_file' 2>&1)"; rc=$?
check "install continues" "${rc}" "0"
check "env file carries the docker DSN" "$(grep '^SERVITOR_DSN=' "${ENV}")" "SERVITOR_DSN=${WANT}"

echo
echo "== --reconfigure: a failed bootstrap aborts and keeps the old env file =="
mkdir -p "$(dirname "${ENV}")"; printf 'SERVITOR_DSN=postgres://old:5432/servitor\nSERVITOR_TOKEN=tok\n' > "${ENV}"
reset 1 database
out="$(printf '\ny\ns3cret\n' | run_tty 'WANT_RECONFIGURE=1; ensure_env_file' 2>&1)"; rc=$?
check "install.sh exits non-zero" "$( [ "${rc}" -ne 0 ] && echo nonzero )" "nonzero"
check "old env file untouched" "$(cat "${ENV}")" "$(printf 'SERVITOR_DSN=postgres://old:5432/servitor\nSERVITOR_TOKEN=tok\n')"
has   "names the kept volume" "${out}" "servitor-db-data"

echo
echo "== --reconfigure: declining keeps the old DSN =="
reset 1
out="$(printf '\nn\n' | run_tty 'WANT_RECONFIGURE=1; ensure_env_file' 2>&1)"; rc=$?
check "install continues" "${rc}" "0"
has   "says it kept the DSN" "${out}" "keeping the DSN already in"
check "old DSN still there" "$(grep '^SERVITOR_DSN=' "${ENV}")" "SERVITOR_DSN=postgres://old:5432/servitor"

echo
if [ "${fail}" -eq 0 ]; then
  echo "PASS — stubbed docker only. The real container's timing is covered by a manual run (see the ticket)."
else
  echo "FAIL"
fi
exit "${fail}"
