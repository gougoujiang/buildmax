// Mirror the repository-root help/ manual into portal/public/help so Vite copies
// it verbatim into dist/ and nginx serves the pages at /help/<slug>.md. Run by
// the predev/prebuild npm hooks, and inside Dockerfile.portal after help/ is
// copied into the build stage. The source of truth is help/, never public/help.
import { cpSync, existsSync, mkdirSync, rmSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const here = dirname(fileURLToPath(import.meta.url))
const portalRoot = resolve(here, "..")
const src = resolve(portalRoot, "..", "help")
const dest = resolve(portalRoot, "public", "help")

if (!existsSync(src)) {
  // Fail loudly: a Help icon that opens an empty manual is a broken build, not a
  // recoverable state. In Docker this means help/ was not copied into the stage.
  console.error(`[sync-help] source manual not found at ${src}`)
  process.exit(1)
}

rmSync(dest, { recursive: true, force: true })
mkdirSync(dirname(dest), { recursive: true })
cpSync(src, dest, { recursive: true })
console.log(`[sync-help] copied ${src} -> ${dest}`)
