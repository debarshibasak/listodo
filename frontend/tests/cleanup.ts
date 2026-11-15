import { rmSync } from "node:fs"
import { basename } from "node:path"

export default function cleanup() {
  const directory = process.env.LISTODO_E2E_DATA_DIR
  if (directory && basename(directory).startsWith("listodo-ui-test-"))
    rmSync(directory, { recursive: true, force: true })
}
