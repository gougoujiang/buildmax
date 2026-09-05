// REPL driver for Portal's browser bundle. Run against a deployment that is
// already serving Portal — `./make kind up` or `./make compose smoke`, both
// at http://localhost:8080 by default. This does not start anything itself;
// unlike Desktop there is no separate dev server to bring up.
//
// Reads commands from stdin, one per line; prints one line of result per
// command, so an agent driving this through a shell can send-keys and read
// back deterministically. See SKILL.md for the command reference.
import { createRequire } from 'node:module'
import * as readline from 'node:readline'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'

const execFileAsync = promisify(execFile)

function repoRoot(start) {
  let dir = start
  while (!fs.existsSync(path.join(dir, 'go.mod'))) {
    const parent = path.dirname(dir)
    if (parent === dir) throw new Error('go.mod not found: run this from inside the buildmax repository')
    dir = parent
  }
  return dir
}

const ROOT = repoRoot(process.cwd())
// portal has @playwright/test installed (./make e2e {local,kind,compose} uses
// it too); resolving from there avoids a second copy of Playwright and a
// second browser download just for this driver.
const require = createRequire(path.join(ROOT, 'portal') + path.sep)
const { chromium } = require('@playwright/test')

const BASE_URL = process.env.BUILDMAX_PORTAL_URL || 'http://localhost:8080'
const SHOT_DIR = process.env.SCREENSHOT_DIR || '/tmp/buildmax-portal-shots'
fs.mkdirSync(SHOT_DIR, { recursive: true })

let browser = null
let page = null
let consoleLog = []

// Tracked from launch, not read on demand: a page that already threw before
// anyone typed `console` must not lose the error, and Playwright has no way
// to ask a page for console history after the fact.
function trackPage(p) {
  consoleLog = []
  p.on('console', (msg) => consoleLog.push({ type: msg.type(), text: msg.text() }))
  p.on('pageerror', (err) => consoleLog.push({ type: 'pageerror', text: err.message }))
}

const COMMANDS = {
  async launch() {
    if (page) return console.log('already launched')
    browser = await chromium.launch()
    page = await browser.newPage()
    trackPage(page)
    await page.goto(BASE_URL, { waitUntil: 'load' })
    console.log('launched:', BASE_URL, '- title:', await page.title())
  },

  // Mints a fresh single-use code with `./make kind login [email]` — a login
  // code cannot be fetched from the browser, since arriving out of band is
  // the whole point of it — then drives the same login-code form
  // portal/e2e/global-setup.ts uses against the real deployment.
  async login(email) {
    if (!page) return console.log('ERROR: launch first')
    const cmdArgs = ['kind', 'login']
    if (email) cmdArgs.push(email)
    let stdout
    try {
      ;({ stdout } = await execFileAsync('./make', cmdArgs, { cwd: ROOT }))
    } catch (e) {
      return console.log('ERROR: ./make kind login failed:', e.stderr || e.message)
    }
    // ./make's own first-run Go build noise (if any) lands on stderr; stdout
    // should be exactly the one JSON line kindLogin prints, but take the last
    // non-blank line rather than assume nothing else was interleaved.
    const lines = stdout.trim().split('\n').filter(Boolean)
    const { email: signedInEmail, code } = JSON.parse(lines[lines.length - 1])
    await page.getByRole('button', { name: /login code/i }).click()
    await page.getByLabel('Email').fill(signedInEmail)
    await page.getByLabel('Login code').fill(code)
    // exact: the form also carries a "Sign in with a password" link, and a
    // substring match would find both and refuse to guess.
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    // Signed in when the login card is gone, the same wait global-setup.ts
    // uses rather than a URL — the post-login landing route can move.
    await page.locator('.login-page__card').waitFor({ state: 'detached', timeout: 15_000 })
    console.log('signed in as', signedInEmail)
  },

  async ss(name) {
    if (!page) return console.log('ERROR: launch first')
    const f = path.join(SHOT_DIR, (name || `ss-${Date.now()}`) + '.png')
    await page.screenshot({ path: f, fullPage: true })
    console.log('screenshot:', f)
  },

  // A real Playwright locator, not a DOM evaluate(): Portal is a plain React
  // SPA behind nginx/ingress with no injected runtime overlay to fight, so
  // Playwright's own actionability checks (visible, scrolled into view,
  // stable) are the more faithful click here.
  async click(sel) {
    if (!page) return console.log('ERROR: launch first')
    try {
      await page.locator(sel).click()
      console.log('click', sel, '-> OK')
    } catch (e) {
      console.log('click', sel, '-> ERROR:', e.message)
    }
  },

  async 'click-text'(text) {
    if (!page) return console.log('ERROR: launch first')
    try {
      await page.getByText(text).first().click()
      console.log('click-text', JSON.stringify(text), '-> OK')
    } catch (e) {
      console.log('click-text', JSON.stringify(text), '-> ERROR:', e.message)
    }
  },

  // Focuses the input behind an accessible label rather than setting .value:
  // Portal's forms are React-controlled, and a raw DOM value assignment does
  // not fire onChange — focus here, then `type` drives real keyboard events.
  async label(text) {
    if (!page) return console.log('ERROR: launch first')
    try {
      await page.getByLabel(text).click()
      console.log('label', JSON.stringify(text), '-> OK')
    } catch (e) {
      console.log('label', JSON.stringify(text), '-> ERROR:', e.message)
    }
  },

  async type(text) {
    if (!page) return console.log('ERROR: launch first')
    await page.keyboard.type(text, { delay: 20 })
    console.log('typed:', JSON.stringify(text))
  },

  async press(key) {
    if (!page) return console.log('ERROR: launch first')
    await page.keyboard.press(key)
    console.log('pressed:', key)
  },

  async wait(sel) {
    if (!page) return console.log('ERROR: launch first')
    try {
      await page.waitForSelector(sel, { timeout: 10_000 })
      console.log('found:', sel)
    } catch {
      console.log('TIMEOUT:', sel)
    }
  },

  async 'wait-text'(text) {
    if (!page) return console.log('ERROR: launch first')
    try {
      await page.getByText(text).first().waitFor({ timeout: 10_000 })
      console.log('found text:', JSON.stringify(text))
    } catch {
      console.log('TIMEOUT waiting for text:', JSON.stringify(text))
    }
  },

  async eval(expr) {
    if (!page) return console.log('ERROR: launch first')
    try {
      console.log(JSON.stringify(await page.evaluate(expr)))
    } catch (e) {
      console.log('ERROR:', e.message)
    }
  },

  async text(sel) {
    if (!page) return console.log('ERROR: launch first')
    console.log(
      await page.evaluate((s) => (s ? document.querySelector(s) : document.body)?.innerText ?? '(null)', sel || null)
    )
  },

  async url() {
    if (!page) return console.log('ERROR: launch first')
    console.log(page.url())
  },

  // The one check a screenshot cannot make: a page can render its shell while
  // every data fetch 500s. `console errors` filters to console.error and
  // uncaught exceptions; bare `console` dumps everything captured so far.
  async console(filter) {
    const entries = filter === 'errors' ? consoleLog.filter((e) => e.type === 'error' || e.type === 'pageerror') : consoleLog
    if (entries.length === 0) return console.log('(none)')
    for (const e of entries) console.log(`[${e.type}] ${e.text}`)
  },

  async reload() {
    if (!page) return console.log('ERROR: launch first')
    consoleLog = []
    await page.reload({ waitUntil: 'load' })
    console.log('reloaded')
  },

  async quit() {
    if (browser) await browser.close().catch(() => {})
    browser = null
    page = null
  },
  help() {
    console.log('commands:', Object.keys(COMMANDS).join(', '))
  },
}

const stdin = fs.createReadStream(null, { fd: fs.openSync('/dev/stdin', 'r') })
const rl = readline.createInterface({ input: stdin, output: process.stdout, prompt: 'driver> ' })

// readline emits every buffered 'line' back to back, without waiting on a
// previous async handler — piped input (a whole script at once, or an agent
// sending several commands before reading output) delivers lines faster than
// launch()/click() resolve. A chain serializes them so "quit" cannot race
// "launch" and exit the process mid-navigation.
let queue = Promise.resolve()

async function handleLine(line) {
  const [cmd, ...rest] = line.trim().split(/\s+/)
  if (!cmd) return safePrompt()
  const fn = COMMANDS[cmd]
  if (!fn) {
    console.log('unknown:', cmd, '- try: help')
    return safePrompt()
  }
  try {
    await fn(rest.join(' '))
  } catch (e) {
    console.log('ERROR:', e.message)
  }
  if (cmd === 'quit') {
    rl.close()
    process.exit(0)
  }
  safePrompt()
}

// Piped input (a whole script at once) hits EOF and closes the underlying
// interface as soon as every line has been read — often before a slow command
// like "launch" (a real browser start) has finished executing further down
// the queue. rl.prompt() throws ERR_USE_AFTER_CLOSE in that case; there is
// nothing useful to prompt for once the input is gone, so this drops it.
function safePrompt() {
  try {
    rl.prompt()
  } catch {
    /* stdin already closed */
  }
}

rl.on('line', (line) => {
  queue = queue.then(() => handleLine(line))
})
rl.on('close', async () => {
  // Piped input (a whole script at once) reaches EOF, and this 'close' fires,
  // as soon as every line has been handed to the queue above — not after the
  // queue has drained. Awaiting it here is what stops a fast EOF from calling
  // quit() while "launch" is still mid-navigation.
  await queue
  await COMMANDS.quit()
  process.exit(0)
})

console.log(`portal driver - target ${BASE_URL} - "help" for commands, "launch" to start`)
rl.prompt()
