<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import LivePosePreview from './LivePosePreview.svelte';
  import VisionTeacher from './VisionTeacher.svelte';

  const labels = ['empty', 'stand', 'walk', 'sit', 'lie', 'gesture', 'multi', 'pose'];

  let recordings = $state<Array<Record<string, unknown>>>([]);
  let label = $state('pose');
  let sessionName = $state('');
  let busy = $state(false);
  let recording = $state(false);
  let recordingId = $state<string | null>(null);
  let visionOn = $state(true);
  let error = $state<string | null>(null);

  function normalizeList(raw: unknown): Array<Record<string, unknown>> {
    if (Array.isArray(raw)) return raw as Array<Record<string, unknown>>;
    if (raw && typeof raw === 'object' && Array.isArray((raw as { recordings?: unknown }).recordings)) {
      return (raw as { recordings: Array<Record<string, unknown>> }).recordings;
    }
    return [];
  }

  async function refresh() {
    try {
      const raw = await api.listRecordings();
      recordings = normalizeList(raw);
      error = null;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function start() {
    busy = true;
    error = null;
    try {
      const session_name = sessionName.trim() || `${label}_${Date.now()}`;
      const j = await api.startRecording({ session_name, label });
      recording = true;
      recordingId = String(j.recording_id || '');
      sessionName = session_name;
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function stop() {
    busy = true;
    try {
      await api.stopRecording();
      recording = false;
      recordingId = null;
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function del(id: string) {
    busy = true;
    try {
      await api.deleteRecording(id);
      await refresh();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(refresh, 3000);
    return () => clearInterval(id);
  });
</script>

<div style="margin-bottom:1rem;">
  <LivePosePreview />
</div>

<div class="card" style="margin-bottom:1rem;">
  <h2>CSI + vision teacher</h2>
  <p class="empty" style="margin-bottom:0.75rem;">
    CSI → <code class="mono">.csi.jsonl</code>. With vision on, MediaPipe writes
    <code class="mono">.pose.jsonl</code> for real limb supervision (prefer Wizard for the full loop).
  </p>
  <div class="row" style="margin-bottom:0.75rem;">
    <label>
      <span class="metric-label">Activity</span><br />
      <select bind:value={label}>
        {#each labels as l}
          <option value={l}>{l}</option>
        {/each}
      </select>
    </label>
    <label>
      <span class="metric-label">Session name</span><br />
      <input bind:value={sessionName} placeholder="wave_desk" style="min-width:14rem;" />
    </label>
    <label class="row" style="align-items:flex-end;">
      <input type="checkbox" bind:checked={visionOn} />
      <span class="metric-label">Vision teacher</span>
    </label>
  </div>
  <div class="row">
    <button class="btn btn-danger" disabled={busy || recording} onclick={start}>Start</button>
    <button class="btn" disabled={busy || !recording} onclick={stop}>Stop</button>
    {#if recording}
      <span class="chip bad">RECORDING</span>
      {#if recordingId}
        <span class="chip muted mono">{recordingId}</span>
      {/if}
    {/if}
  </div>
  {#if error}
    <p class="empty" style="color:var(--bad);margin-top:0.75rem;">{error}</p>
  {/if}
</div>

{#if visionOn}
  <div class="card" style="margin-bottom:1rem;">
    <h2>MediaPipe teacher</h2>
    <VisionTeacher recordingId={recording ? recordingId : null} />
  </div>
{/if}

<div class="card">
  <h2>Recordings ({recordings.length})</h2>
  {#if recordings.length === 0}
    <p class="empty">No recordings in data/recordings yet.</p>
  {:else}
    <table>
      <thead>
        <tr>
          <th>ID</th>
          <th>Session / label</th>
          <th>Pose</th>
          <th>Meta</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each recordings as r}
          {@const id = String(r.id ?? '')}
          {@const session = String(r.session_name ?? r.name ?? id)}
          {@const lab = String(r.label ?? '')}
          <tr>
            <td class="mono">{id}</td>
            <td>
              <span class="mono">{session}</span>
              {#if lab}
                <span class="chip muted" style="margin-left:0.35rem;">{lab}</span>
              {/if}
            </td>
            <td>
              {#if r.has_pose}
                <span class="chip ok">{String(r.teacher || 'pose')}</span>
                <span class="muted" style="font-size:0.72rem;">{String(r.pose_frames ?? '')}</span>
              {:else}
                <span class="chip muted">no pose</span>
              {/if}
            </td>
            <td class="mono" style="color:var(--muted);font-size:0.75rem;">
              {r.frames != null ? `${r.frames} frames` : ''}
              {r.size_bytes != null ? ` · ${r.size_bytes} B` : ''}
              {r.format === 'legacy' ? ' · legacy' : ''}
            </td>
            <td>
              <button class="btn" disabled={busy || !id} onclick={() => del(id)}>Delete</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>
