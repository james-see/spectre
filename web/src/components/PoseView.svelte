<script lang="ts">
  import { onMount } from 'svelte';
  import {
    fetchDrawablePose,
    maxKeypointConfidence,
    POSE_CONF_THRESHOLD,
    type PoseCurrent,
  } from '../lib/api';
  import { drawPoseSkeleton } from '../lib/poseDraw';
  import SourceChip from './SourceChip.svelte';

  let pose = $state<PoseCurrent | null>(null);
  let connected = $state(false);
  let error = $state<string | null>(null);
  let canvasEl: HTMLCanvasElement;

  const maxConf = $derived(maxKeypointConfidence(pose?.persons));
  const showSkeleton = $derived(
    maxConf >= POSE_CONF_THRESHOLD &&
      !!pose?.persons?.length &&
      (pose?.pose_source === 'model_inference' || pose?.pose_source === 'densepose'),
  );

  function draw() {
    const canvas = canvasEl;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.strokeStyle = '#2a3441';
    const ok = showSkeleton
      ? drawPoseSkeleton(ctx, pose?.persons || [], {
          fillStyle: '#141a22',
          bothEnds: true,
        })
      : false;
    if (!ok) {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      ctx.fillStyle = '#141a22';
      ctx.fillRect(0, 0, canvas.width, canvas.height);
    }
    ctx.strokeStyle = '#2a3441';
    ctx.lineWidth = 1;
    ctx.strokeRect(0.5, 0.5, canvas.width - 1, canvas.height - 1);
  }

  async function refresh() {
    try {
      pose = await fetchDrawablePose();
      connected = true;
      error = null;
      draw();
    } catch (e) {
      connected = false;
      error = e instanceof Error ? e.message : String(e);
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(refresh, 200);
    return () => clearInterval(id);
  });
</script>

<div class="row" style="margin-bottom: 1rem;">
  <SourceChip source={pose?.source} {connected} />
  <span class="chip muted">pose_source {pose?.pose_source ?? '—'}</span>
  <span class={`chip ${showSkeleton ? 'ok' : 'warn'}`}>
    max kp conf {maxConf.toFixed(2)} (need ≥ {POSE_CONF_THRESHOLD})
  </span>
  <span class="chip muted">persons {pose?.persons?.length ?? 0}</span>
</div>

{#if error}
  <div class="card empty">{error}</div>
{/if}

<div class="grid grid-2">
  <div class="card">
    <h2>Skeleton</h2>
    <canvas
      bind:this={canvasEl}
      width="640"
      height="480"
      style="width:100%;height:auto;border-radius:6px;"
    ></canvas>
    {#if showSkeleton}
      <p class="empty" style="margin-top:0.75rem;">
        Drawing model-space skeleton (auto-fit). Pretrained RVF conf can be high while joint XYZ is
        still schematic for this room — fine-tune via Record → Train for better limbs.
      </p>
    {:else}
      <p class="empty" style="margin-top:0.75rem;">
        No drawable keypoints yet (need presence + loaded RVF + conf ≥ {POSE_CONF_THRESHOLD}). No
        heuristic stick figures. Check ModelMenu has <code class="mono">wifi-densepose-pretrained</code>
        loaded.
      </p>
    {/if}
  </div>
  <div class="card">
    <h2>Diagnostics</h2>
    <p class="empty">
      Trust rule: limbs render only when both bone endpoints clear confidence ≥
      {POSE_CONF_THRESHOLD}. Uses pose/current, with sensing <code class="mono">pose_keypoints</code>
      fallback if tracker stripped joint conf.
    </p>
    <ul class="mono" style="color:var(--muted);font-size:0.8rem;line-height:1.6;">
      <li>total_persons field: {pose?.total_persons ?? '—'}</li>
      <li>drawn: {showSkeleton ? 'yes' : 'no'}</li>
      <li>threshold: {POSE_CONF_THRESHOLD}</li>
    </ul>
  </div>
</div>
