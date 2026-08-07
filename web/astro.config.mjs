// @ts-check
import { defineConfig } from 'astro/config';
import svelte from '@astrojs/svelte';

const SENSING_HTTP = process.env.SPECTRE_API ?? 'http://127.0.0.1:3000';
const SENSING_WS = process.env.SPECTRE_WS ?? 'http://127.0.0.1:3001';
const DECO_HTTP = process.env.SPECTRE_DECO ?? 'http://127.0.0.1:3002';

/** Shared Vite proxy — must be present for both `astro dev` and `astro preview`. */
const proxy = {
  // Longer prefix first — otherwise /api steals /api/lan → sensing :3000 (404).
  '/api/lan': { target: DECO_HTTP, changeOrigin: true },
  '/api': { target: SENSING_HTTP, changeOrigin: true },
  '/health': { target: SENSING_HTTP, changeOrigin: true },
  '/oauth': { target: SENSING_HTTP, changeOrigin: true },
  '/ws': {
    target: SENSING_WS,
    changeOrigin: true,
    ws: true,
  },
};

/** @type {import('astro').AstroUserConfig} */
export default defineConfig({
  integrations: [svelte()],
  server: {
    port: 4321,
    host: true,
  },
  vite: {
    server: { proxy },
    preview: { proxy },
    // Chart.js (and sometimes three) can miss the Rolldown/Vite dep cache → 504 on
    // /node_modules/.vite/deps/chart__js.js and a dead Sensing island. Force-include.
    optimizeDeps: {
      include: ['chart.js', 'three', 'three/addons/controls/OrbitControls.js'],
    },
  },
});
