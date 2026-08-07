<script lang="ts">
  import { onMount } from 'svelte';
  import {
    Chart,
    LineController,
    LineElement,
    PointElement,
    LinearScale,
    CategoryScale,
    Filler,
  } from 'chart.js';
  import {
    api,
    gatedBreathingBpm,
    gatedHeartRateBpm,
    sensingWsUrl,
    type SensingUpdate,
  } from '../lib/api';
  import SourceChip from './SourceChip.svelte';

  Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, Filler);

  const MAX = 60;

  let connected = $state(false);
  let source = $state<string | null>(null);
  let latest = $state<SensingUpdate | null>(null);
  let error = $state<string | null>(null);

  let varCanvas: HTMLCanvasElement;
  let motionCanvas: HTMLCanvasElement;
  let breathCanvas: HTMLCanvasElement;
  let rssiCanvas: HTMLCanvasElement;

  let charts: Chart[] = [];
  const series = {
    variance: [] as number[],
    motion: [] as number[],
    breath: [] as number[],
    rssi: [] as number[],
    labels: [] as string[],
  };

  function push(f: Record<string, number> | undefined) {
    if (!f) return;
    const t = new Date().toLocaleTimeString();
    series.labels.push(t);
    series.variance.push(Number(f.variance ?? 0));
    series.motion.push(Number(f.motion_band_power ?? 0));
    series.breath.push(Number(f.breathing_band_power ?? 0));
    series.rssi.push(Number(f.mean_rssi ?? 0));
    if (series.labels.length > MAX) {
      for (const k of Object.keys(series) as (keyof typeof series)[]) {
        series[k].shift();
      }
    }
    for (const c of charts) c.update('none');
  }

  function makeChart(el: HTMLCanvasElement, label: string, data: number[], color: string) {
    return new Chart(el, {
      type: 'line',
      data: {
        labels: series.labels,
        datasets: [
          {
            label,
            data,
            borderColor: color,
            backgroundColor: color + '33',
            fill: true,
            tension: 0.25,
            pointRadius: 0,
            borderWidth: 1.5,
          },
        ],
      },
      options: {
        responsive: true,
        animation: false,
        scales: {
          x: { display: false },
          y: {
            ticks: { color: '#8b9bb0', maxTicksLimit: 4 },
            grid: { color: '#2a344155' },
          },
        },
        plugins: { legend: { display: false } },
      },
    });
  }

  function onFrame(data: SensingUpdate) {
    latest = data;
    source = data.source ?? source;
    push(data.features);
  }

  onMount(() => {
    charts = [
      makeChart(varCanvas, 'variance', series.variance, '#3d9cf0'),
      makeChart(motionCanvas, 'motion', series.motion, '#e6b84d'),
      makeChart(breathCanvas, 'breath', series.breath, '#3ecf8e'),
      makeChart(rssiCanvas, 'rssi', series.rssi, '#ef6b6b'),
    ];

    let ws: WebSocket | null = null;
    let closed = false;
    let poll: ReturnType<typeof setInterval> | undefined;

    const connect = () => {
      if (closed) return;
      try {
        ws = new WebSocket(sensingWsUrl());
      } catch (e) {
        error = e instanceof Error ? e.message : String(e);
        connected = false;
        return;
      }
      ws.onopen = () => {
        connected = true;
        error = null;
      };
      ws.onmessage = (ev) => {
        try {
          const data = JSON.parse(ev.data) as SensingUpdate;
          if ((data.type || data.msg_type) === 'sensing_update' || data.features) {
            onFrame(data);
          }
        } catch {
          /* ignore */
        }
      };
      ws.onclose = () => {
        connected = false;
        if (!closed) setTimeout(connect, 1500);
      };
      ws.onerror = () => {
        /* onclose handles retry */
      };
    };

    connect();
    // Seed from REST once
    api.sensingLatest().then(onFrame).catch(() => {});
    poll = setInterval(() => {
      if (!connected) api.sensingLatest().then(onFrame).catch(() => {});
    }, 2000);

    return () => {
      closed = true;
      ws?.close();
      if (poll) clearInterval(poll);
      for (const c of charts) c.destroy();
    };
  });
</script>

<div class="row" style="margin-bottom: 1rem;">
  <SourceChip {source} {connected} />
  {#if latest?.classification}
    <span class="chip muted">{latest.classification.motion_level ?? '—'}</span>
    <span class={`chip ${latest.classification.presence ? 'ok' : 'muted'}`}>
      presence {latest.classification.presence ? 'yes' : 'no'}
    </span>
    <span class="chip muted">
      est {latest.estimated_persons ?? '—'}
    </span>
    <span class="chip muted">
      conf {((latest.classification.confidence ?? 0) * 100).toFixed(0)}%
    </span>
    {@const br = gatedBreathingBpm(latest)}
    {@const hr = gatedHeartRateBpm(latest)}
    <span class={`chip ${br != null ? 'ok' : 'muted'}`} title="Experimental CSI breathing estimate">
      BR {br ?? '—'}
    </span>
    <span class={`chip ${hr != null ? 'ok' : 'muted'}`} title="Experimental CSI heart-rate estimate (needs stillness)">
      HR {hr ?? '—'}
    </span>
    {#if latest.vital_signs}
      <span class="chip muted">
        sq {((latest.vital_signs.signal_quality ?? 0) * 100).toFixed(0)}%
      </span>
    {/if}
  {/if}
</div>
<p class="muted" style="margin: -0.35rem 0 1rem; font-size: 0.8rem; max-width: 48rem;">
  Presence and motion are live CSI heuristics (no training). Breathing/heart BPM use FFT on the
  amplitude stream — wellness-grade only; they blank unless confidence and stillness pass the gate.
</p>

{#if error && !latest}
  <div class="card empty">{error}</div>
{/if}

<div class="grid grid-2">
  <div class="card">
    <h2>Variance (auto-scale)</h2>
    <canvas bind:this={varCanvas} height="120"></canvas>
    <div class="mono" style="margin-top:0.4rem;color:var(--muted)">
      {latest?.features?.variance?.toFixed?.(1) ?? '—'}
    </div>
  </div>
  <div class="card">
    <h2>Motion band power</h2>
    <canvas bind:this={motionCanvas} height="120"></canvas>
    <div class="mono" style="margin-top:0.4rem;color:var(--muted)">
      {latest?.features?.motion_band_power?.toFixed?.(1) ?? '—'}
    </div>
  </div>
  <div class="card">
    <h2>Breathing band power</h2>
    <canvas bind:this={breathCanvas} height="120"></canvas>
    <div class="mono" style="margin-top:0.4rem;color:var(--muted)">
      {latest?.features?.breathing_band_power?.toFixed?.(1) ?? '—'}
    </div>
  </div>
  <div class="card">
    <h2>Mean RSSI</h2>
    <canvas bind:this={rssiCanvas} height="120"></canvas>
    <div class="mono" style="margin-top:0.4rem;color:var(--muted)">
      {latest?.features?.mean_rssi?.toFixed?.(1) ?? '—'} dBm
    </div>
  </div>
</div>
