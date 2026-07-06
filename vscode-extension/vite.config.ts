import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'path';

// Builds the Svelte webview apps. Each view kind is its own entry, producing
// media/<name>.js + media/<name>.css that the extension host references via
// asWebviewUri (see src/webview/html.ts). One entry per view keeps bundles
// tiny and free of other views' dead code — adding a view = adding one line.
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'media',
    emptyOutDir: true,
    // One shared stylesheet: per-entry CSS splitting would attach shared
    // component styles to JS chunks, which the host's <link> tag never loads.
    // Svelte's scoped class hashes make a combined file collision-free.
    cssCodeSplit: false,
    // Webviews load assets from vscode-webview:// URIs — sourcemaps and
    // hashed names only complicate that. Stable names, no hashes.
    rollupOptions: {
      input: {
        track: resolve(__dirname, 'webviews/entries/track.ts'),
        bus: resolve(__dirname, 'webviews/entries/bus.ts'),
        master: resolve(__dirname, 'webviews/entries/master.ts'),
        project: resolve(__dirname, 'webviews/entries/project.ts'),
        mixer: resolve(__dirname, 'webviews/entries/mixer.ts'),
        transport: resolve(__dirname, 'webviews/entries/transport.ts'),
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: 'chunks/[name].js',
        assetFileNames: '[name][extname]',
      },
    },
  },
});
