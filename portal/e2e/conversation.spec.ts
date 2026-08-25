import { expect, test } from "@playwright/test"

import { postJSON, reportLeftovers, session } from "./fixtures"

/**
 * A conversation turn never touches HTTP.
 *
 * `useConversationDetail` sends the message with `ws.send("conversation.message")`
 * and draws the reply from `conversation.message.delta` frames. Both halves ride
 * the socket the Portal opens at `GET /api/teams/{id}/ws`, which means the
 * published bundle's client, the deployment's handling of the upgrade, the
 * bearer token on the socket, and Tier 1's streaming all have to cooperate
 * before a single character appears.
 *
 * Nothing else proves that. The deployment smoke creates a conversation task and
 * polls `/tasks/{id}` over HTTP; the handler tests hold a socket in memory with
 * no proxy in front of it. A deployment whose edge does not forward the upgrade
 * passes every one of them and shows a person an empty thread.
 */

/** What the deterministic model answers, whatever it is asked. */
const REPLY = "deployment smoke ok"

test("a conversation turn crosses the deployment's WebSocket in both directions", async ({ page }) => {
  // Registered before anything navigates: `session()` loads the app, and the
  // socket opens with it.
  const sent: string[] = []
  const received: string[] = []
  page.on("websocket", (ws) => {
    ws.on("framesent", (frame) => {
      if (typeof frame.payload === "string") sent.push(frame.payload)
    })
    ws.on("framereceived", (frame) => {
      if (typeof frame.payload === "string") received.push(frame.payload)
    })
  })

  const current = await session(page)
  const conversation = await postJSON<{ conversation_id: string }>(
    page,
    `${current.team}/conversations`,
    current,
    { channel: "portal" }
  )
  reportLeftovers(current.teamId, [`conversation ${conversation.conversation_id}`])

  await page.goto(`/#/conversation/${conversation.conversation_id}`)
  // By role: the section around the composer is labelled "Send a message", and
  // a label match alone finds both it and the box inside it.
  const composer = page.getByRole("textbox", { name: "Message" })
  await expect(composer).toBeVisible()

  // A prompt that does not contain the answer. The model replies the same
  // sentence whatever it is asked, so asking for it verbatim would put the
  // string in the thread twice and leave the assertion unable to say which one
  // the deployment produced.
  await composer.fill("How is this deployment doing?")
  await composer.press("Enter")

  // The assertion a person would make: the answer is on screen, in the thread.
  const history = page.getByLabel("Conversation history")
  await expect(history.getByText(REPLY, { exact: true })).toBeVisible({ timeout: 20_000 })

  // And the assertion that says how it got there. Without these two the test
  // would still pass against a deployment that fell back to polling, which is
  // the failure this spec exists to catch.
  const frameFor = (frames: string[], type: string) =>
    frames.some((frame) => frame.includes(`"${type}"`) && frame.includes(conversation.conversation_id))
  expect(frameFor(sent, "conversation.message"), `frames sent: ${sent.length}`).toBeTruthy()
  expect(
    frameFor(received, "conversation.message.delta"),
    `no delta frame among ${received.length} received`
  ).toBeTruthy()
})
