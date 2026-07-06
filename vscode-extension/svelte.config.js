import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

export default {
  // Enables lang="ts" in <script> blocks.
  preprocess: vitePreprocess(),
};
