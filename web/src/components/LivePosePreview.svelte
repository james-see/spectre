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
      draw();
    } catch {
      connected = false;
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(refresh, 250);
    return () => clearInterval(id);
  });
</script>

<div class="card live-pose">
  <div class="row" style="margin-bottom:0.5rem;justify-content:space-between;">
    <h2 style="margin:0;">Live DensePose</h2>
    <div class="row">
      <SourceChip source={pose?.source} {connected} />
      <span class="chip muted">{pose?.pose_source ?? '—'}</span>
      <span class={`chip ${showSkeleton ? 'ok' : 'warn'}`}>kp {maxConf.toFixed(2)}</span>
    </div>
  </div>
  <canvas bind:this={canvasEl} width="480" height="360" class="pose-canvas"></canvas>
  <p class="empty hint">
    {#if showSkeleton}
      Model-space skeleton (auto-fit) — not camera-registered. Pretrained conf can look high while
      geometry is schematic until you fine-tune on this room.
    {:else}
      Waiting for usable model keypoints (conf ≥ {POSE_CONF_THRESHOLD}). Presence + loaded RVF
      required; no heuristic stick figures.
    {/if}
  </p>
</div>

<style>
  .pose-canvas {
    width: 100%;
    height: auto;
    display: block;
    border-radius: 6px;
  }
  .hint {
    margin-top: 0.55rem;
    font-size: 0.75rem;
  }
</style>
