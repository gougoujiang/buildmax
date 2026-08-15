# BuildMax Portal

Web UI for BuildMax. Built with React, TypeScript, and Vite.

**URL scheme**: The app uses hash-based routing. See [URL scheme](docs/url-scheme.md) for route patterns and examples.

## Current Role

The Portal is the main browser UI for the team-scoped BuildMax product.

It currently integrates with the Go backend for:

- auth and team resolution
- conversations and live chat turns
- issues and issue flow visibility
- workflows and workflow runs
- agents and team settings
- team-scoped file browsing and upload

The Portal also depends on the shared `@buildmax/gui` package at the repo root for common theme and presentational widgets.

## Setup

Recommended local flow from the repo root:

```bash
./make run portal
```

This will build `gui` when needed and start the Portal dev server.

If you want to run pieces manually:

1. Build the shared GUI package

```bash
cd gui
npm ci
npm run build
```

1. Install Portal dependencies

```bash
cd ../portal
npm ci
```

The repository pins Node 22 in `.node-version` and npm 10 in `package.json`.

## Development

Start the dev server (default: <http://localhost:5173>):

```bash
npm run dev
```

The Portal expects the Go server to be running separately, usually via:

```bash
./make run server
```

## Build

Produce a static build in `dist/`:

```bash
npm run build
```

Run `npm run lint` and `npm test` before submitting Portal changes; lint is a
zero-warning gate.

If the local `file:../gui` dependency is not built yet, build `../gui` first.

## Preview production build

After `npm run build`, serve the built files locally:

```bash
npm run preview
```

## Where The API URL Comes From

`getApiBase()` in `src/lib/api/client.ts` reads, in order:

1. `window.__BUILDMAX_CONFIG__.apiBase` — written into `config.js` at container
   start from `BUILDMAX_API_BASE`. `public/config.js` is the empty default that
   ships in the bundle.
2. `VITE_API_BASE` — build time, for `npm run dev` and hand-built bundles.
3. `http://localhost:5678` — where a local `buildmax-server` listens.

The runtime step is what lets one published image serve any deployment. When
changing this, keep `src/lib/api/client.test.ts` honest: a broken precedence is
invisible in dev and total in a container.

Container image: `deployment/docker/Dockerfile.portal`, published as
`ghcr.io/gougoujiang/buildmax-portal`. Running it is documented in
[deploy/overview.md](../docs/deploy/overview.md#portal).
