# Docker

The `core` image is **self-contained**: one binary serves both the Go API
(`cmd/api`) and the React dashboard (`web/`) on a single port. The dashboard is
built as a static client-only SPA (TanStack Start SPA mode, no server functions)
and embedded into the binary via `//go:embed` (`internal/web`). One process,
one image, one port — no separate dashboard container.

## How the dashboard gets embedded

`web/`'s `pnpm build` runs Vite in SPA mode, prerenders the app shell to
`index.html`, and copies the static client bundle (`dist/client`) into
`internal/web/dist` (via `web/scripts/embed-dist.mjs`). The Go build then
embeds that directory. The Go handler (`internal/web/web.go`) serves the
assets and falls back to `index.html` for client-side routes.

- Locally: run `make web-build` (or `cd web && pnpm build`) before `go build` /
  `make build` so a real dashboard is embedded.
- In Docker: the `web` node stage does this automatically.
- A committed **placeholder** `internal/web/dist/index.html` (a tiny stub) is
  the only tracked file in that directory, so `go build` compiles on a fresh
  checkout even before any web build. Real built assets are git-ignored.

The dashboard is served **same-origin** with the API, so it calls `/v1/*` via
relative paths. `VITE_KEELWAVE_API_URL` is now optional — set it only for
split-origin local dev (`vite dev` on :3000 hitting the API on :8080).

## Build locally

```bash
docker build --build-arg VERSION="$(git describe --tags --always)" -t keelwave:dev .
```

Multi-stage:

1. `node:22` **web** stage — `pnpm install` + `pnpm build` produces the embedded
   dashboard in `internal/web/dist`.
2. `golang:1.26` **build** stage — copies the built dashboard from the web stage,
   then `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=..."`
   embeds it (cross-compiled via `TARGETOS`/`TARGETARCH`).
3. `gcr.io/distroless/static:nonroot` **runtime** — non-root, no shell.

## Run

Requires a reachable Postgres/TimescaleDB. Minimum:

```bash
docker run --rm -p 8080:8080 \
  -e DB_ADDR="postgres://keelwave:keelwave@host:5432/keelwave?sslmode=disable" \
  keelwave:dev
```

The dashboard is at `/` and the API under `/v1/*` on the same port.

Optional env: `ADDR` (default `:8080`), `ENV`, `CORS_ALLOWED_ORIGINS`,
`PUBLIC_URL`, `DASHBOARD_URL` (point at the app's own origin now that the
dashboard is same-origin), `SESSION_TTL`, `RESEND_API_KEY`, `MAIL_FROM`,
`GOOGLE_*`, `GITHUB_*`, `ALERT_EVAL_INTERVAL`, `SHUTDOWN_TIMEOUT`. Health at
`GET /v1/health`.

## Compose

```bash
docker compose up -d db                 # just the database (unchanged)
docker compose --profile app up --build # db → migrate → app
```

## Migrations

The app image does **not** run migrations on start. Apply them out-of-band with
the `migrate` service (via the `app` profile above) or `make migrate-up` before
rolling a new image.

## Releases

`.github/workflows/release.yml` builds a multi-arch image (amd64 + arm64) and
pushes to `ghcr.io/keelwave/keelwave` on a `v*` tag or a published release, with
provenance + SBOM. Tags: semver (`X.Y.Z`, `X.Y`, `X`), `sha-<sha>`, and `latest`
on tags. Auth uses the built-in `GITHUB_TOKEN` (`packages: write`) — no secrets
needed. Set the package to Public in GitHub Packages if you want anonymous pulls.
