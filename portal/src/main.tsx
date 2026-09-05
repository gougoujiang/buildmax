import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ThemeProvider } from '@buildmax/gui'
import App from './App'
import { SharedArtifact } from './pages/shared/SharedArtifact'
import './index.css'

// A public share link is a real path, /shared/artifacts/<token>, not a hash
// route: it opens for someone with no login, so it renders its own minimal,
// themed page rather than the authenticated app shell. Everything else is the
// app.
function rootElement() {
  const match = window.location.pathname.match(/^\/shared\/artifacts\/([^/]+)\/?$/)
  if (match) {
    return (
      <ThemeProvider>
        <SharedArtifact token={decodeURIComponent(match[1])} />
      </ThemeProvider>
    )
  }
  return <App />
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>{rootElement()}</StrictMode>,
)
