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
cd ../gui
npm install
npm run build
```

2. Install Portal dependencies

```bash
npm install
```

## Development

Start the dev server (default: http://localhost:5173):

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

If the local `file:../gui` dependency is not built yet, build `../gui` first.

## Preview production build

After `npm run build`, serve the built files locally:

```bash
npm run preview
```
