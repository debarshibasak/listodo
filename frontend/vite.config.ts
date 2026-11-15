import path from "node:path"
import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"

const backend = process.env.LISTODO_API_URL || "http://127.0.0.1:34115"

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    {
      name: "listodo-local-token",
      async transformIndexHtml(html) {
        try {
          const response = await fetch(backend)
          const token = (await response.text()).match(
            /<meta name="listodo-token" content="[a-f0-9]+">/,
          )?.[0]
          return token ? html.replace("<head>", `<head>${token}`) : html
        } catch {
          return html
        }
      },
    },
  ],
  server: { proxy: { "/api": backend } },
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  build: { outDir: "../internal/desktop/dist", emptyOutDir: true },
})
