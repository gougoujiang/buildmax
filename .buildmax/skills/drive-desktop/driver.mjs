// REPL driver for the Desktop app's browser bridge. Run against `./make run
// desktop-dev` (not the packaged app — that has no HTTP bridge to attach to).
//
// Reads commands from stdin, one per line; prints one line of result per
// command, so an agent driving this through a shell can send-keys and read
// back deterministically. See SKILL.md for the command reference.
import { createRequire } from 'node:module'
import * as readline from 'node:readline'
import * as fs from 'node:fs'
import * as path from 'node:path'

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
// desktop/frontend has @playwright/test installed (./make e2e desktop-ui uses
// it too); resolving from there avoids a second copy of Playwright and a
// second browser download just for this driver.
const require = createRequire(path.join(ROOT, 'desktop', 'frontend') + path.sep)
const { chromium } = require('@playwright/test')

const BASE_URL = process.env.BUILDMAX_DESKTOP_DEV_URL || 'http://localhost:34115'
const SHOT_DIR = process.env.SCREENSHOT_DIR || '/tmp/buildmax-desktop-shots'
fs.mkdirSync(SHOT_DIR, { recursive: true })

let browser = null
let page = null

const COMMANDS = {
  async launch() {
    if (page) return console.log('already launched')
    browser = await chromium.launch()
    page = await browser.newPage()
    await page.goto(BASE_URL, { waitUntil: 'load' })
    // The app resolves GetAuthStatus() and the project list behind a
    // "Loading…" placeholder before anything real is in the DOM; the .catch
    // covers a build where that copy changed rather than block forever.
    await page
      .getByText('Loading…')
      .waitFor({ state: 'detached', timeout: 15_000 })
      .catch(() => {})
    console.log('launched:', BASE_URL, '- title:', await page.title())
  },

  async ss(name) {
    if (!page) return console.log('ERROR: launch first')
    const f = path.join(SHOT_DIR, (name || `ss-${Date.now()}`) + '.png')
    await page.screenshot({ path: f, fullPage: true })
    console.log('screenshot:', f)
  },

  // DOM click via evaluate(), not locator.click(): this page renders behind
  // the Wails runtime scripts injected into <head>, and evaluate() is one less
  // thing that can disagree with Playwright's own hit-testing.
  async click(sel) {
    if (!page) return console.log('ERROR: launch first')
    const r = await page.evaluate((s) => {
      const el = document.querySelector(s)
      if (!el) return 'NOT_FOUND'
      el.click()
      return 'OK'
    }, sel)
    console.log('click', sel, '->', r)
  },

  async 'click-text'(text) {
    if (!page) return console.log('ERROR: launch first')
    const r = await page.evaluate((t) => {
      const els = [...document.querySelectorAll('button, a, [role="button"]')]
      const el = els.find((e) => e.textContent?.trim() === t) ?? els.find((e) => e.textContent?.includes(t))
      if (!el) return 'NOT_FOUND'
      el.click()
      return 'OK: ' + el.tagName
    }, text)
    console.log('click-text', JSON.stringify(text), '->', r)
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

  // The Go bridge itself, independent of anything the React app does with it
  // — this is what proves the dev server is really injecting bindings, not
  // just serving static assets.
  async bindings() {
    if (!page) return console.log('ERROR: launch first')
    const names = await page.evaluate(() => Object.keys(window.go?.desktop?.App ?? {}))
    console.log(names.length, 'bound methods:', names.slice(0, 10).join(', '), names.length > 10 ? '...' : '')
  },

  async reload() {
    if (!page) return console.log('ERROR: launch first')
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

console.log(`desktop driver - target ${BASE_URL} - "help" for commands, "launch" to start`)
rl.prompt()
