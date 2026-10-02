# Shared helpers for scripts/test.sh and scripts/serve.sh (sourced).
# Works with bash 3.2+ on Linux, macOS and Git Bash on Windows.

set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) ON_WINDOWS=1; EXE=.exe ;;
  *)                    ON_WINDOWS=0; EXE= ;;
esac
BIN="$ROOT/bin"
MELC="$BIN/melc$EXE"
MELHTTPD="$BIN/melhttpd$EXE"

if [ -t 1 ]; then
  CYAN=$'\033[36m' GREEN=$'\033[32m' YELLOW=$'\033[33m' RED=$'\033[31m' GRAY=$'\033[90m' RESET=$'\033[0m'
else
  CYAN= GREEN= YELLOW= RED= GRAY= RESET=
fi

step() { printf '\n%s==> %s%s\n' "$CYAN" "$*" "$RESET"; }
die()  { printf '%s%s%s\n' "$RED" "$*" "$RESET" >&2; exit 1; }

# ensure_go finds the Go toolchain (also in its default install locations,
# in case this shell started before Go was installed).
ensure_go() {
  command -v go >/dev/null 2>&1 && return 0
  for d in "/c/Program Files/Go/bin" /usr/local/go/bin "$HOME/go/bin" /opt/homebrew/bin; do
    if [ -x "$d/go" ] || [ -x "$d/go.exe" ]; then PATH="$d:$PATH"; export PATH; return 0; fi
  done
  die "Go was not found. Install Go 1.26+ (https://go.dev/dl/) and open a new terminal."
}

# build_binaries builds melc and melhttpd into bin/. The browser VM
# (melhttp.wasm) is regenerated only when missing or older than its sources.
build_binaries() {
  ensure_go
  local wasm="$ROOT/internal/webvm/assets/melhttp.wasm"
  if [ ! -f "$wasm" ] || [ -n "$(cd "$ROOT" && find internal/malbolge internal/gen internal/obfs cmd/melwasm -name '*.go' -newer "$wasm" | head -n 1)" ]; then
    echo "    building the browser VM (melhttp.wasm)"
    (cd "$ROOT" && go generate ./internal/webvm) || return 1
  fi
  (cd "$ROOT" && go build -o bin/ ./cmd/melc ./cmd/melhttpd)
}

docker_running() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

# host_path prints a path Docker on this OS understands (Git Bash: C:/...).
host_path() {
  if [ "$ON_WINDOWS" = 1 ]; then (cd "$1" && pwd -W); else echo "$1"; fi
}

# wait_healthy URL PID [SECONDS]: polls /healthz until 200 or the process dies.
wait_healthy() {
  local url="$1" pid="$2" secs="${3:-60}" i=0
  while [ "$i" -lt $((secs * 4)) ]; do
    kill -0 "$pid" 2>/dev/null || return 1
    curl -fsS -o /dev/null --max-time 2 "$url" 2>/dev/null && return 0
    sleep 0.25
    i=$((i + 1))
  done
  return 1
}

open_url() {
  case "$(uname -s)" in
    Darwin) open "$1" ;;
    MINGW*|MSYS*|CYGWIN*) cmd.exe //c start "" "$1" ;;
    *) command -v xdg-open >/dev/null 2>&1 && xdg-open "$1" >/dev/null 2>&1 || echo "    open $1 in your browser" ;;
  esac
}
