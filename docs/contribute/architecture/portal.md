# Portal

## Purpose

The Portal is the team collaboration surface under `portal/`. It is a React 19
+ Vite + TypeScript app that talks to the Go server over HTTP and WebSocket.

Portal owns the cloud/team lane:

- login/signup
- team/space switching and settings
- conversations
- issues
- workflows and workflow runs
- agents
- team files
- usage and webhook keys

## Current Shape

- Routes are in `portal/src/router.ts`.
- Pages live under `portal/src/pages/*`.
- API calls live under `portal/src/features/*/api.ts` and `portal/src/lib/api`.
- Shared presentation components come from `@buildmax/gui`.
- Conversation streaming uses `portal/src/contexts/WebSocketContext.tsx`.

## Product Boundary

Portal is the cloud team workspace. CLI and Desktop are the local execution
lane. Desktop may bridge to Portal later, but Portal remains the place for
team administration, issue/workflow management, team files, governance, and
cloud results.
