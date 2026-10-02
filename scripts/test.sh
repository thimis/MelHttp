#!/usr/bin/env bash
# Build MelHttp and run its tests, with a pass/fail summary at the end.
#
#   scripts/test.sh --quick   build + unit tests + Hello World smoke test (about a minute)
#   scripts/test.sh           ... + the acceptance goals that need no Docker (a few minutes;
#                             the Angular/React/Vue goals need Node.js and npm)
#   scripts/test.sh --full    ... + Docker and ACME goals, reference interpreter, race detector
#                             (needs Docker running; about 10 minutes)
#
# Works on Linux, macOS and Git Bash on Windows.

source "$(dirname "$0")/_common.sh"

MODE=default
for arg in "$@"; do
  case "$arg" in
    --quick|-q) MODE=quick ;;
    --full|-f)  MODE=full ;;
    -h|--help)  sed -n '2,10p' "$0"; exit 0 ;;
    *) die "unknown option $arg (use --quick or --full)" ;;
  esac
done

ensure_go

NAMES=() RESULTS=()
stage() { # stage NAME COMMAND...
  local name="$1" start rc
  shift
  step "$name"
  start=$(date +%s)
  ( "$@" )
  rc=$?
  NAMES+=("$name")
  if [ "$rc" -eq 0 ]; then RESULTS+=("PASS ($(( $(date +%s) - start ))s)"); else RESULTS+=("FAIL (exit $rc)"); fi
}
skip() { NAMES+=("$1"); RESULTS+=("SKIP  $2"); }

go_in_root() { cd "$ROOT" && go "$@"; }

smoke() {
  local out
  out="$("$MELC" run "$ROOT/testdata/programs/hello.mb")" || return 1
  echo "    $out"
  [ "$out" = "Hello, world." ]
}

race() {
  local src
  src="$(host_path "$ROOT")"
  MSYS_NO_PATHCONV=1 docker run --rm -v "$src:/src" -w /src -e GOFLAGS=-buildvcs=false golang:1.27 \
    sh -c 'go generate ./internal/webvm >/dev/null && go test -race -count=1 ./...'
}

stage "Build melc and melhttpd" build_binaries
stage "Unit tests" go_in_root test ./...
stage "Smoke test: Hello World in Malbolge" smoke

if [ "$MODE" != quick ]; then
  if [ "$MODE" = full ] && docker_running; then export MELHTTP_DOCKER=1; else unset MELHTTP_DOCKER; fi
  [ "$MODE" = full ] && ! docker_running && printf '%sDocker is not running: the Docker goals will be skipped.%s\n' "$YELLOW" "$RESET"
  printf '\n%s    (the acceptance goals take a few minutes; only failures are printed)%s\n' "$GRAY" "$RESET"
  stage "Acceptance goals (G1-G15)" go_in_root test -tags acceptance -count=1 -timeout 90m ./acceptance
fi

if [ "$MODE" = full ]; then
  if docker_running; then
    stage "Reference interpreter (1998 C original, in Docker)" go_in_root test -tags reference -count=1 ./internal/malbolge ./internal/gen
    stage "Race detector (Linux container)" race
  else
    skip "Reference interpreter (1998 C original, in Docker)" "Docker is not running"
    skip "Race detector (Linux container)" "Docker is not running"
  fi
fi

printf '\n%sSummary%s\n' "$CYAN" "$RESET"
failed=0
i=0
while [ "$i" -lt "${#NAMES[@]}" ]; do
  r="${RESULTS[$i]}"
  case "$r" in
    PASS*) c="$GREEN" ;;
    SKIP*) c="$YELLOW" ;;
    *)     c="$RED"; failed=1 ;;
  esac
  printf '  %s%-52s %s%s\n' "$c" "${NAMES[$i]}" "$r" "$RESET"
  i=$((i + 1))
done
exit "$failed"
