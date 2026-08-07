<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';

  /** Spectre Record activity → adaptive class (server also maps these). */
  const LABEL_MAP: Array<{ activity: string; klass: string; tip: string }> = [
    { activity: 'empty', klass: 'absent', tip: 'empty room' },
    { activity: 'stand / sit / lie / pose', klass: 'present_still', tip: 'person still' },
    { activity: 'walk / gesture', klass: 'present_moving', tip: 'person moving' },
    { activity: 'multi', klass: 'active', tip: 'high motion / multi' },
  ];

  let recordings = $state<Array<Record<string, unknown>>>([]);
  let selected = $state<string[]>([]);
  let status = $state<Record<string, unknown> | null>(null);
  let busy = $state(false);
  let error = $state<string | null>(null);
  let lastResult = $state<Record<string, unknown> | null>(null);

  function normalizeRecs(raw: unknown): Array<Record<string, unknown>> {
    if (Array.isArray(raw)) return raw as Array<Record<string, unknown>>;
    if (raw && typeof raw === 'object' && Array.isArray((raw as { recordings?: unknown }).recordings)) {
      return (raw as { recordings: Array<Record<string, unknown>> }).recordings;
    }
    return [];
  }

  async function refresh() {
    try {
      const [r, s] = await Promise.all([api.listRecordings(), api.adaptiveStatus()]);
      recordings = normalizeRecs(r);
      status = s;
      error = null;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function toggle(id: string) {
    if (selected.includes(id)) selected = selected.filter((x) => x !== id);
    else selected = [...selected, id];
  }

  function selectLabeled() {
    selected = recordings
      .filter((r) => String(r.label ?? '').trim().length > 0)
      .map((r) => String(r.id ?? ''))
      .filter(Boolean);
  }

  async function train() {
    busy = true;
    error = null;
    lastResult = null;
    try {
      const body = selected.length ? { dataset_ids: selected } : {};
      const res = await api.adaptiveTrain(body);
      lastResult = res;
      if (!res.success) {
        error = String(res.error || 'Adaptive train failed');
      }
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function unload() {
    busy = true;
    try {
      await api.adaptiveUnload();
      lastResult = null;
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(refresh, 4000);
    return () => clearInterval(id);
  });
</script>

<div class="card" style="margin-top:1.25rem;">
  <h2>Presence / motion (adaptive)</h2>
  <p class="empty">
    Room-specific classifier that overrides live presence + motion_level after train. Separate from
    DensePose limbs. Needs labeled Record sessions — ideally
    <strong>empty</strong>, <strong>stand/sit</strong>, and <strong>walk</strong>.
  </p>

  <div class="row" style="margin:0.75rem 0;flex-wrap:wrap;">
    {#each LABEL_MAP as row}
      <span class="chip muted" title={row.tip}>{row.activity} → {row.klass}</span>
    {/each}
  </div>

  <div class="row" style="margin-bottom:0.85rem;">
    {#if status?.loaded}
      <span class="chip ok">adaptive loaded</span>
      <span class="chip muted">
        frames {String(status.trained_frames ?? '—')} · acc{' '}
        {status.accuracy != null ? `${(Number(status.accuracy) * 100).toFixed(0)}%` : '—'}
      </span>
      {#if Array.isArray(status.classes)}
        <span class="chip muted">classes {(status.classes as string[]).join(', ')}</span>
      {/if}
    {:else}
      <span class="chip warn">adaptive off (heuristic presence)</span>
    {/if}
  </div>

  <div class="row" style="margin-bottom:0.75rem;">
    <button class="btn" disabled={busy} onclick={selectLabeled}>Select labeled</button>
    <button
      class="btn btn-primary"
      disabled={busy}
      onclick={train}
      title={selected.length ? `Train on ${selected.length} selected` : 'Train on all labeled recordings'}
    >
      {selected.length ? `Train adaptive (${selected.length})` : 'Train adaptive (all labeled)'}
    </button>
    <button class="btn" disabled={busy || !status?.loaded} onclick={unload}>Unload adaptive</button>
  </div>

  {#if recordings.length === 0}
    <p class="empty">No recordings yet — capture labeled sessions on Record first.</p>
  {:else}
    <div style="display:flex;flex-direction:column;gap:0.35rem;max-height:14rem;overflow:auto;">
      {#each recordings as r}
        {@const id = String(r.id ?? '')}
        {@const label = String(r.label ?? '')}
        {@const name = String(r.session_name ?? r.name ?? id)}
        <label class="row" style="justify-content:flex-start;">
          <input
            type="checkbox"
            checked={selected.includes(id)}
            onchange={() => toggle(id)}
            disabled={!id}
          />
          <span class="mono">{id}</span>
          <span class="muted" style="font-size:0.78rem;">
            {name}{label ? ` · ${label}` : ' · unlabeled'} · {r.frames ?? '—'} frames
          </span>
        </label>
      {/each}
    </div>
  {/if}

  {#if lastResult?.success}
    <p class="empty" style="margin-top:0.75rem;color:var(--ok);">
      Trained {String(lastResult.trained_frames)} frames · accuracy{' '}
      {lastResult.accuracy != null ? `${(Number(lastResult.accuracy) * 100).toFixed(1)}%` : '—'}
      · hot-loaded (overrides presence/motion now).
    </p>
  {/if}
  {#if error}
    <p class="empty" style="color:var(--bad);margin-top:0.75rem;">{error}</p>
  {/if}
</div>
