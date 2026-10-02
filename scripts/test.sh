#!/usr/bin/env bash
# Build MelHttp and run its tests, with a pass/fail summary at the end.
#
#   scripts/test.sh --quick   build + unit tests + Hello World smoke test (about a minute)
#   scripts/test.sh           ... + acceptance goals G1-G15 (a few minutes; the Angular/React/Vue
#                             goals need Node.js). Docker goals are skipped and listed as "Not run".
#   scripts/test.sh --full    ... + Docker and ACME goals, reference interpreter, race detector.
#                             Strict: a missing tool (Docker, Node.js, git) is a failure, not a
#                             skip, so a passing --full run means everything really ran.
#
# The Windows service goal (G15) only runs on Windows from an elevated prompt.
# Works on Linux, macOS and Git Bash on Windows.

source "$(dirname "$0")/_common.sh"

MODE=default
for arg in "$@"; do
  case "$arg" in
    --quick|-q) MODE=quick ;;
    --full|-f)  MODE=full ;;
    -h|--help)  sed -n '2,12p' "$0"; exit 0 ;;
    *) die "unknown option $arg (use --quick or --full)" ;;
  esac
done

ensure_go

if [ "$MODE" = full ]; then
  export MELHTTP_STRICT=1 MELHTTP_DOCKER=1
  docker_running || printf '%sDocker is not running: --full needs it. The Docker stages will fail.%s\n' "$YELLOW" "$RESET"
else
  unset MELHTTP_STRICT MELHTTP_DOCKER
fi

NAMES=() RESULTS=() NOT_RUN=()
stage() { # stage NAME COMMAND...
  local name="$1" start rc totals
  shift
  step "$name"
  : > "$LAST_REPORT"
  start=$(date +%s)
  ( "$@" )
  rc=$?
  NAMES+=("$name")
  totals="$(grep '^Tests: ' "$LAST_REPORT" | tail -n 1 | sed 's/^Tests: //')"
  if [ "$rc" -eq 0 ]; then RESULTS+=("PASS ($(( $(date +%s) - start ))s)${totals:+  $totals}")
  else RESULTS+=("FAIL (exit $rc)${totals:+  $totals}"); fi
  # Collect skipped tests for the final summary.
  while IFS= read -r line; do NOT_RUN+=("$line"); done < <(awk '/^Skipped/{s=1;next} s&&/^  /{sub(/^  /,"");print;next} {s=0}' "$LAST_REPORT")
}

smoke() {
  local out
  out="$("$MELC" run "$ROOT/testdata/programs/hello.mb")" || return 1
  echo "    $out"
  [ "$out" = "Hello, world." ]
}

need_docker() { docker_running || { echo "Docker is not running"; return 1; }; }

reference() { need_docker && go_tests -tags reference -count=1 ./internal/malbolge ./internal/gen; }

race() {
  need_docker || return 1
  local src
  src="$(host_path "$ROOT")"
  MSYS_NO_PATHCONV=1 docker run --rm -v "$src:/src" -w /src -e GOFLAGS=-buildvcs=false golang:1.27 \
    sh -c 'go generate ./internal/webvm >/dev/null && go test -race -count=1 ./...'
}

stage "Build melc and melhttpd" build_binaries
stage "Unit tests" go_tests ./...
stage "Smoke test: Hello World in Malbolge" smoke

if [ "$MODE" != quick ]; then
  printf '%s    (the acceptance goals take a few minutes; each goal is listed as it finishes)%s\n' "$GRAY" "$RESET"
  stage "Acceptance goals (G1-G15)" go_tests -each -tags acceptance -count=1 -timeout 90m ./acceptance
fi

if [ "$MODE" = full ]; then
  stage "Reference interpreter (1998 C original, in Docker)" reference
  stage "Race detector (Linux container)" race
fi

printf '\n%sSummary%s\n' "$CYAN" "$RESET"
failed=0
i=0
while [ "$i" -lt "${#NAMES[@]}" ]; do
  r="${RESULTS[$i]}"
  case "$r" in
    PASS*) c="$GREEN" ;;
    *)     c="$RED"; failed=1 ;;
  esac
  printf '  %s%-52s %s%s\n' "$c" "${NAMES[$i]}" "$r" "$RESET"
  i=$((i + 1))
done
if [ "${#NOT_RUN[@]}" -gt 0 ]; then
  printf '\n%sNot run (skipped, so not verified by this run):%s\n' "$YELLOW" "$RESET"
  for s in "${NOT_RUN[@]}"; do printf '  %s%s%s\n' "$YELLOW" "$s" "$RESET"; done
fi
rm -f "$LAST_REPORT"
exit "$failed"
