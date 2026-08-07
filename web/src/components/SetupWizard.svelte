<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import VisionTeacher from './VisionTeacher.svelte';
  import LivePosePreview from './LivePosePreview.svelte';

  const steps = [
    'Nodes',
    'Base models',
    'Camera labels',
    'Train',
    'Validate',
  ] as const;

  let step = $state(0);
  let nodesOnline = $state(0);
  let presence = $state(false);
  let models = $state<Array<Record<string, unknown>>>([]);
  let active = $state<Record<string, unknown> | null>(null);
  let recordings = $state<Array<Record<string, unknown>>>([]);
  let status = $state<Record<string, unknown> | null>(null);
  let decoClients = $state(0);
  let error = $state<string | null>(null);
  let busy = $state(false);
  let recordingId = $state<string | null>(null);
  let recordLabel = $state('pose');
  let selectedRoom = $state<string[]>([]);
  let epochs = $state(40);

  const mmfiRecs = $derived(
    recordings.filter((r) => String(r.teacher) === 'mmfi' || String(r.id).startsWith('mmfi_')),
  );
  const roomRecs = $derived(
    recordings.filter(
      (r) => r.has_pose && String(r.teacher) !== 'mmfi' && !String(r.id).startsWith('mmfi_'),
    ),
  );
  const bestBaseId = $derived.by(() => {
    const mmfiModel = models.find((m) => String(m.id).includes('mmfi'));
    if (mmfiModel) return String(mmfiModel.id);
    const pre = models.find((m) => String(m.id) === 'wifi-densepose-pretrained');
    return pre ? String(pre.id) : models[0] ? String(models[0].id) : '';
  });

  function normalizeRecs(raw: unknown): Array<Record<string, unknown>> {
    if (Array.isArray(raw)) return raw as Array<Record<string, unknown>>;
    if (raw && typeof raw === 'object' && Array.isArray((raw as { recordings?: unknown }).recordings)) {
      return (raw as { recordings: Array<Record<string, unknown>> }).recordings;
    }
    return [];
  }

  async function refresh() {
    try {
      const [s, m, a, r, t, lan] = await Promise.all([
        api.sensingLatest().catch(() => null),
        api.models(),
        api.activeModel(),
        api.listRecordings(),
        api.trainStatus().catch(() => null),
        api.lanClients().catch(() => ({ clients: [] })),
      ]);
      nodesOnline = (s?.nodes || []).length;
      presence = !!s?.classification?.presence;
      models = m.models || [];
      active = a.active;
      recordings = normalizeRecs(r);
      status = t;
      decoClients = (lan.clients || []).length;
      error = null;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function startRoomRecord() {
    busy = true;
    error = null;
    try {
      const j = await api.startRecording({
        session_name: `room_${recordLabel}_${Date.now()}`,
        label: recordLabel,
      });
      recordingId = String(j.recording_id || '');
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function stopRoomRecord() {
    busy = true;
    try {
      await api.stopRecording();
      recordingId = null;
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  function toggle(list: string[], id: string, set: (v: string[]) => void) {
    set(list.includes(id) ? list.filter((x) => x !== id) : [...list, id]);
  }

  async function runTrain(dataset_ids: string[], base: string | null, tag: string) {
    busy = true;
    error = null;
    try {
      await api.trainStart({
        dataset_ids,
        config: {
          epochs,
          batch_size: 32,
          learning_rate: base ? 1e-4 : 3e-4,
          weight_decay: 1e-4,
          early_stopping_patience: 15,
          warmup_epochs: base ? 2 : 5,
          pretrained_rvf: base,
          allow_heuristic_targets: false,
        },
      });
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : `${tag}: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function loadBest() {
    const id = bestBaseId;
    if (!id) return;
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

  async function createFixture() {
    busy = true;
    error = null;
    try {
      await api.createMmfiFixture();
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(refresh, 1500);
    return () => clearInterval(id);
  });
</script>

<div class="wizard">
  <div class="steps">
    {#each steps as label, i}
      <button
        type="button"
        class="step"
        class:active={step === i}
        class:done={i < step}
        onclick={() => (step = i)}>{i + 1}. {label}</button
      >
    {/each}
  </div>

  {#if error}
    <p class="empty" style="color:var(--bad)">{error}</p>
  {/if}

  {#if step === 0}
    <div class="card">
      <h2>1 · ESP32 triad + hotspot</h2>
      <p class="empty">
        Place three ESP32-S3 nodes in the room on SSID Patience (or your mesh). Deco clients are
        context only — limbs need CSI + vision labels.
      </p>
      <div class="row" style="margin:0.75rem 0;">
        <span class={`chip ${nodesOnline >= 3 ? 'ok' : 'warn'}`}>CSI nodes {nodesOnline}/3</span>
        <span class={`chip ${presence ? 'ok' : 'muted'}`}>presence {presence ? 'yes' : 'no'}</span>
        <span class="chip muted">Deco clients {decoClients}</span>
      </div>
      <button class="btn btn-primary" disabled={nodesOnline < 1} onclick={() => (step = 1)}
        >Continue</button
      >
    </div>
  {:else if step === 1}
    <div class="card">
      <h2>2 · Public / pretrained base</h2>
      <p class="empty">
        <strong>Goal:</strong> have pose-labeled public sessions + a loaded base RVF, then move on.
        You do <em>not</em> need the shell script if the buttons below succeed.
      </p>
      <ol class="howto">
        <li>Ensure pose data exists (button A, or you already have mmfi sessions).</li>
        <li>Optional: Pretrain on those sessions (button B) — skip if a prior run already completed.</li>
        <li>Load best base (button C), then Continue to camera labeling.</li>
      </ol>
      <div class="row" style="margin:0.75rem 0;">
        <span class={`chip ${mmfiRecs.length > 0 ? 'ok' : 'warn'}`}>mmfi sessions {mmfiRecs.length}</span>
        <span class="chip muted">models {models.length}</span>
        <span class={`chip ${active ? 'ok' : 'muted'}`}>active {String(active?.id ?? '—')}</span>
      </div>
      {#if mmfiRecs.length > 0 && active}
        <p class="chip ok" style="display:inline-block;margin-bottom:0.75rem;">
          Ready for this step — Continue to camera labels (step 3).
        </p>
      {/if}
      <div class="row" style="gap:0.5rem;flex-wrap:wrap;">
        <button class="btn" disabled={busy} onclick={createFixture}
          >A · Add MM-Fi fixture data</button
        >
        <button
          class="btn"
          disabled={busy || mmfiRecs.length === 0 || !!status?.active}
          onclick={() =>
            runTrain(
              mmfiRecs.slice(0, 32).map((r) => String(r.id)),
              'wifi-densepose-pretrained',
              'mmfi-pretrain',
            )}>B · Pretrain on MM-Fi</button
        >
        <button class="btn" disabled={busy || !bestBaseId} onclick={loadBest}
          >C · Load best base</button
        >
        <button class="btn btn-primary" onclick={() => (step = 2)}>Continue → camera</button>
      </div>
      {#if status && (status.phase !== 'idle' || Number(status.epoch) > 0)}
        <p class="mono last-train" style="margin-top:0.75rem;">
          Last train job (not this step’s checklist): {String(status.phase)} · epoch
          {String(status.epoch)}/{String(status.total_epochs || '—')}
          {#if status.matched_pose_frames != null}
            · pose {String(status.matched_pose_frames)}
          {/if}
          {#if status.output_model}
            · wrote {String(status.output_model)}
          {/if}
        </p>
      {/if}
      <details class="adv">
        <summary>Advanced: import full MM-Fi from disk</summary>
        <p class="empty">
          After downloading MM-Fi (large), run once in the Spectre repo. Optional — fixture is enough
          to proceed; full MM-Fi improves the geometry prior.
        </p>
        <pre class="cmd">python3 scripts/import_mmfi.py --root ~/data/MMFi --env E01 --max-sequences 16</pre>
      </details>
    </div>
  {:else if step === 2}
    <div class="card">
      <h2>3 · Camera teacher + CSI record</h2>
      <p class="empty">
        Start recording, move (wave, sit, walk). MediaPipe uploads pose labels synced to CSI.
      </p>
      <div class="row" style="margin-bottom:0.75rem;">
        <label>
          <span class="metric-label">Label</span><br />
          <select bind:value={recordLabel}>
            {#each ['pose', 'sit', 'stand', 'walk', 'gesture'] as l}
              <option value={l}>{l}</option>
            {/each}
          </select>
        </label>
        <button class="btn btn-danger" disabled={busy || !!recordingId} onclick={startRoomRecord}
          >Start CSI+pose</button
        >
        <button class="btn" disabled={busy || !recordingId} onclick={stopRoomRecord}>Stop</button>
      </div>
      <VisionTeacher recordingId={recordingId} />
      <p class="empty" style="margin-top:0.75rem;">
        Room sessions with pose: {roomRecs.length}
      </p>
      <button class="btn btn-primary" style="margin-top:0.5rem;" onclick={() => (step = 3)}
        >Continue</button
      >
    </div>
  {:else if step === 3}
    <div class="card">
      <h2>4 · Fine-tune loop</h2>
      <p class="empty">
        Select room sessions (has_pose). Warm-start from MM-Fi / pretrained RVF. Repeat record→train
        until Pose looks right.
      </p>
      <label>
        <span class="metric-label">Epochs</span><br />
        <input type="number" bind:value={epochs} min="5" max="200" style="width:6rem;" />
      </label>
      <div style="margin:0.75rem 0;display:flex;flex-direction:column;gap:0.35rem;">
        {#each roomRecs as r}
          {@const id = String(r.id)}
          <label class="row" style="justify-content:flex-start;">
            <input
              type="checkbox"
              checked={selectedRoom.includes(id)}
              onchange={() => toggle(selectedRoom, id, (v) => (selectedRoom = v))}
            />
            <span class="mono">{id}</span>
            <span class="chip muted">{String(r.teacher || 'mediapipe')}</span>
            <span class="muted" style="font-size:0.75rem;">{String(r.pose_frames ?? 0)} pose</span>
          </label>
        {/each}
        {#if roomRecs.length === 0}
          <p class="empty">No room pose sessions yet — go back to step 3.</p>
        {/if}
      </div>
      <div class="row">
        <button
          class="btn btn-primary"
          disabled={busy || selectedRoom.length === 0 || !!status?.active}
          onclick={() => runTrain(selectedRoom, bestBaseId || null, 'room-ft')}
          >Fine-tune from base</button
        >
        <button class="btn" disabled={busy || !status?.active} onclick={() => api.trainStop()}
          >Stop</button
        >
        <button class="btn" onclick={() => (step = 4)}>Continue</button>
      </div>
      {#if status}
        <p class="mono" style="margin-top:0.75rem;font-size:0.8rem;">
          {status.active ? 'RUNNING' : String(status.phase)} · loss
          {Number(status.train_loss ?? 0).toFixed(2)}
          {#if status.output_model}
            · wrote {String(status.output_model)}
          {/if}
        </p>
      {/if}
    </div>
  {:else}
    <div class="card">
      <h2>5 · Validate</h2>
      <p class="empty">
        Load the new RVF (Train page or Model menu), then check CSI-only limbs below. Camera is off —
        this is what the app should run on.
      </p>
      <LivePosePreview />
      <div class="row" style="margin-top:0.75rem;">
        <a class="btn btn-primary" href="/pose">Open Pose</a>
        <a class="btn" href="/train">Open Train</a>
        <button class="btn" onclick={() => (step = 2)}>Record more</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .wizard {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .steps {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .step {
    appearance: none;
    border: 1px solid var(--border);
    background: rgba(12, 15, 20, 0.55);
    color: var(--muted);
    font: inherit;
    font-size: 0.78rem;
    padding: 0.35rem 0.65rem;
    border-radius: 999px;
    cursor: pointer;
  }
  .step.active {
    color: var(--ok);
    border-color: rgba(62, 207, 142, 0.4);
  }
  .step.done {
    color: var(--text);
  }
  .cmd {
    background: #0a0e14;
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.75rem;
    font-size: 0.75rem;
    overflow-x: auto;
    color: var(--muted);
  }
  .howto {
    margin: 0.5rem 0 0.75rem 1.1rem;
    color: var(--muted);
    font-size: 0.85rem;
    line-height: 1.45;
  }
  .last-train {
    font-size: 0.75rem;
    color: var(--muted);
  }
  .adv {
    margin-top: 1rem;
    color: var(--muted);
    font-size: 0.8rem;
  }
  .adv summary {
    cursor: pointer;
    color: var(--text);
  }
</style>
