#!/usr/bin/env bash
# Compile a site to Malbolge and serve it with one command.
#
#   scripts/serve.sh [SITE] [options]
#
# SITE: hello | classic (default) | cgi | angular | react | vue | transport | wasi | <path to your folder>
#   angular/react/vue need Node.js (built on first use); a folder with package.json is built first.
#
# Options:
#   --port N         HTTP port (default 8080)
#   --https          also serve HTTPS with a self-signed certificate (--https-port, default 8443)
#   --watch          rebuild on every change (your own folders)
#   --open           open the browser when ready
#   --metrics        Prometheus metrics on http://127.0.0.1:9090/metrics
#   --obfuscate      Malbolge transport for any site: pages travel to the browser as Malbolge
#   --rebuild        rebuild framework apps even if built before
#   --docker         all eight demo sites in Docker (ports 8080-8087); --docker --stop stops them
#
# Examples:
#   scripts/serve.sh                       # classic demo on http://localhost:8080
#   scripts/serve.sh angular --open
#   scripts/serve.sh angular --obfuscate --open   # the Angular app over the Malbolge transport
#   scripts/serve.sh ~/my-site --watch --open
#   scripts/serve.sh classic --https
#   scripts/serve.sh --docker
#
# Stop the server with Ctrl+C. Works on Linux, macOS and Git Bash on Windows.

source "$(dirname "$0")/_common.sh"

SITE=classic PORT=8080 HTTPS_PORT=8443
HTTPS=0 WATCH=0 OPEN=0 METRICS=0 OBFUSCATE=0 REBUILD=0 DOCKER=0 STOP=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port)       PORT="$2"; shift ;;
    --https-port) HTTPS_PORT="$2"; shift ;;
    --https)      HTTPS=1 ;;
    --watch)      WATCH=1 ;;
    --open)       OPEN=1 ;;
    --metrics)    METRICS=1 ;;
    --obfuscate)  OBFUSCATE=1 ;;
    --rebuild)    REBUILD=1 ;;
    --docker)     DOCKER=1 ;;
    --stop)       STOP=1 ;;
    -h|--help)    sed -n '2,28p' "$0"; exit 0 ;;
    -*)           die "unknown option $1 (see --help)" ;;
    *)            SITE="$1" ;;
  esac
  shift
done

# ---- Docker: all demo sites -------------------------------------------------
if [ "$OBFUSCATE" = 1 ] && { [ "$WATCH" = 1 ] || [ "$DOCKER" = 1 ]; }; then
  die "--obfuscate works with a single site, not with --watch or --docker (in Docker, the transport demo is on port 8086)."
fi
if [ "$DOCKER" = 0 ]; then ensure_go; fi
if [ "$DOCKER" = 1 ]; then
  docker_running || die "Docker is not running. Start Docker and try again."
  cd "$ROOT" || exit 1
  if [ "$STOP" = 1 ]; then docker compose down; exit $?; fi
  step "Building and starting the demo sites in Docker (the first build takes a few minutes)"
  docker compose up --build -d --wait || die "docker compose failed"
  cat <<'EOF'

  http://localhost:8080  Angular Material showcase (Malbolge corner shows page source)
  http://localhost:8081  classic multi-page site
  http://localhost:8082  MelCGI demos (try /echo.txt?hi=1)
  http://localhost:8083  hello
  http://localhost:8084  React
  http://localhost:8085  Vue
  http://localhost:8086  Malbolge transport (reload once) and /_melhttp/playground.html
  http://localhost:8087  WASI handler at /hello.html

Stop them with: scripts/serve.sh --docker --stop
EOF
  [ "$OPEN" = 1 ] && open_url "http://localhost:8080"
  exit 0
fi

# ---- Resolve the site ---------------------------------------------------------
step "Building melc and melhttpd"
build_binaries || die "build failed"

PRESET=static EXTRA=() HINT= FRAMEWORK=0
case "$(echo "$SITE" | tr '[:upper:]' '[:lower:]')" in
  hello|classic|cgi) SRC="$ROOT/testsites/$SITE" ;;
  angular) SRC="$ROOT/testsites/angular-showcase"; PRESET=angular; FRAMEWORK=1; EXTRA+=(-expose-source) ;;
  react)   SRC="$ROOT/testsites/react-vite"; PRESET=vite; FRAMEWORK=1 ;;
  vue)     SRC="$ROOT/testsites/vue-vite"; PRESET=vite; FRAMEWORK=1 ;;
  transport|obfuscated)
    SRC="$ROOT/testsites/obfuscated/site"; EXTRA+=(-obfuscate -playground)
    HINT="Reload the page once; then pages travel as Malbolge (DevTools > Network shows it). Playground: /_melhttp/playground.html" ;;
  wasi)
    SRC="$ROOT/testsites/wasi"; EXTRA+=(-wasi)
    HINT="Open /hello.html: a Go program compiled to WebAssembly answers each request."
    step "Compiling the WASI demo handler"
    (cd "$ROOT" && GOOS=wasip1 GOARCH=wasm go build -o testsites/wasi/hello.html.wasi ./testsites/wasi/src) || die "building the WASI handler failed" ;;
  *)
    [ -d "$SITE" ] || die "Unknown site '$SITE'. Use hello, classic, cgi, angular, react, vue, transport, wasi, or a folder path."
    SRC="$(cd "$SITE" && pwd)"
    [ -f "$SRC/package.json" ] && { PRESET=auto; FRAMEWORK=1; } ;;
esac
NAME="$(basename "$SRC")"
[ "$NAME" = site ] && NAME="$(basename "$(dirname "$SRC")")"
OUT="$ROOT/out/$NAME"

BUILD_ARGS=(--preset "$PRESET")
if [ "$FRAMEWORK" = 1 ] && { [ "$REBUILD" = 1 ] || [ ! -d "$SRC/dist" ]; }; then BUILD_ARGS+=(--run-build); fi

SERVER_ARGS=(-root "$OUT" -addr ":$PORT" ${EXTRA[@]+"${EXTRA[@]}"})
URLS=("http://localhost:$PORT")
if [ "$HTTPS" = 1 ]; then
  SERVER_ARGS+=(-tls-self-signed -tls-addr ":$HTTPS_PORT" -https-redirect=false)
  URLS+=("https://localhost:$HTTPS_PORT  (self-signed: accept the browser warning)")
fi
if [ "$OBFUSCATE" = 1 ] && [[ " ${EXTRA[*]-} " != *" -obfuscate "* ]]; then
  SERVER_ARGS+=(-obfuscate-inject)
  HINT="Malbolge transport is on: reload the page once, then every page and file travels as Malbolge (DevTools > Network: responses carry X-Malbolge-Decoded). Use the http:// address: browsers refuse service workers on self-signed HTTPS."
fi
if [ "$METRICS" = 1 ]; then
  SERVER_ARGS+=(-metrics-addr 127.0.0.1:9090)
  URLS+=("http://127.0.0.1:9090/metrics")
fi

PID=
cleanup() { [ -n "$PID" ] && kill "$PID" 2>/dev/null; }
trap cleanup EXIT
trap 'exit 130' INT TERM

# ---- Watch mode ----------------------------------------------------------------------
if [ "$WATCH" = 1 ]; then
  step "Watching $SRC (Ctrl+C to stop)"
  "$MELC" watch -o "$OUT" -serve ":$PORT" --preset "$PRESET" "$SRC" &
  PID=$!
  if [ "$OPEN" = 1 ] && wait_healthy "http://127.0.0.1:$PORT/healthz" "$PID"; then open_url "http://localhost:$PORT"; fi
  wait "$PID"
  exit $?
fi

# ---- Build and serve ----------------------------------------------------------------
step "Compiling $SRC to Malbolge"
"$MELC" build -o "$OUT" "${BUILD_ARGS[@]}" "$SRC" || die "melc build failed"

step "Serving $NAME (Ctrl+C to stop)"
"$MELHTTPD" "${SERVER_ARGS[@]}" &
PID=$!
wait_healthy "http://127.0.0.1:$PORT/healthz" "$PID" || die "melhttpd did not start (is port $PORT in use? try --port 8090)"
echo
for u in "${URLS[@]}"; do printf '  %s%s%s\n' "$GREEN" "$u" "$RESET"; done
[ -n "$HINT" ] && printf '  %s%s%s\n' "$GRAY" "$HINT" "$RESET"
echo
[ "$OPEN" = 1 ] && open_url "http://localhost:$PORT"
wait "$PID"
