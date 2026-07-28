// Copies the static SPA client build (dist/client) into the Go embed
// directory (../internal/web/dist) so `go:embed` can bundle it into the
// keelwave binary. Runs after `vite build` completes (post-prerender), so
// index.html and all hashed assets are present.
import { cp, rm, mkdir, access } from "node:fs/promises"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const here = dirname(fileURLToPath(import.meta.url))
const src = resolve(here, "..", "dist", "client")
const dest = resolve(here, "..", "..", "internal", "web", "dist")

try {
  await access(resolve(src, "index.html"))
} catch {
  console.error(`embed-dist: ${src}/index.html not found; run the SPA build first`)
  process.exit(1)
}

await rm(dest, { recursive: true, force: true })
await mkdir(dest, { recursive: true })
await cp(src, dest, { recursive: true })
console.log(`embed-dist: copied ${src} -> ${dest}`)
