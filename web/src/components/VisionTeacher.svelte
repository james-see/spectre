<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { api } from '../lib/api';
  import { detectPoseOnVideo, type TeacherPoseSample } from '../lib/mediapipePose';
  import { drawPoseSkeleton } from '../lib/poseDraw';
  import { COCO_KP_NAMES, type PersonDetection } from '../lib/api';

  interface Props {
    /** When true, samples are POSTed to the active recording id. */
    recordingId?: string | null;
    enabled?: boolean;
  }
  let { recordingId = null, enabled = true }: Props = $props();

  let videoEl: HTMLVideoElement;
  let canvasEl: HTMLCanvasElement;
  let camError = $state<string | null>(null);
  let ready = $state(false);
  let poseHits = $state(0);
  let uploaded = $state(0);
  let stream: MediaStream | null = null;
  let raf = 0;
  let clockOffsetMs = 0;
  let batch: TeacherPoseSample[] = [];
  let lastDetect = 0;

  function sampleToPerson(s: TeacherPoseSample): PersonDetection {
    return {
      id: 1,
      confidence: 1,
      keypoints: s.keypoints.map((k) => ({
        name: k.name,
        x: k.x * 640,
        y: k.y * 480,
        z: k.z,
        confidence: k.confidence,
      })),
    };
  }

  async function flush() {
    if (!recordingId || !batch.length) {
      batch = [];
      return;
    }
    const chunk = batch.splice(0, batch.length);
    try {
      const res = await api.appendRecordingPose(recordingId, chunk);
      uploaded += Number(res.written ?? chunk.length);
    } catch (e) {
      camError = e instanceof Error ? e.message : String(e);
    }
  }

  async function loop() {
    if (typeof requestAnimationFrame === 'undefined') return;
    raf = requestAnimationFrame(loop);
    if (!enabled || !videoEl || videoEl.readyState < 2) return;
    const now = performance.now();
    if (now - lastDetect < 100) return;
    lastDetect = now;
    try {
      const sample = await detectPoseOnVideo(videoEl, now);
      const ctx = canvasEl?.getContext('2d');
      if (ctx && canvasEl) {
        ctx.clearRect(0, 0, canvasEl.width, canvasEl.height);
        if (sample) {
          poseHits += 1;
          drawPoseSkeleton(ctx, [sampleToPerson(sample)], {
            fillStyle: null,
            clear: false,
            bothEnds: true,
            minConf: 0.35,
          });
          if (recordingId) {
            const t_ms = Date.now() + clockOffsetMs;
            batch.push({ ...sample, t_ms });
            if (batch.length >= 5) void flush();
          }
        }
      }
    } catch (e) {
      camError = e instanceof Error ? e.message : String(e);
    }
  }

  onMount(async () => {
    try {
      const latest = await api.sensingLatest().catch(() => null);
      if (latest?.timestamp != null) {
        clockOffsetMs = Number(latest.timestamp) * 1000 - Date.now();
      }
      stream = await navigator.mediaDevices.getUserMedia({
        video: { width: { ideal: 640 }, height: { ideal: 480 }, facingMode: 'user' },
        audio: false,
      });
      videoEl.srcObject = stream;
      await videoEl.play();
      ready = true;
      void detectPoseOnVideo(videoEl, performance.now()).catch(() => {});
      raf = requestAnimationFrame(loop);
    } catch (e) {
      camError = e instanceof Error ? e.message : String(e);
    }
    const flushId = setInterval(() => void flush(), 1000);
    return () => clearInterval(flushId);
  });

  onDestroy(() => {
    if (typeof cancelAnimationFrame !== 'undefined') cancelAnimationFrame(raf);
    if (typeof window !== 'undefined') void flush();
    stream?.getTracks().forEach((t) => t.stop());
  });
</script>

<div class="teacher">
  <div class="stage">
    <video bind:this={videoEl} playsinline muted class="vid"></video>
    <canvas bind:this={canvasEl} width="640" height="480" class="ov"></canvas>
  </div>
  <div class="row meta">
    <span class={`chip ${ready ? 'ok' : 'muted'}`}>{ready ? 'camera on' : 'camera…'}</span>
    <span class="chip muted">teacher hits {poseHits}</span>
    {#if recordingId}
      <span class="chip ok">uploading → {recordingId}</span>
      <span class="chip muted">pose lines {uploaded}</span>
    {:else}
      <span class="chip muted">preview only</span>
    {/if}
  </div>
  {#if camError}
    <p class="empty" style="color:var(--bad)">{camError}</p>
  {/if}
  <p class="empty hint">
    MediaPipe labels body joints for CSI training (normalized [0,1]). This overlay is the teacher —
    not the CSI inference skeleton. Unused names: {COCO_KP_NAMES.length} COCO joints.
  </p>
</div>

<style>
  .teacher {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .stage {
    position: relative;
    width: 100%;
    max-width: 640px;
    aspect-ratio: 4/3;
    background: #0a0e14;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid var(--border);
  }
  .vid,
  .ov {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .ov {
    pointer-events: none;
  }
  .meta {
    gap: 0.4rem;
  }
  .hint {
    font-size: 0.75rem;
    margin: 0;
  }
</style>
