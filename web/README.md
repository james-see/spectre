# Spectre operator UI

Astro + Svelte console for the local sensing-server. No RuView demoware tabs.

## Dev

```bash
# terminal 1 — sensing engine
cd ~/p/spectre && ./scripts/run_local.sh

# terminal 2 — UI (proxies /api and /ws)
cd ~/p/spectre/web && npm install && npm run dev
open http://127.0.0.1:4321/
```

Routes: `/` status · `/observatory` · `/sensing` · `/pose` · `/record` · `/train`

- **Observatory** — room / AP router / Wi‑Fi waves / signal field / ESP32 markers (live WS only, no demo scenarios)
- **Pose** — limbs only when keypoint confidence ≥ 0.2
