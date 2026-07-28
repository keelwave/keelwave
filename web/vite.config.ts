import { defineConfig } from "vite"
import { devtools } from "@tanstack/devtools-vite"
import { tanstackStart } from "@tanstack/react-start/plugin/vite"
import viteReact from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

// The dashboard ships as a client-only SPA embedded into the Go binary.
// TanStack Start SPA mode prerenders the shell to index.html and emits a
// static client bundle with hashed assets; the Go web handler serves them
// with index.html fallback for client-side routing.
const config = defineConfig({
  resolve: { tsconfigPaths: true },
  plugins: [
    devtools(),
    tailwindcss(),
    tanstackStart({
      spa: { enabled: true, prerender: { outputPath: "/index" } },
    }),
    viteReact(),
  ],
})

export default config
