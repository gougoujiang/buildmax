import { defineConfig } from "vitest/config"
import react from "@vitejs/plugin-react"

export default defineConfig({
  plugins: [react()],
  // These components are the shared surface Portal and Desktop both render, so
  // what is worth testing here is behaviour in a document — which key does
  // what, what a viewer can see — rather than the return value of a function.
  // That needs a DOM, which is what jsdom is for.
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
  build: {
    lib: {
      entry: "src/index.ts",
      formats: ["es"],
    },
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      external: ["react", "react-dom", "react/jsx-runtime"],
      output: {
        entryFileNames: "index.js",
      },
    },
  },
})
