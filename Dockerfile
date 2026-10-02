# syntax=docker/dockerfile:1
#
# MelHttp image: melhttpd + melc on distroless, with the test sites compiled
# into Malbolge under /srv/sites/{angular,react,vue,classic,cgi,hello}.
#
#   docker build -t melhttp .
#   docker run --rm -p 8080:8080 melhttp                        # Angular showcase
#   docker run --rm -p 8080:8080 -e MELHTTP_ROOT=/srv/sites/classic melhttp
#   docker build --target test .                                # run the Go tests
#
# Serve your own site: build it with melc (or the melc service in compose.yaml)
# and mount it:  docker run -p 8080:8080 -v ./site:/srv/site -e MELHTTP_ROOT=/srv/site melhttp

ARG GO_VERSION=1.27
ARG NODE_VERSION=24

# ---- frontend test sites (Angular, React, Vue) ------------------------------
FROM node:${NODE_VERSION}-bookworm-slim AS frontends
WORKDIR /src/testsites
ENV npm_config_fund=false npm_config_audit=false npm_config_update_notifier=false
COPY testsites/angular-showcase/package.json testsites/angular-showcase/package-lock.json angular-showcase/
COPY testsites/react-vite/package.json testsites/react-vite/package-lock.json react-vite/
COPY testsites/vue-vite/package.json testsites/vue-vite/package-lock.json vue-vite/
RUN --mount=type=cache,target=/root/.npm \
    (cd angular-showcase && npm ci) && (cd react-vite && npm ci) && (cd vue-vite && npm ci)
COPY testsites/ ./
RUN (cd angular-showcase && npm run build) && (cd react-vite && npm run build) && (cd vue-vite && npm run build)

# ---- Go build ----------------------------------------------------------------
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/ ./cmd/melc ./cmd/melhttpd

# ---- tests (docker build --target test .) -------------------------------------
FROM build AS test
RUN --mount=type=cache,target=/root/.cache/go-build go vet ./... && go test ./...

# ---- compile the test sites into Malbolge ---------------------------------------
FROM build AS sites
COPY --from=frontends /src/testsites/angular-showcase/dist testsites/angular-showcase/dist
COPY --from=frontends /src/testsites/react-vite/dist testsites/react-vite/dist
COPY --from=frontends /src/testsites/vue-vite/dist testsites/vue-vite/dist
RUN /out/melc build -o /sites/hello testsites/hello \
 && /out/melc build -o /sites/classic testsites/classic \
 && /out/melc build -o /sites/cgi testsites/cgi \
 && /out/melc build --preset angular -o /sites/angular testsites/angular-showcase \
 && /out/melc build --preset vite -o /sites/react testsites/react-vite \
 && /out/melc build --preset vite -o /sites/vue testsites/vue-vite

# ---- runtime ------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/melhttpd /out/melc /usr/local/bin/
COPY --from=sites /sites /srv/sites
ENV MELHTTP_ADDR=:8080 \
    MELHTTP_ROOT=/srv/sites/angular \
    MELHTTP_LOG_FORMAT=json
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD ["/usr/local/bin/melhttpd", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/melhttpd"]
