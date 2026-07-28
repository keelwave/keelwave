# syntax=docker/dockerfile:1

# Stage 1 — build the React dashboard as a static client-only SPA.
# `pnpm build` runs vite (SPA mode) + prerenders the shell to index.html and
# copies dist/client into internal/web/dist so the Go stage can embed it.
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@11.15.1 --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY web/ ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm build

# Stage 2 — build the Go binary with the dashboard embedded.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# Overlay the built dashboard produced by the web stage so //go:embed picks it
# up. (The build context ships only the placeholder internal/web/dist.)
COPY --from=web /src/internal/web/dist ./internal/web/dist

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/keelwave ./cmd/api

# Stage 3 — minimal runtime. Single process, single port, dashboard + API.
FROM gcr.io/distroless/static:nonroot AS runtime
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="keelwave" \
      org.opencontainers.image.description="Open-source observability and alerting for AI agents. API + dashboard in one image." \
      org.opencontainers.image.source="https://github.com/keelwave/keelwave" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=build /out/keelwave /usr/local/bin/keelwave
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/keelwave"]
