import { createEnv } from "@t3-oss/env-core"
import { z } from "zod"

export const env = createEnv({
  server: {},

  clientPrefix: "VITE_",

  client: {
    // Optional. When the dashboard is served same-origin by the keelwave
    // binary (single-image deploy), leave this unset and the API client
    // uses relative paths against the current origin. Set it only when the
    // dashboard and API live on different origins (e.g. local `vite dev`).
    VITE_KEELWAVE_API_URL: z.url().optional(),
    VITE_KEELWAVE_API_KEY: z.string().optional(),
  },

  runtimeEnv: import.meta.env,

  emptyStringAsUndefined: true,
})
