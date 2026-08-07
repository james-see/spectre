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

  const CAM_KEY = 'spectre.camera.deviceId';

  let pose = $state<PoseCurrent | null>(null);
  let connected = $state(false);
  let error = $state<string | null>(null);
  let camError = $state<string | null>(null);
  let camOn = $state(false);
  let streaming = $state(false);
  let devices = $state<MediaDeviceInfo[]>([]);
  let deviceId = $state('');

  let stageEl: HTMLDivElement;
  let videoEl: HTMLVideoElement;
  let canvasEl: HTMLCanvasElement;
  let stream: MediaStream | null = null;
  let raf = 0;
  let poll: ReturnType<typeof setInterval> | undefined;

  const maxConf = $derived(maxKeypointConfidence(pose?.persons));
  const poseSource = $derived(pose?.pose_source ?? null);
  const showSkeleton = $derived(
    maxConf >= POSE_CONF_THRESHOLD &&
      !!pose?.persons?.length &&
      (poseSource === 'model_inference' || poseSource === 'densepose'),
  );
  const selectedLabel = $derived(
    devices.find((d) => d.deviceId === deviceId)?.label ||
      (deviceId ? 'Selected camera' : 'Default camera'),
  );

  async function refreshDevices() {
    if (!navigator.mediaDevices?.enumerateDevices) {
      devices = [];
      return;
    }
    const all = await navigator.mediaDevices.enumerateDevices();
    devices = all.filter((d) => d.kind === 'videoinput');
    if (!deviceId) {
      const saved = localStorage.getItem(CAM_KEY) || '';
      if (saved && devices.some((d) => d.deviceId === saved)) deviceId = saved;
      else if (devices[0]) deviceId = devices[0].deviceId;
    } else if (devices.length && !devices.some((d) => d.deviceId === deviceId)) {
      deviceId = devices[0]?.deviceId || '';
    }
  }

  function videoConstraints(): MediaTrackConstraints {
    const base: MediaTrackConstraints = {
      width: { ideal: 1280 },
      height: { ideal: 720 },
    };
    if (deviceId) return { ...base, deviceId: { exact: deviceId } };
    return { ...base, facingMode: 'user' };
  }

  async function startCamera() {
    camError = null;
    try {
      // Labels often empty until permission — brief open then enumerate.
      if (!devices.length || devices.every((d) => !d.label)) {
        const probe = await navigator.mediaDevices.getUserMedia({ video: true, audio: false });
        probe.getTracks().forEach((t) => t.stop());
        await refreshDevices();
      }
      stream?.getTracks().forEach((t) => t.stop());
      stream = await navigator.mediaDevices.getUserMedia({
        video: videoConstraints(),
        audio: false,
      });
      const track = stream.getVideoTracks()[0];
      const liveId = track?.getSettings()?.deviceId;
      if (liveId) {
        deviceId = liveId;
        localStorage.setItem(CAM_KEY, liveId);
      }
      videoEl.srcObject = stream;
      await videoEl.play();
      camOn = true;
      streaming = true;
      await refreshDevices();
      syncCanvas();
    } catch (e) {
      camError = e instanceof Error ? e.message : String(e);
      camOn = false;
      streaming = false;
    }
  }

  function stopCamera() {
    stream?.getTracks().forEach((t) => t.stop());
    stream = null;
    if (videoEl) videoEl.srcObject = null;
    camOn = false;
    streaming = false;
    paint();
  }

  async function onDeviceChange() {
    if (deviceId) localStorage.setItem(CAM_KEY, deviceId);
    else localStorage.removeItem(CAM_KEY);
    if (camOn) await startCamera();
  }

  function syncCanvas() {
    if (!stageEl || !canvasEl) return;
    const w = stageEl.clientWidth;
    const h = stageEl.clientHeight;
    if (w < 2 || h < 2) return;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvasEl.width = Math.round(w * dpr);
    canvasEl.height = Math.round(h * dpr);
    canvasEl.style.width = `${w}px`;
    canvasEl.style.height = `${h}px`;
    const ctx = canvasEl.getContext('2d');
    if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  function paint() {
    const canvas = canvasEl;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const cssW = stageEl?.clientWidth || canvas.width;
    const cssH = stageEl?.clientHeight || canvas.height;

    // Logical size after setTransform(dpr)
    ctx.clearRect(0, 0, cssW, cssH);

    if (!showSkeleton) {
      if (!camOn) {
        ctx.fillStyle = '#141a22';
        ctx.fillRect(0, 0, cssW, cssH);
      }
      return;
    }

    // Soft vignette plate so limbs read over busy video
    if (camOn) {
      ctx.fillStyle = 'rgba(8, 12, 18, 0.18)';
      ctx.fillRect(0, 0, cssW, cssH);
    } else {
      ctx.fillStyle = '#141a22';
      ctx.fillRect(0, 0, cssW, cssH);
    }

    drawPoseSkeleton(ctx, pose?.persons || [], {
      clear: false,
      destW: cssW,
      destH: cssH,
      pad: Math.round(Math.min(cssW, cssH) * 0.12),
      bothEnds: true,
      lineWidth: 3,
      jointRadius: 5,
    });
  }

  async function refreshPose() {
    try {
      pose = await fetchDrawablePose();
      connected = true;
      error = null;
    } catch (e) {
      connected = false;
      error = e instanceof Error ? e.message : String(e);
    }
  }

  onMount(() => {
    const saved = localStorage.getItem(CAM_KEY);
    if (saved) deviceId = saved;
    refreshDevices().catch(() => {});
    const onDeviceList = () => {
      refreshDevices().catch(() => {});
    };
    navigator.mediaDevices?.addEventListener?.('devicechange', onDeviceList);

    const loop = () => {
      raf = requestAnimationFrame(loop);
      if (camOn) syncCanvas();
      paint();
    };
    syncCanvas();
    loop();
    refreshPose();
    poll = setInterval(refreshPose, 200);
    const ro = new ResizeObserver(() => {
      syncCanvas();
      paint();
    });
    if (stageEl) ro.observe(stageEl);

    return () => {
      cancelAnimationFrame(raf);
      if (poll) clearInterval(poll);
      ro.disconnect();
      navigator.mediaDevices?.removeEventListener?.('devicechange', onDeviceList);
      stopCamera();
    };
  });
</script>

<div class="row cam-bar" style="margin-bottom: 1rem;">
  <SourceChip source={pose?.source} {connected} />
  <span class="chip muted">pose_source {poseSource ?? '—'}</span>
  <span class={`chip ${showSkeleton ? 'ok' : 'warn'}`}>
    max kp {maxConf.toFixed(2)} ≥ {POSE_CONF_THRESHOLD}
  </span>
  <span class="chip muted">persons {pose?.persons?.length ?? 0}</span>
  <span class={`chip ${camOn ? 'ok' : 'muted'}`}>{camOn ? 'camera on' : 'camera off'}</span>
  <label class="cam-select">
    <span class="cam-select-label">Camera</span>
    <select bind:value={deviceId} onchange={onDeviceChange}>
      {#if !devices.length}
        <option value="">Default / grant access…</option>
      {:else}
        {#each devices as d, i (d.deviceId)}
          <option value={d.deviceId}>{d.label || `Camera ${i + 1}`}</option>
        {/each}
      {/if}
    </select>
  </label>
  {#if !camOn}
    <button type="button" class="btn btn-primary" onclick={startCamera}>Enable camera</button>
  {:else}
    <button type="button" class="btn" onclick={stopCamera}>Stop camera</button>
  {/if}
</div>

<p class="muted note">
  Webcam stays local (never uploaded). Limbs come only from sensing-server DensePose when
  keypoint confidence clears the gate — not from a client-side video CNN or invented stick figures.
  Skeleton is auto-fit into the frame (CSI model space); it is not camera-calibrated yet.
</p>

{#if error}
  <div class="card empty">{error}</div>
{/if}
{#if camError}
  <div class="card empty">{camError}</div>
{/if}

<div class="stage-wrap">
  <div class="stage" bind:this={stageEl}>
    <video
      bind:this={videoEl}
      class:hidden={!camOn}
      autoplay
      playsinline
      muted
    ></video>
    {#if !camOn}
      <div class="stage-placeholder">
        <p>
          Camera off — pick a source above, then enable. Skeleton still draws on the dark stage when
          the model is trusted.
        </p>
        {#if selectedLabel}
          <p class="muted" style="font-size:0.8rem;margin:0;">Source: {selectedLabel}</p>
        {/if}
        <button type="button" class="btn btn-primary" onclick={startCamera}>Enable camera</button>
      </div>
    {/if}
    <canvas bind:this={canvasEl}></canvas>
    <div class="stage-hud">
      {#if showSkeleton}
        <span class="tag ok">limbs live</span>
        <span class="tag">CSI model space · not camera-aligned</span>
      {:else}
        <span class="tag warn">limbs gated</span>
      {/if}
      {#if streaming}
        <span class="tag">local cam</span>
      {/if}
    </div>
  </div>
</div>

{#if !showSkeleton}
  <p class="empty gate-msg">
    No trusted limbs right now (need pose_source from model + max keypoint conf ≥
    {POSE_CONF_THRESHOLD}). Load DensePose in the model menu; Record + Train for room adaptation.
  </p>
{/if}

<style>
  .note {
    margin: -0.25rem 0 1rem;
    font-size: 0.8rem;
    max-width: 48rem;
    line-height: 1.4;
  }
  .cam-bar {
    align-items: center;
  }
  .cam-select {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    pointer-events: auto;
  }
  .cam-select-label {
    font-size: 0.75rem;
    color: var(--muted);
    font-weight: 600;
  }
  .cam-select select {
    min-width: 12rem;
    max-width: min(22rem, 50vw);
  }
  .stage-wrap {
    border: 1px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
    background: #080c14;
  }
  .stage {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    min-height: 320px;
    background: #0a0e16;
  }
  .stage video,
  .stage canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .stage canvas {
    pointer-events: none;
  }
  .stage video.hidden {
    visibility: hidden;
  }
  .stage-placeholder {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.85rem;
    color: var(--muted);
    font-size: 0.9rem;
    text-align: center;
    padding: 1.5rem;
    z-index: 1;
  }
  .stage-hud {
    position: absolute;
    left: 0.75rem;
    top: 0.75rem;
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    z-index: 2;
    pointer-events: none;
  }
  .tag {
    font-family: var(--mono);
    font-size: 0.68rem;
    font-weight: 600;
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: rgba(12, 15, 20, 0.72);
    color: var(--muted);
    backdrop-filter: blur(4px);
  }
  .tag.ok {
    color: var(--ok);
    border-color: rgba(62, 207, 142, 0.4);
  }
  .tag.warn {
    color: var(--warn);
    border-color: rgba(230, 184, 77, 0.4);
  }
  .gate-msg {
    margin-top: 0.85rem;
  }
</style>
