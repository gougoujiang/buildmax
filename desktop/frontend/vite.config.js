import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react()],
  root: '.',
  resolve: {
    alias: {
      // Resolve CSS subpath so Vite does not pass the bare specifier to the file system (avoids ENOENT)
      '@buildmax/gui/theme.css': path.resolve(__dirname, 'node_modules/@buildmax/gui/dist/theme.css'),
    },
  },
  build: {
    // Built one level up, into desktop/dist, rather than inside this directory.
    // desktop/assets_embed.go embeds it, and //go:embed cannot cross a module
    // boundary — this directory is its own Go module so that the node_modules
    // it installs stays out of the root module. See ../../portal/go.mod.
    outDir: '../dist',
    emptyOutDir: true,
  },
});
