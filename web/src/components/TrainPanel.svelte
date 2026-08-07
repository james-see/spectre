<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import LivePosePreview from './LivePosePreview.svelte';

  let models = $state<Array<Record<string, unknown>>>([]);
  let active = $state<Record<string, unknown> | null>(null);
  let recordings = $state<Array<Record<string, unknown>>>([]);
  let selected = $state<string[]>([]);
  let status = $state<Record<string, unknown> | null>(null);
  let epochs = $state(50);
  let baseModel = $state('wifi-densepose-pretrained');
  let busy = $state(false);
  let error = $state<string | null>(null);
  let lastStart = $state<string | null>(null);

  const activeTrain = $derived(!!status?.active);
  const phase = $derived(String(status?.phase ?? 'idle'));
  const epoch = $derived(Number(status?.epoch ?? 0));
  const totalEpochs = $derived(Number(status?.total_epochs ?? 0));
  const progressPct = $derived(
    totalEpochs > 0 ? Math.min(100, Math.round((epoch / totalEpochs) * 100)) : activeTrain ? 2 : 0,
  );
  const phaseTone = $derived(
    phase.includes('fail') || phase === 'cancelled'
      ? 'bad'
      : activeTrain
        ? 'ok'
        : phase === 'completed' || phase === 'early_stopped'
          ? 'ok'
          : 'muted',
  );

  function normalizeRecs(raw: unknown): Array<Record<string, unknown>> {
    if (Array.isArray(raw)) return raw as Array<Record<string, unknown>>;
    if (raw && typeof raw === 'object' && Array.isArray((raw as { recordings?: unknown }).recordings)) {
      return (raw as { recordings: Array<Record<string, unknown>> }).recordings;
    }
    return [];
  }

  function formatEta(secs: unknown): string {
    const n = Number(secs);
    if (!Number.isFinite(n) || n <= 0) return '—';
    if (n < 60) return `${Math.round(n)}s`;
    const m = Math.floor(n / 60);
    const s = Math.round(n % 60);
    return `${m}m ${s}s`;
  }

  async function refresh() {
    try {
      const [m, a, r, t] = await Promise.all([
        api.models(),
        api.activeModel(),
        api.listRecordings(),
        api.trainStatus().catch(() => null),
      ]);
      models = m.models || [];
      active = a.active;
      recordings = normalizeRecs(r);
      status = t;
      if (
        baseModel === 'wifi-densepose-pretrained' &&
        !models.some((x) => String(x.id) === baseModel) &&
        models[0]
      ) {
        /* keep default even if list lags */
      }
      error = null;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  function toggle(id: string) {
    if (selected.includes(id)) selected = selected.filter((x) => x !== id);
    else selected = [...selected, id];
  }

  async function load(id: string) {
    busy = true;
    try {
      await api.loadModel(id);
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
      await api.unloadModel();
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function startTrain() {
    busy = true;
    error = null;
    lastStart = null;
    try {
      const finetune = !!baseModel;
      const missingPose = selected.some((id) => {
        const r = recordings.find((x) => String(x.id) === id);
        return !r?.has_pose;
      });
      if (missingPose) {
        throw new Error(
          'Selected datasets need .pose.jsonl (use Wizard / Record with vision teacher, or import MM-Fi)',
        );
      }
      await api.trainStart({
        dataset_ids: selected,
        config: {
          epochs,
          batch_size: 32,
          learning_rate: finetune ? 1e-4 : 3e-4,
          weight_decay: 1e-4,
          early_stopping_patience: 20,
          warmup_epochs: finetune ? 2 : 5,
          pretrained_rvf: finetune ? baseModel : null,
          allow_heuristic_targets: false,
        },
      });
      lastStart = finetune ? `fine-tune from ${baseModel}` : 'train from scratch';
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function stopTrain() {
    busy = true;
    try {
      await api.trainStop();
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(() => {
      void refresh();
    }, 500);
    return () => clearInterval(id);
  });
</script>

<div style="margin-bottom:1rem;">
  <LivePosePreview />
</div>

<div class="grid grid-2" style="margin-bottom:1rem;">
  <div class="card">
    <h2>Active model</h2>
    {#if active}
      <div class="mono">{String(active.id ?? active.name ?? '—')}</div>
      <div class="metric-label" style="margin-top:0.35rem;">
        {String(active.path ?? active.format ?? '')}
      </div>
      <div class="row" style="margin-top:0.75rem;">
        <button class="btn" disabled={busy} onclick={unload}>Unload</button>
      </div>
    {:else}
      <p class="empty">No active model id.</p>
    {/if}
  </div>
  <div class="card">
    <h2>Train status</h2>
    <div class="row" style="margin-bottom:0.5rem;">
      <span class={`chip ${phaseTone}`}>
        {activeTrain ? 'RUNNING' : phase === 'idle' ? 'idle' : phase}
      </span>
      {#if status?.init_mode}
        <span class="chip muted">{String(status.init_mode)}</span>
      {/if}
      {#if status?.base_model}
        <span class="chip muted">base {String(status.base_model)}</span>
      {/if}
    </div>
    <div class="bar" aria-hidden="true">
      <div class="bar-fill" class:pulse={activeTrain} style={`width:${progressPct}%`}></div>
    </div>
    <div class="mono status-lines">
      <div>
        epoch {epoch}/{totalEpochs || '—'}
        {#if status?.train_loss != null}
          · loss {Number(status.train_loss).toFixed(2)}
        {/if}
      </div>
      <div>
        best pck {Number(status?.best_pck ?? 0).toFixed(3)}@{String(status?.best_epoch ?? 0)}
        · patience {String(status?.patience_remaining ?? '—')}
        · eta {formatEta(status?.eta_secs)}
      </div>
      {#if status?.output_model}
        <div>output <span class="ok-text">{String(status.output_model)}</span></div>
      {/if}
      {#if status?.message}
        <div class="muted-line">{String(status.message)}</div>
      {/if}
      {#if lastStart && activeTrain}
        <div class="muted-line">started: {lastStart}</div>
      {/if}
    </div>
  </div>
</div>

<div class="card" style="margin-bottom:1rem;">
  <h2>Models</h2>
  {#if models.length === 0}
    <p class="empty">No .rvf files under models/.</p>
  {:else}
    <table>
      <thead>
        <tr>
          <th>ID</th>
          <th>Size</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each models as m}
          {@const id = String(m.id ?? '')}
          <tr>
            <td class="mono">{id}</td>
            <td class="mono">{m.size_bytes ?? '—'}</td>
            <td>
              <button class="btn btn-primary" disabled={busy || !id} onclick={() => load(id)}>Load</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<div class="card">
  <h2>Train from recordings</h2>
  <p class="empty">
    Select CSI sessions, optionally warm-start from a base RVF, then start. Writes a new linear
    DensePose <code class="mono">.rvf</code> into <code class="mono">models/</code>. Fine-tune keeps
    pretrained weights when feature width matches; otherwise falls back to scratch (status shows
    <code class="mono">finetune_mismatch</code>).
  </p>
  <div class="row" style="margin:0.75rem 0;flex-wrap:wrap;">
    <label>
      <span class="metric-label">Base model</span><br />
      <select bind:value={baseModel} style="min-width:16rem;">
        <option value="">(from scratch)</option>
        {#each models as m}
          {@const id = String(m.id ?? '')}
          {#if id}
            <option value={id}>{id}</option>
          {/if}
        {/each}
      </select>
    </label>
    <label>
      <span class="metric-label">Epochs</span><br />
      <input type="number" bind:value={epochs} min="1" max="500" style="width:6rem;" />
    </label>
    <button
      class="btn btn-primary"
      disabled={busy || selected.length === 0 || activeTrain}
      onclick={startTrain}
      >{baseModel ? 'Start fine-tune' : 'Start train'}</button
    >
    <button class="btn" disabled={busy || !activeTrain} onclick={stopTrain}>Stop</button>
  </div>
  {#if recordings.length === 0}
    <p class="empty">No recordings — capture some on the Record page first.</p>
  {:else}
    <div style="display:flex;flex-direction:column;gap:0.35rem;">
      {#each recordings as r}
        {@const id = String(r.id ?? '')}
        {@const label = String(r.label ?? '')}
        {@const name = String(r.session_name ?? r.name ?? id)}
        {@const frames = r.frames != null ? String(r.frames) : '—'}
        <label class="row" style="justify-content:flex-start;">
          <input
            type="checkbox"
            checked={selected.includes(id)}
            onchange={() => toggle(id)}
            disabled={!id || activeTrain}
          />
          <span class="mono">{id}</span>
          {#if r.has_pose}
            <span class="chip ok">{String(r.teacher || 'pose')}</span>
          {:else}
            <span class="chip warn">no pose</span>
          {/if}
          <span class="muted" style="font-size:0.78rem;">
            {name}{label ? ` · ${label}` : ''} · {frames} frames{r.format === 'legacy' ? ' · legacy' : ''}
          </span>
        </label>
      {/each}
    </div>
  {/if}
  {#if error}
    <p class="empty" style="color:var(--bad);margin-top:0.75rem;">{error}</p>
  {/if}
</div>

<style>
  .bar {
    height: 6px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.06);
    overflow: hidden;
    margin-bottom: 0.55rem;
  }
  .bar-fill {
    height: 100%;
    background: linear-gradient(90deg, #3ecf8e, #3d9cf0);
    transition: width 0.35s ease;
  }
  .bar-fill.pulse {
    animation: pulse 1.2s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      filter: brightness(1.25);
    }
  }
  .status-lines {
    font-size: 0.8rem;
    line-height: 1.45;
    color: var(--text);
  }
  .muted-line {
    color: var(--muted);
  }
  .ok-text {
    color: var(--ok);
  }
</style>
