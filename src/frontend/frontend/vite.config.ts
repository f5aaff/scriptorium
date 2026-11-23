import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  // PDF.js uses top-level await; supported browser/WebView builds must be modern.
  build: { target: 'es2022' }
})
