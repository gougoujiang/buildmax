# BuildMax Portal

Web UI for BuildMax. Built with React, TypeScript, and Vite.

**URL scheme**: The app uses hash-based routing. See [URL scheme](docs/url-scheme.md) for route patterns and examples.

**Data**: The app currently uses synchronous mock data from `src/data/`. When adding a real API, introduce a data layer (e.g. React Query + hooks or a small API module) and have App and pages consume that instead of calling `data/mockData` directly; a thin re-export in `src/data/index.ts` allows swapping the implementation in one place.

## Setup

```bash
npm install
```

## Development

Start the dev server (default: http://localhost:5173):

```bash
npm run dev
```

## Build

Produce a static build in `dist/`:

```bash
npm run build
```

## Preview production build

After `npm run build`, serve the built files locally:

```bash
npm run preview
```
