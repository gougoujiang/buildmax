import { expect, type Page } from "@playwright/test"

/**
 * What the specs share: who they are signed in as, how they seed through the
 * API, and how they name what they leave behind.
 *
 * Seeding goes through the API rather than through forms. What these specs test
 * is whether Portal reports the deployment truthfully — not whether a create
 * form submits — and driving setup through the UI would spend the run's budget
 * on parts covered elsewhere, then fail for reasons unrelated to the subject.
 */

/**
 * Tag for everything a run creates.
 *
 * These specs attach to a deployment they do not own, and most resources have
 * no delete route, so what they create stays. A fixed name would leave a
 * deployment holding a dozen identical objects with no way to tell which run
 * left which. `./make e2e` supplies the id; a bare `npx playwright test` gets
 * "local".
 */
export const RUN_ID = process.env.BUILDMAX_E2E_RUN_ID ?? "local"

/** A name no other run will produce, for a resource this one creates. */
export function tagged(name: string): string {
  return `${name} ${RUN_ID}`
}

export interface Session {
  token: string
  spaceId: string
  /** Origin the API answers on, which is not always the one serving the Portal. */
  apiBase: string
  /** Space-scoped API prefix, which is what nearly every call needs. */
  space: string
}

/**
 * The credentials and API origin the running app is actually using.
 *
 * Two things have to be discovered rather than assumed.
 *
 * `page.request` does not inherit the session: the Portal authenticates with a
 * bearer token in localStorage, not a cookie, so the saved storage state that
 * signs the browser in leaves the request context anonymous.
 *
 * And the API base is a property of the deployment, not of the Portal. Behind
 * one ingress it is same-origin; the Compose quickstart publishes the server on
 * its own port. Taking it from the app's own first call keeps these specs
 * correct on both without restating the precedence in `lib/api/client.ts`.
 */
export async function session(page: Page): Promise<Session> {
  const spacesRequest = page.waitForRequest((req) => /\/api\/spaces(\?|$)/.test(req.url()))
  await page.goto("/")
  const url = (await spacesRequest).url()
  const apiBase = url.slice(0, url.indexOf("/api/spaces"))

  // The space is stored only after `GET /api/spaces` answers, so this waits for
  // the app to settle rather than reading straight after navigation.
  const handle = await page.waitForFunction(() => {
    const token = localStorage.getItem("buildmax_token")
    const spaceId = localStorage.getItem("buildmax_current_space")
    return token && spaceId ? { token, spaceId } : null
  })
  const { token, spaceId } = await handle.jsonValue()
  return { token, spaceId, apiBase, space: `${apiBase}/api/spaces/${encodeURIComponent(spaceId)}` }
}

export async function getJSON<T>(page: Page, path: string, session: Session): Promise<T> {
  const res = await page.request.get(path, {
    headers: { Authorization: `Bearer ${session.token}` },
  })
  expect(res.ok(), `GET ${path} → ${res.status()} ${await res.text()}`).toBeTruthy()
  return res.json() as Promise<T>
}

export async function postJSON<T>(page: Page, path: string, session: Session, body: unknown): Promise<T> {
  const res = await page.request.post(path, {
    headers: { Authorization: `Bearer ${session.token}` },
    data: body,
  })
  expect(res.ok(), `POST ${path} → ${res.status()} ${await res.text()}`).toBeTruthy()
  return res.json() as Promise<T>
}

export async function patchJSON<T>(page: Page, path: string, session: Session, body: unknown): Promise<T> {
  const res = await page.request.patch(path, {
    headers: { Authorization: `Bearer ${session.token}` },
    data: body,
  })
  expect(res.ok(), `PATCH ${path} → ${res.status()} ${await res.text()}`).toBeTruthy()
  return res.json() as Promise<T>
}

/** Upload one small text file to the space's storage. */
export async function uploadFile(page: Page, session: Session, name: string, content: string): Promise<void> {
  const res = await page.request.post(`${session.apiBase}/api/spaces/${encodeURIComponent(session.spaceId)}/upload`, {
    headers: { Authorization: `Bearer ${session.token}` },
    multipart: {
      files: { name, mimeType: "text/plain", buffer: Buffer.from(content) },
    },
  })
  expect(res.ok(), `upload ${name} → ${res.status()} ${await res.text()}`).toBeTruthy()
}

/**
 * Name what a run created and could not remove.
 *
 * The harness contract is to clean up, or to report an exact safe cleanup
 * target when the deployment has to keep the evidence. An attached deployment
 * keeps it: most of these resources have no delete route. Printing the ids is
 * what turns "the smoke account accumulates things" into a one-line cleanup.
 */
export function reportLeftovers(spaceId: string, resources: string[]): void {
  console.log(`[e2e] run ${RUN_ID} left in space ${spaceId}: ${resources.join(", ")}`)
}
