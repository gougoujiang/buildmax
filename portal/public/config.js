// Runtime configuration, read by src/lib/api/client.ts before any request.
//
// This default is empty on purpose: `npm run dev` and a hand-built bundle fall
// through to VITE_API_BASE. The container image overwrites this file at start
// from BUILDMAX_API_BASE, which is how one published image serves deployments
// whose API URL was unknowable when the bundle was built.
window.__BUILDMAX_CONFIG__ = window.__BUILDMAX_CONFIG__ || {}
