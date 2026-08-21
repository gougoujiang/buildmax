import { expect, test, type Locator, type Page } from "@playwright/test"

/**
 * A run has to explain itself, and it has to be honest about what confined it.
 *
 * The roadmap is unusually specific here: "A run that is not sandboxed must say
 * so — in its trace and in Portal. An unreported boundary is worse than a
 * missing one." `RunTraceModal` is the only surface that makes that claim, and
 * nothing below the UI can check it. The API-level smoke never reaches it
 * either: the modal opens from an issue's outputs, and the smoke creates a
 * conversation task, which has none.
 *
 * So this spec creates the issue the smoke does not, and then reads the answer
 * the way an operator would.
 *
 * Seeding goes through the API rather than through four forms. What is under
 * test is whether Portal reports a run truthfully — not whether the issue form
 * submits — and driving the setup through the UI would spend the run's budget
 * on the parts already covered elsewhere, then fail for reasons that have
 * nothing to do with the boundary.
 */

// The worker has to start, run, and write its trace to storage. The API smoke
// allows two minutes for the same shape of work; this adds the polling that
// follows it.
const RUN_TIMEOUT_MS = 150_000

/**
 * Tag for everything this run creates.
 *
 * These specs attach to a deployment they do not own, and neither an issue nor
 * an agent can be deleted through the API, so what they create stays. A fixed
 * name would leave a deployment holding a dozen identical "Run trace probe"
 * issues with no way to tell which run left which — and a later spec that
 * looked one up by name would find whichever came first. The tag makes each
 * one attributable, and `reportLeftovers` says exactly what to remove.
 *
 * `./make e2e` supplies the id. A bare `npx playwright test` gets "local".
 */
const RUN_ID = process.env.BUILDMAX_E2E_RUN_ID ?? "local"

interface Session {
  token: string
  teamId: string
  /** Origin the API answers on, which is not always the one serving the Portal. */
  apiBase: string
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
 * its own port. Taking it from the app's own first call keeps this spec correct
 * on both without restating the precedence in `lib/api/client.ts`.
 */
async function session(page: Page): Promise<Session> {
  const teamsRequest = page.waitForRequest((req) => /\/api\/teams(\?|$)/.test(req.url()))
  await page.goto("/")
  const url = (await teamsRequest).url()
  const apiBase = url.slice(0, url.indexOf("/api/teams"))

  // The team is stored only after `GET /api/teams` answers, so this waits for
  // the app to settle rather than reading straight after navigation.
  const handle = await page.waitForFunction(() => {
    const token = localStorage.getItem("buildmax_token")
    const teamId = localStorage.getItem("buildmax_current_team")
    return token && teamId ? { token, teamId } : null
  })
  const stored = (await handle.jsonValue()) as { token: string; teamId: string }
  return { ...stored, apiBase }
}

async function postJSON<T>(page: Page, path: string, session: Session, body: unknown): Promise<T> {
  const res = await page.request.post(path, {
    headers: { Authorization: `Bearer ${session.token}` },
    data: body,
  })
  expect(res.ok(), `POST ${path} → ${res.status()} ${await res.text()}`).toBeTruthy()
  return res.json() as Promise<T>
}

async function patchJSON<T>(page: Page, path: string, session: Session, body: unknown): Promise<T> {
  const res = await page.request.patch(path, {
    headers: { Authorization: `Bearer ${session.token}` },
    data: body,
  })
  expect(res.ok(), `PATCH ${path} → ${res.status()} ${await res.text()}`).toBeTruthy()
  return res.json() as Promise<T>
}

/** The value cell of one run-trace statistic, addressed by its exact term. */
function statValue(page: Page, term: string): Locator {
  const rows = page.locator(".run-trace__stats > div")
  return rows.filter({ has: page.locator("dt", { hasText: new RegExp(`^${term}$`) }) }).locator("dd")
}

/**
 * Name what this run created and could not remove.
 *
 * The harness contract is to clean up, or to report an exact safe cleanup
 * target when the deployment has to keep the evidence. This deployment keeps
 * it: there is no delete route for either resource. Printing the ids is what
 * turns "the smoke account accumulates things" into a one-line cleanup.
 */
function reportLeftovers(teamId: string, resources: string[]): void {
  console.log(`[e2e] run ${RUN_ID} left in team ${teamId}: ${resources.join(", ")}`)
}

/** Seed an issue whose agent has run to completion, and return its issue id. */
async function seedCompletedAgentRun(page: Page, session: Session): Promise<string> {
  const team = `${session.apiBase}/api/teams/${encodeURIComponent(session.teamId)}`

  const agent = await postJSON<{ id: string }>(page, `${team}/agents`, session, {
    name: `Run trace probe ${RUN_ID}`,
    description: "Created by the Portal browser tests.",
    instructions: "Reply with exactly: deployment smoke ok",
  })
  const issue = await postJSON<{ id: string }>(page, `${team}/issues`, session, {
    title: `Run trace probe ${RUN_ID}`,
    description: "Created by the Portal browser tests to exercise the run trace view.",
  })
  reportLeftovers(session.teamId, [`agent ${agent.id}`, `issue ${issue.id}`])
  // An agent run is refused unless the issue is assigned to an agent, so this
  // is a precondition of the next call rather than a separate assertion.
  await patchJSON(page, `${team}/issues/${encodeURIComponent(issue.id)}`, session, {
    assignee_kind: "agent",
    assignee_id: agent.id,
  })
  const task = await postJSON<{ id: string }>(
    page,
    `${team}/issues/${encodeURIComponent(issue.id)}/agent-runs`,
    session,
    { input: "Reply with exactly: deployment smoke ok" }
  )

  // Poll the task rather than the UI. A run that never finishes should fail
  // here, naming the status it got stuck in, instead of thirty screenshots
  // later as a missing button.
  await expect
    .poll(
      async () => {
        const res = await page.request.get(`${team}/tasks/${encodeURIComponent(task.id)}`, {
          headers: { Authorization: `Bearer ${session.token}` },
        })
        if (!res.ok()) return `HTTP ${res.status()}`
        const body = (await res.json()) as { status: string; error_message?: string | null }
        return body.status === "FAILED" ? `FAILED: ${body.error_message ?? "no message"}` : body.status
      },
      { timeout: RUN_TIMEOUT_MS, intervals: [1000] }
    )
    .toBe("SUCCEEDED")

  return issue.id
}

test("Portal states what confined a run, and what the run spent", async ({ page }) => {
  test.setTimeout(RUN_TIMEOUT_MS + 60_000)

  const current = await session(page)
  const issueId = await seedCompletedAgentRun(page, current)

  await page.goto(`/#/issue/${issueId}`)

  // The output card is what carries the run id, so its button is the way in —
  // the same way an operator reaches it.
  //
  // Scoping to the card is load-bearing, not decorative. An agent run also
  // posts a comment, and that comment carries its own "Run details" button
  // (issue-discussion__actions), rendered earlier in the document. An unscoped
  // role query would match both and resolve to the comment's.
  await page
    .locator(".issue-outputs__card-actions")
    .getByRole("button", { name: "Run details" })
    .first()
    .click()

  const dialog = page.getByRole("dialog", { name: "Run details" })
  await expect(dialog).toBeVisible()

  const boundary = dialog.locator(".run-trace__boundary")
  const failure = dialog.locator(".modal__error")

  // The modal fetches the trace when it opens, so wait for it to settle into
  // one of its two outcomes before judging which one it is. Asserting "no
  // error" while the request is still in flight would pass on an empty modal.
  await expect(boundary.or(failure).first()).toBeVisible()

  // A trace the server never recorded, or one storage lost, lands here. Either
  // is a real failure of the gate: a run nobody can read cannot explain itself.
  await expect(failure).toHaveCount(0)
  // The assertion the roadmap actually demands. `--unknown` is the class the
  // Portal uses for "this trace predates boundary recording", and a fresh
  // deployment producing it means the boundary stopped being recorded — the
  // unreported boundary the roadmap calls worse than a missing one.
  //
  // It deliberately does not assert `--open`. Workers are unsandboxed today,
  // but hardening them is planned P0.5 work, and this test should keep passing
  // the day it lands rather than failing as though something broke.
  await expect(boundary).not.toHaveClass(/run-trace__boundary--unknown/)

  // What the run spent. The mock model in the smoke deployments reports both a
  // model name and token usage, so "—" here means the trace lost them rather
  // than that the deployment never had them.
  //
  // Rows are matched by their exact term because "Model" is a prefix of "Model
  // calls", and a prefix match would quietly assert against the wrong number.
  await expect(statValue(page, "Model")).not.toHaveText("—")
  await expect(statValue(page, "Duration")).not.toHaveText("—")
  await expect(statValue(page, "Tokens")).toHaveText(/\d+ in · \d+ out/)

  // The governance ledger, beside the trace. The smoke deployments run in
  // direct mode, so this section is expected to be empty here — what is under
  // test is that an empty one says which kind of empty it is. A blank section
  // would read as "this run spent nothing", which is the one thing an empty
  // ledger does not mean.
  const spend = dialog.locator(".run-trace__section").filter({ hasText: "Managed model calls" })
  await expect(spend).toBeVisible()
  await expect(spend.locator(".run-trace__spend-note, .run-trace__calls").first()).toBeVisible()
})
