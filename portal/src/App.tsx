import { useState } from "react"
import { Header } from "./Header"
import { PromptArea } from "./PromptArea"
import { RecentActivity } from "./RecentActivity"
import { MOCK_ACTIVITY } from "./mockActivity"

function App() {
  const [prompt, setPrompt] = useState("")

  function handleRun() {
    // No-op: no API call in this task
    if (prompt.trim()) {
      console.log("Run (no-op):", prompt.trim())
    }
  }

  return (
    <main className="app">
      <Header workspaceName="Sales Team" />
      <PromptArea value={prompt} onChange={setPrompt} onRun={handleRun} />
      <RecentActivity items={MOCK_ACTIVITY} />
    </main>
  )
}

export default App
