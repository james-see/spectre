<script lang="ts">
  import { onMount } from 'svelte';
  import * as THREE from 'three';
  import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
  import { api, sensingWsUrl, type SensingUpdate } from '../lib/api';
  import {
    CSI_NODE_IDS,
    CSI_NODE_SLOTS,
    FIELD_GRID,
    NODE_COLORS,
    ROOM_SIZE,
    fieldCellWorld,
  } from '../lib/roomLayout';
  import SourceChip from './SourceChip.svelte';

  type Pov = 'fused' | 1 | 2 | 3;

  let hostEl: HTMLDivElement;
  let canvasEl: HTMLCanvasElement;
  let connected = $state(false);
  let source = $state<string | null>(null);
  let pov = $state<Pov>('fused');
  let onlineNodes = $state(0);
  let maxField = $state(0);
  let povLabel = $state('Fused room');

  function setPov(next: Pov) {
    pov = next;
  }

  onMount(() => {
    const renderer = new THREE.WebGLRenderer({
      canvas: canvasEl,
      antialias: true,
      powerPreference: 'high-performance',
    });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.toneMapping = THREE.ACESFilmicToneMapping;
    renderer.toneMappingExposure = 0.92;

    const scene = new THREE.Scene();
    scene.background = new THREE.Color(0x070a10);
    scene.fog = new THREE.FogExp2(0x070a10, 0.032);

    const camera = new THREE.PerspectiveCamera(48, 1, 0.1, 200);
    camera.position.set(7.5, 7.2, 9.5);

    const controls = new OrbitControls(camera, canvasEl);
    controls.enableDamping = true;
    controls.dampingFactor = 0.08;
    controls.minDistance = 2;
    controls.maxDistance = 24;
    controls.maxPolarAngle = Math.PI * 0.49;
    controls.target.set(0, 0.4, 0);

    scene.add(new THREE.AmbientLight(0x9aabbd, 0.55));
    scene.add(new THREE.HemisphereLight(0x5a6f88, 0x121820, 0.85));
    const key = new THREE.DirectionalLight(0xe8eef8, 0.9);
    key.position.set(5, 9, 4);
    scene.add(key);

    const grid = new THREE.GridHelper(ROOM_SIZE.w, 24, 0x243848, 0x152030);
    (grid.material as THREE.Material).opacity = 0.35;
    (grid.material as THREE.Material).transparent = true;
    scene.add(grid);

    const roomWire = new THREE.LineSegments(
      new THREE.EdgesGeometry(new THREE.BoxGeometry(ROOM_SIZE.w, ROOM_SIZE.h, ROOM_SIZE.d)),
      new THREE.LineBasicMaterial({ color: 0x2a3d52, opacity: 0.45, transparent: true }),
    );
    roomWire.position.y = ROOM_SIZE.h / 2;
    scene.add(roomWire);

    const floor = new THREE.Mesh(
      new THREE.PlaneGeometry(ROOM_SIZE.w, ROOM_SIZE.d),
      new THREE.MeshStandardMaterial({
        color: 0x0c121a,
        roughness: 0.92,
        metalness: 0.08,
        emissive: 0x04080e,
        emissiveIntensity: 0.2,
      }),
    );
    floor.rotation.x = -Math.PI / 2;
    scene.add(floor);

    const fieldCount = FIELD_GRID * FIELD_GRID;
    const fieldPos = new Float32Array(fieldCount * 3);
    const fieldColors = new Float32Array(fieldCount * 3);
    for (let iz = 0; iz < FIELD_GRID; iz++) {
      for (let ix = 0; ix < FIELD_GRID; ix++) {
        const idx = iz * FIELD_GRID + ix;
        const { x, z } = fieldCellWorld(ix, iz);
        fieldPos[idx * 3] = x;
        fieldPos[idx * 3 + 1] = 0.02;
        fieldPos[idx * 3 + 2] = z;
      }
    }
    const fieldGeo = new THREE.BufferGeometry();
    fieldGeo.setAttribute('position', new THREE.BufferAttribute(fieldPos, 3));
    fieldGeo.setAttribute('color', new THREE.BufferAttribute(fieldColors, 3));
    scene.add(
      new THREE.Points(
        fieldGeo,
        new THREE.PointsMaterial({
          size: 0.24,
          vertexColors: true,
          transparent: true,
          opacity: 0.55,
          blending: THREE.AdditiveBlending,
          depthWrite: false,
          sizeAttenuation: true,
        }),
      ),
    );

    type NodeViz = {
      id: number;
      beacon: THREE.Mesh;
      beaconMat: THREE.MeshStandardMaterial;
      halo: THREE.Mesh;
      haloMat: THREE.MeshBasicMaterial;
      spectrumGroup: THREE.Group;
      bars: THREE.Mesh[];
      barMats: THREE.MeshBasicMaterial[];
    };
    const nodeViz = new Map<number, NodeViz>();

    for (const id of CSI_NODE_IDS) {
      const slot = CSI_NODE_SLOTS[id];
      const color = NODE_COLORS[id % NODE_COLORS.length];
      const beaconMat = new THREE.MeshStandardMaterial({
        color: 0x121820,
        emissive: color,
        emissiveIntensity: 0.55,
        metalness: 0.4,
        roughness: 0.35,
        transparent: true,
        opacity: 0.9,
      });
      const beacon = new THREE.Mesh(new THREE.SphereGeometry(0.14, 20, 20), beaconMat);
      beacon.position.set(slot.x, slot.y, slot.z);
      scene.add(beacon);

      const haloMat = new THREE.MeshBasicMaterial({
        color,
        transparent: true,
        opacity: 0.15,
        side: THREE.DoubleSide,
        blending: THREE.AdditiveBlending,
        depthWrite: false,
      });
      const halo = new THREE.Mesh(new THREE.RingGeometry(0.35, 0.95, 48), haloMat);
      halo.rotation.x = -Math.PI / 2;
      halo.position.set(slot.x, 0.04, slot.z);
      scene.add(halo);

      const spectrumGroup = new THREE.Group();
      spectrumGroup.position.set(slot.x, 0.08, slot.z);
      scene.add(spectrumGroup);

      const bars: THREE.Mesh[] = [];
      const barMats: THREE.MeshBasicMaterial[] = [];
      const nBars = 56;
      for (let k = 0; k < nBars; k++) {
        const mat = new THREE.MeshBasicMaterial({
          color,
          transparent: true,
          opacity: 0.55,
          depthWrite: false,
        });
        const bar = new THREE.Mesh(new THREE.BoxGeometry(0.04, 1, 0.04), mat);
        bar.geometry.translate(0, 0.5, 0);
        const ang = (k / nBars) * Math.PI * 2;
        const rad = 0.55;
        bar.position.set(Math.cos(ang) * rad, 0, Math.sin(ang) * rad);
        bar.scale.y = 0.05;
        spectrumGroup.add(bar);
        bars.push(bar);
        barMats.push(mat);
      }

      nodeViz.set(id, {
        id,
        beacon,
        beaconMat,
        halo,
        haloMat,
        spectrumGroup,
        bars,
        barMats,
      });
    }

    let latest: SensingUpdate | null = null;
    let closed = false;
    let ws: WebSocket | null = null;
    let appliedPov: Pov = 'fused';

    const applyPov = (next: Pov) => {
      appliedPov = next;
      if (next === 'fused') {
        povLabel = 'Fused room';
        controls.enabled = true;
        camera.position.set(7.5, 7.2, 9.5);
        controls.target.set(0, 0.4, 0);
        controls.update();
        return;
      }
      const slot = CSI_NODE_SLOTS[next];
      povLabel = `Node ${next} POV`;
      controls.enabled = false;
      camera.position.set(slot.x, slot.y + 0.35, slot.z);
      camera.lookAt(0, 0.6, 0);
    };

    const resize = () => {
      const w = hostEl.clientWidth;
      const h = hostEl.clientHeight;
      if (w < 1 || h < 1) return;
      renderer.setSize(w, h, false);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
    };
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(hostEl);

    const connect = () => {
      if (closed) return;
      try {
        ws = new WebSocket(sensingWsUrl());
      } catch {
        connected = false;
        return;
      }
      ws.onopen = () => {
        connected = true;
      };
      ws.onmessage = (ev) => {
        try {
          const data = JSON.parse(ev.data) as SensingUpdate;
          if ((data.type || data.msg_type) === 'sensing_update' || data.features) {
            latest = data;
            source = data.source ?? source;
          }
        } catch {
          /* ignore */
        }
      };
      ws.onclose = () => {
        connected = false;
        if (!closed) setTimeout(connect, 1500);
      };
    };
    connect();
    api
      .sensingLatest()
      .then((d) => {
        latest = d;
        source = d.source ?? source;
      })
      .catch(() => {});

    const heatColor = (v: number) => {
      let r = 0,
        g = 0,
        b = 0;
      if (v < 0.3) {
        g = v * 1.5;
        b = v * 0.3;
      } else if (v < 0.6) {
        const t = (v - 0.3) / 0.3;
        r = t * 0.3;
        g = 0.45 + t * 0.4;
        b = 0.09 - t * 0.05;
      } else {
        const t = (v - 0.6) / 0.4;
        r = 0.3 + t * 0.7;
        g = 0.85 - t * 0.2;
        b = 0.04;
      }
      return [r, g, b] as const;
    };

    let raf = 0;
    const animate = () => {
      raf = requestAnimationFrame(animate);
      if (pov !== appliedPov) applyPov(pov);

      const data = latest;
      const field = data?.signal_field?.values;
      let peak = 0;
      if (field) {
        const count = Math.min(field.length, fieldCount);
        for (let i = 0; i < count; i++) {
          const v = field[i] || 0;
          if (v > peak) peak = v;
          const [r, g, b] = heatColor(v);
          fieldColors[i * 3] = r;
          fieldColors[i * 3 + 1] = g;
          fieldColors[i * 3 + 2] = b;
        }
        fieldGeo.attributes.color.needsUpdate = true;
      }
      maxField = peak;

      const liveIds = new Set((data?.nodes || []).map((n) => n.node_id));
      onlineNodes = liveIds.size;

      for (const [id, viz] of nodeViz) {
        const on = liveIds.has(id);
        const feats = data?.node_features?.find((n) => n.node_id === id);
        const variance = Number(feats?.features?.variance ?? 0);
        const motion = Number(feats?.features?.motion_band_power ?? 0);
        const energy = Math.min(1, (variance / 800 + motion / 600) * 0.5);
        viz.beaconMat.opacity = on ? 0.95 : 0.25;
        viz.beaconMat.emissiveIntensity = on ? 0.55 + energy * 0.6 : 0.12;
        viz.haloMat.opacity = on ? 0.12 + energy * 0.45 : 0.04;
        const haloScale = 0.85 + energy * 1.4;
        viz.halo.scale.setScalar(haloScale);

        const isPov = pov === id;
        const showSpec = isPov && on;
        viz.spectrumGroup.visible = showSpec || (pov === 'fused' && on);
        const amp =
          data?.nodes?.find((n) => n.node_id === id)?.amplitude ||
          [];
        let maxAmp = 1e-6;
        for (const a of amp) if (a > maxAmp) maxAmp = a;
        const dim = pov === 'fused' ? 0.35 : isPov ? 1 : 0.12;
        for (let k = 0; k < viz.bars.length; k++) {
          const a = amp[k] ?? 0;
          const h = 0.08 + (a / maxAmp) * 1.35;
          viz.bars[k].scale.y = showSpec || pov === 'fused' ? h : 0.04;
          viz.barMats[k].opacity = (showSpec ? 0.7 : 0.25) * dim * (on ? 1 : 0.2);
        }
      }

      if (controls.enabled) controls.update();
      renderer.render(scene, camera);
    };
    animate();

    return () => {
      closed = true;
      cancelAnimationFrame(raf);
      ro.disconnect();
      ws?.close();
      renderer.dispose();
      scene.traverse((obj) => {
        if (
          obj instanceof THREE.Mesh ||
          obj instanceof THREE.Points ||
          obj instanceof THREE.LineSegments
        ) {
          obj.geometry?.dispose?.();
          const m = obj.material;
          if (Array.isArray(m)) m.forEach((x) => x.dispose());
          else m?.dispose?.();
        }
      });
    };
  });
</script>

<div class="field-wrap" bind:this={hostEl}>
  <canvas class="field-canvas" bind:this={canvasEl}></canvas>
  <div class="field-hud">
    <div class="row">
      <SourceChip {source} {connected} />
      <span class="chip muted">{povLabel}</span>
      <span class="chip muted">csi {onlineNodes}</span>
      <span class="chip muted">peak {maxField.toFixed(2)}</span>
    </div>
    <div class="row pov-row">
      <button
        type="button"
        class={`chip pov ${pov === 'fused' ? 'ok' : 'muted'}`}
        onclick={() => setPov('fused')}>Fused</button
      >
      {#each CSI_NODE_IDS as id}
        <button
          type="button"
          class={`chip pov ${pov === id ? 'ok' : 'muted'}`}
          onclick={() => setPov(id)}>Node {id}</button
        >
      {/each}
    </div>
    <p class="hint">
      Fused heat = server field heuristic. Node POV = camera at that ESP32 + local energy halo +
      subcarrier amplitude fan (CSI signature, not spatial vision).
    </p>
  </div>
</div>

<style>
  .field-wrap {
    position: relative;
    width: 100%;
    height: calc(100vh - 3.4rem);
    min-height: 420px;
    border-radius: 10px;
    overflow: hidden;
    border: 1px solid var(--border);
    background: #080c14;
  }
  .field-canvas {
    width: 100%;
    height: 100%;
    display: block;
    cursor: grab;
  }
  .field-hud {
    position: absolute;
    left: 0.85rem;
    top: 0.85rem;
    right: 0.85rem;
    z-index: 2;
    pointer-events: none;
  }
  .field-hud .row {
    pointer-events: none;
  }
  .field-hud :global(.chip),
  .field-hud .chip.pov {
    pointer-events: auto;
  }
  .pov-row {
    margin-top: 0.4rem;
    gap: 0.4rem;
  }
  .field-hud button.chip.pov {
    appearance: none;
    -webkit-appearance: none;
    cursor: pointer;
    margin: 0;
    font: inherit;
    font-family: var(--mono);
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--muted);
    background: rgba(12, 15, 20, 0.55);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0.2rem 0.55rem;
    box-shadow: none;
  }
  .field-hud button.chip.pov.ok {
    color: var(--ok);
    border-color: rgba(62, 207, 142, 0.35);
    background: rgba(62, 207, 142, 0.08);
  }
  .field-hud button.chip.pov:hover {
    color: var(--text);
    border-color: #3a4a5c;
  }
  .hint {
    margin: 0.5rem 0 0;
    color: var(--muted);
    font-size: 0.75rem;
    max-width: 36rem;
    line-height: 1.35;
    text-shadow: 0 1px 2px #000a;
  }
</style>
