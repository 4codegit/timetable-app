import { defineConfig, Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Wails serves the frontend from its own asset server, which does NOT emit
// CORS headers. Vite's default production output marks the module <script> and
// <link> tags with `crossorigin`, which makes the browser refuse to load them
// (CORS failure) and leaves a blank window. Strip that attribute. Relative base
// paths also avoid any absolute-path mismatches with the Wails asset server.
function stripCrossOrigin(): Plugin {
  return {
    name: 'wails-strip-crossorigin',
    transformIndexHtml(html) {
      return html.replace(/\s+crossorigin(?:="[^"]*")?/g, '')
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  base: './',
  plugins: [svelte(), stripCrossOrigin()],
})
