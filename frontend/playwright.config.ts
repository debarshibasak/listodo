import { defineConfig } from "@playwright/test"
import { mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"

const dataDir = (process.env.LISTODO_E2E_DATA_DIR ||= mkdtempSync(
  join(tmpdir(), "listodo-ui-test-"),
))
export default defineConfig({
  testDir: "./tests",
  globalTeardown: "./tests/cleanup.ts",
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:34115",
    viewport: { width: 1380, height: 880 },
    launchOptions: { args: ["--single-process", "--no-zygote", "--disable-gpu"] },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: {
    command: `go run ../cmd/listodo --serve --listen 127.0.0.1:34115 --data-dir '${dataDir.replace(/'/g, "'\\''")}'`,
    url: "http://127.0.0.1:34115",
    reuseExistingServer: false,
    timeout: 60000,
    gracefulShutdown: { signal: "SIGINT", timeout: 5000 },
  },
})
