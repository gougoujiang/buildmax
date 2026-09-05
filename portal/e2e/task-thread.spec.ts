import { expect, test, type Page } from "@playwright/test"

import { getJSON, postJSON, reportLeftovers, session, tagged, type Session } from "./fixtures"

/**
 * Direct Agent execution is a Portal-only claim: it creates a Task and its
 * first TaskRun without a Conversation in between, and the Task page is the
 * durable thread a Continue or a Retry both land on. Nothing below the UI
 * proves that a person can actually reach it this way — the API smoke drives
 * the same routes directly, and a handler test never renders a page.
 *
 * The mock model answers every call with the same fixed sentence regardless
 * of what it is asked (see internal/testsupport/mockllm), so this spec asserts
 * output content only to prove a turn produced one at all — what tells
 * Continue and Retry apart is the recorded input and lineage, asserted
 * against the API, not the model's reply.
 *
 * See docs/design/agent-execution-and-task-threads.md §4.1, §6, and §14.
 */

// The worker has to start, run, and finish each turn. The API smoke allows two
// minutes for the same shape of work; this adds the polling that follows it.
const RUN_TIMEOUT_MS = 150_000
const REPLY = "deployment smoke ok"

interface TaskStatus {
  status: string
  error_message?: string | null
}

/** Wait for a task's current run to reach a terminal status, polling the API rather than the UI. */
async function waitForTaskSucceeded(page: Page, current: Session, taskId: string): Promise<void> {
  await expect
    .poll(
      async () => {
        const task = await getJSON<TaskStatus>(page, `${current.team}/tasks/${encodeURIComponent(taskId)}`, current)
        return task.status === "FAILED" ? `FAILED: ${task.error_message ?? "no message"}` : task.status
      },
      { timeout: RUN_TIMEOUT_MS, intervals: [1000] }
    )
    .toBe("SUCCEEDED")
}

test("running an Agent directly reaches a Task with no Conversation, and Continue/Retry keep it one thread", async ({
  page,
}) => {
  test.setTimeout(RUN_TIMEOUT_MS * 3 + 60_000)

  const current = await session(page)
  const agentName = tagged("Task thread probe")
  const agent = await postJSON<{ id: string }>(page, `${current.team}/agents`, current, {
    name: agentName,
    description: "Created by the Portal browser tests.",
    instructions: `Reply with exactly: ${REPLY}`,
  })
  reportLeftovers(current.teamId, [`agent ${agent.id}`])

  await page.goto("/#/agents")
  await expect(page.getByRole("heading", { name: "Agents", exact: true })).toBeVisible()

  // The Run button is a typed request, not a detour through a Conversation: it
  // carries the agent id the user already picked and the input alone, with no
  // agent description or instructions folded into it. See §11.1.
  await page.getByRole("button", { name: `Run ${agentName}` }).click()
  const modal = page.getByRole("dialog", { name: `Run ${agentName}` })
  await expect(modal).toBeVisible()
  await modal.getByLabel("Task").fill("First turn")
  await modal.getByRole("button", { name: "Start" }).click()

  // Submitting navigates straight to the Task page. No intermediate
  // Conversation page is ever reached.
  await page.waitForURL(/#\/task\//, { timeout: 15_000 })
  const taskId = decodeURIComponent(page.url().split("/task/")[1] ?? "")
  expect(taskId).not.toBe("")
  reportLeftovers(current.teamId, [`task ${taskId}`])

  const task = await getJSON<{ conversation_id?: string; agent_id?: string }>(
    page,
    `${current.team}/tasks/${encodeURIComponent(taskId)}`,
    current
  )
  expect(task.conversation_id ?? "").toBe("")
  expect(task.agent_id).toBe(agent.id)

  const history = page.getByLabel("Task conversation")
  await expect(history.locator(".bm-chat-thread__row--user")).toHaveCount(1)

  await waitForTaskSucceeded(page, current, taskId)
  await expect(history.locator(".bm-chat-thread__row--assistant").last()).toContainText(REPLY)

  // --- Continue: a new input on the same Task, a new TaskRun. ---
  const followUp = "Second turn, asked as a Continue"
  await page.getByRole("textbox", { name: "Continue task" }).fill(followUp)
  await page.getByRole("button", { name: "Continue" }).click()
  await expect(history.locator(".bm-chat-thread__row--user")).toHaveCount(2)

  const afterContinue = await getJSON<{ runs: { id: string; input: string }[] }>(
    page,
    `${current.team}/tasks/${encodeURIComponent(taskId)}/runs`,
    current
  )
  expect(afterContinue.runs).toHaveLength(2)
  expect(afterContinue.runs[1].input).toBe(followUp)

  await waitForTaskSucceeded(page, current, taskId)
  await expect(history.locator(".bm-chat-thread__row--assistant").last()).toContainText(REPLY)

  // --- Retry: repeats the run just continued with, not the task's first
  // input. That is what tells it apart from Continue in the data, and the UI
  // must show a third turn either way. ---
  await page.getByRole("button", { name: "Retry last run" }).click()
  await expect(history.locator(".bm-chat-thread__row--user")).toHaveCount(3)

  const afterRetry = await getJSON<{ runs: { id: string; input: string; retry_of_task_run_id?: string | null }[] }>(
    page,
    `${current.team}/tasks/${encodeURIComponent(taskId)}/runs`,
    current
  )
  expect(afterRetry.runs).toHaveLength(3)
  expect(afterRetry.runs[2].input).toBe(followUp)
  expect(afterRetry.runs[2].retry_of_task_run_id).toBe(afterRetry.runs[1].id)

  await waitForTaskSucceeded(page, current, taskId)
  await expect(history.locator(".bm-chat-thread__row--assistant").last()).toContainText(REPLY)
})
