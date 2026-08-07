<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';

  let open = $state(false);
  let models = $state<Array<{ id: string; name?: string; size_bytes?: number }>>([]);
  let activeId = $state<string | null>(null);
  let busy = $state(false);
  let error = $state<string | null>(null);
  let rootEl: HTMLDivElement;

  const PREFERRED = 'wifi-densepose-pretrained';

  function modelId(m: Record<string, unknown> | { id: string }) {
    return String((m as { id?: unknown }).id ?? (m as { name?: unknown }).name ?? '');
  }

  async function refresh() {
    try {
      const [m, a] = await Promise.all([api.models(), api.activeModel()]);
      models = (m.models || [])
        .map((row) => {
          const id = modelId(row);
          return {
            id,
            name: String(row.name ?? id),
            size_bytes: typeof row.size_bytes === 'number' ? row.size_bytes : undefined,
          };
        })
        .filter((row) => row.id);
      activeId = a.active ? modelId(a.active) : null;
      error = null;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
      models = [];
      activeId = null;
    }
  }

  async function load(id: string) {
    if (!id || busy) return;
    busy = true;
    error = null;
    try {
      await api.loadModel(id);
      await refresh();
      open = false;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function unload() {
    if (busy) return;
    busy = true;
    error = null;
    try {
      await api.unloadModel();
      await refresh();
      open = false;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  function label() {
    if (activeId) {
      const short =
        activeId.length > 22 ? `${activeId.slice(0, 10)}…${activeId.slice(-8)}` : activeId;
      return short;
    }
    return 'Model';
  }

  onMount(() => {
    refresh();
    const poll = setInterval(refresh, 5000);
    const onDoc = (ev: MouseEvent) => {
      if (!open) return;
      if (rootEl && !rootEl.contains(ev.target as Node)) open = false;
    };
    document.addEventListener('pointerdown', onDoc);
    return () => {
      clearInterval(poll);
      document.removeEventListener('pointerdown', onDoc);
    };
  });
</script>

<div class="model-menu" bind:this={rootEl}>
  <button
    type="button"
    class={`model-trigger chip ${activeId ? 'ok' : 'muted'}`}
    aria-haspopup="menu"
    aria-expanded={open}
    disabled={busy}
    onclick={() => {
      open = !open;
      if (open) refresh();
    }}
    title={activeId ? `Active: ${activeId}` : 'Load DensePose RVF model'}
  >
    {#if busy}
      …
    {:else}
      {label()}
      <span class="caret" aria-hidden="true">▾</span>
    {/if}
  </button>

  {#if open}
    <div class="model-dropdown" role="menu">
      <div class="model-head">DensePose model</div>
      {#if error}
        <p class="model-err">{error}</p>
      {/if}
      {#if models.length === 0}
        <p class="model-empty">No .rvf under models/</p>
      {:else}
        <ul>
          {#each models as m}
            <li>
              <button
                type="button"
                class="model-item"
                role="menuitem"
                disabled={busy || m.id === activeId}
                onclick={() => load(m.id)}
              >
                <span class="mono name">{m.id}</span>
                {#if m.id === activeId}
                  <span class="chip ok">active</span>
                {:else if m.id === PREFERRED}
                  <span class="chip muted">pretrained</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
      <div class="model-actions">
        {#if models.some((m) => m.id === PREFERRED) && activeId !== PREFERRED}
          <button
            type="button"
            class="btn btn-primary"
            disabled={busy}
            onclick={() => load(PREFERRED)}
          >
            Load pretrained
          </button>
        {/if}
        <button type="button" class="btn" disabled={busy || !activeId} onclick={unload}>
          Unload
        </button>
        <a class="btn" href="/train" onclick={() => (open = false)}>Train…</a>
      </div>
    </div>
  {/if}
</div>

<style>
  .model-menu {
    position: relative;
    margin-left: auto;
  }
  .model-trigger {
    cursor: pointer;
    border: 1px solid var(--border);
    background: transparent;
    max-width: 16rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .model-trigger:disabled {
    opacity: 0.6;
  }
  .caret {
    margin-left: 0.2rem;
    opacity: 0.7;
  }
  .model-dropdown {
    position: absolute;
    right: 0;
    top: calc(100% + 0.35rem);
    z-index: 40;
    min-width: 17rem;
    max-width: min(22rem, 92vw);
    padding: 0.65rem;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: rgba(12, 15, 20, 0.96);
    backdrop-filter: blur(8px);
    box-shadow: 0 10px 28px rgba(0, 0, 0, 0.45);
  }
  .model-head {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--muted);
    font-weight: 600;
    margin-bottom: 0.45rem;
  }
  .model-err {
    margin: 0 0 0.5rem;
    color: var(--bad);
    font-size: 0.78rem;
  }
  .model-empty {
    margin: 0;
    color: var(--muted);
    font-size: 0.8rem;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 14rem;
    overflow: auto;
  }
  .model-item {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    text-align: left;
    padding: 0.45rem 0.4rem;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text);
    cursor: pointer;
    font: inherit;
  }
  .model-item:hover:not(:disabled) {
    background: rgba(61, 156, 240, 0.12);
  }
  .model-item:disabled {
    opacity: 0.85;
    cursor: default;
  }
  .name {
    font-size: 0.78rem;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .model-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin-top: 0.55rem;
    padding-top: 0.55rem;
    border-top: 1px solid var(--border);
  }
  .model-actions :global(.btn) {
    font-size: 0.78rem;
    padding: 0.3rem 0.55rem;
  }
</style>
