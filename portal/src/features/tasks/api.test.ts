import { afterEach, describe, expect, it, vi } from "vitest"
import { cancelTask, retryTask } from "./api"
import { taskIsRetryable, taskIsStoppable } from "../../lib/taskStatus"

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

describe("cancelTask", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("posts to the task's cancel route with the caller's token", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(202, { task_id: "t1", task_run_id: "r1", status: "RUNNING", cancel_requested: true }),
    )
    vi.stubGlobal("fetch", fetchMock)

    const got = await cancelTask("tm 1", "t1", "token-123")

    expect(got.cancel_requested).toBe(true)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain("/api/teams/tm%201/tasks/t1/cancel")
    expect(init.method).toBe("POST")
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer token-123")
  })

  // The run can finish between the page drawing a Stop button and the click
  // arriving. The message has to say that rather than read as a failure to stop.
  it("surfaces the server's reason when there is nothing to cancel", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonResponse(409, { error: "this run has already finished" })),
    )

    await expect(cancelTask("tm1", "t1", "token")).rejects.toThrow("this run has already finished")
  })
})

describe("retryTask", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("posts to the task's retry route and reports which run it repeats", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(201, {
        task_id: "t1",
        task_run_id: "r2",
        retry_of_task_run_id: "r1",
        status: "PENDING",
      }),
    )
    vi.stubGlobal("fetch", fetchMock)

    const got = await retryTask("tm 1", "t1", "token-123")

    expect(got.retry_of_task_run_id).toBe("r1")
    expect(got.task_run_id).toBe("r2")
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toContain("/api/teams/tm%201/tasks/t1/retry")
    expect(init.method).toBe("POST")
    expect(init.body).toBeUndefined()
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer token-123")
  })

  // 409 means three different things - a run in flight, nothing finished to
  // repeat, a workflow step - so the server's own sentence is what reaches the
  // page rather than one guess covering all three.
  it("surfaces the server's reason for refusing", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(409, { error: "this run belongs to a workflow step and cannot be retried on its own" }),
      ),
    )

    await expect(retryTask("tm1", "t1", "token")).rejects.toThrow("workflow step")
  })
})

describe("taskIsRetryable", () => {
  it("covers every finished run, including one that succeeded", () => {
    expect(taskIsRetryable("success")).toBe(true)
    expect(taskIsRetryable("failed")).toBe(true)
    expect(taskIsRetryable("canceled")).toBe(true)
  })

  it("leaves work in flight to the Stop button", () => {
    expect(taskIsRetryable("pending")).toBe(false)
    expect(taskIsRetryable("running")).toBe(false)
  })
})

describe("taskIsStoppable", () => {
  it("covers work in flight, including a run still waiting for a worker", () => {
    expect(taskIsStoppable("pending")).toBe(true)
    expect(taskIsStoppable("running")).toBe(true)
  })

  it("leaves finished work alone", () => {
    expect(taskIsStoppable("success")).toBe(false)
    expect(taskIsStoppable("failed")).toBe(false)
    expect(taskIsStoppable("canceled")).toBe(false)
  })
})
