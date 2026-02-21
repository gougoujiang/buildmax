import { useState } from "react"
import { navigate } from "../router"
import { createTask } from "../lib/api"
import { PromptArea } from "../components/PromptArea"

interface NewChatProps {
  workspaceId: string
  token?: string
  onRefetchWorkspaceTasks?: () => void
}

export function NewChat({
  workspaceId,
  token,
  onRefetchWorkspaceTasks,
}: NewChatProps) {
  const [prompt, setPrompt] = useState("")
  const [running, setRunning] = useState(false)
  const [runError, setRunError] = useState<string | null>(null)

  async function handleRun() {
    const input = prompt.trim()
    if (!input || !token || running) return
    setRunning(true)
    setRunError(null)
    try {
      const task = await createTask(workspaceId, { input }, token)
      setPrompt("")
      onRefetchWorkspaceTasks?.()
      navigate({ name: "task", workspaceId, taskId: task.id })
    } catch (err) {
      setRunError(err instanceof Error ? err.message : "Failed to start chat")
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="page-new-chat">
      <h1 className="page-new-chat__title">New Chat</h1>
      <p className="page-new-chat__subtitle">
        Start a new conversation. Describe what you want to accomplish and the agent will work on it.
      </p>
      <PromptArea
        value={prompt}
        onChange={(v) => { setPrompt(v); setRunError(null) }}
        onRun={handleRun}
        heading="What would you like to do?"
        placeholder="e.g. Help me analyze last month's sales data"
      />
      {runError && (
        <p className="page-new-chat__error" role="alert">
          {runError}
        </p>
      )}
    </div>
  )
}
