<script lang="ts">
  import { onMount } from 'svelte';
  import * as THREE from 'three';
  import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
  import {
    api,
    gatedBreathingBpm,
    gatedHeartRateBpm,
    sensingWsUrl,
    type DecoLanClient,
    type DecoMeshNode,
    type NodeRow,
    type SensingUpdate,
  } from '../lib/api';
  import SourceChip from './SourceChip.svelte';

  const C = {
    greenGlow: 0x3ecf8e,
    greenDim: 0x1a3d2e,
    blueSignal: 0x4aa3ff,
    bgDeep: 0x070a10,
    decoMaster: 0xe8a145,
    decoSat: 0xc47a2e,
    lanClient: 0x8ea4bc,
    presence: 0x5ee0a8,
  };

  let hudOpen = $state(false);

  const NODE_COLORS = [0x3d9cf0, 0xe6b84d, 0x3ecf8e, 0xef6b6b];

  type HoverEsp = {
    kind: 'esp32';
    id: number;
    x: number;
    y: number;
    online: boolean;
    rssi: string;
    lastSeen: string;
    motion: string;
    persons: string;
    subcarriers: string;
    variance: string;
    conf: string;
  };

  type HoverDeco = {
    kind: 'deco';
    x: number;
    y: number;
    name: string;
    role: string;
    ip: string;
    mac: string;
    online: boolean;
  };

  type HoverClient = {
    kind: 'client';
    x: number;
    y: number;
    name: string;
    ip: string;
    mac: string;
    band: string;
    deco: string;
    rates: string;
    online: boolean;
  };

  type HoverInfo = HoverEsp | HoverDeco | HoverClient;

  function hashMac(mac: string): number {
    let h = 0;
    for (let i = 0; i < mac.length; i++) h = (h * 31 + mac.charCodeAt(i)) >>> 0;
    return h;
  }

  let hostEl: HTMLDivElement;
  let canvasEl: HTMLCanvasElement;
  let connected = $state(false);
  let source = $state<string | null>(null);
  let presence = $state(false);
  let motion = $state('—');
  let est = $state<number | null>(null);
  let classConf = $state<number | null>(null);
  let hrBpm = $state<number | null>(null);
  let brBpm = $state<number | null>(null);
  let vitalsQuality = $state<number | null>(null);
  let breathBand = $state<number | null>(null);
  let nodeCount = $state(0);
  let lanClientCount = $state(0);
  let lanMeshCount = $state(0);
  let lanAgeSec = $state<number | null>(null);
  let decoFeedDown = $state(false);
  let hover = $state<HoverInfo | null>(null);

  onMount(() => {
    const renderer = new THREE.WebGLRenderer({
      canvas: canvasEl,
      antialias: true,
      powerPreference: 'high-performance',
    });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.toneMapping = THREE.ACESFilmicToneMapping;
    renderer.toneMappingExposure = 0.92;
    renderer.shadowMap.enabled = true;
    renderer.shadowMap.type = THREE.PCFSoftShadowMap;

    const scene = new THREE.Scene();
    scene.background = new THREE.Color(C.bgDeep);
    scene.fog = new THREE.FogExp2(C.bgDeep, 0.035);

    const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 300);
    camera.position.set(7.2, 6.4, 9.2);
    camera.lookAt(0, 0.6, 0);

    const controls = new OrbitControls(camera, canvasEl);
    controls.enableDamping = true;
    controls.dampingFactor = 0.07;
    controls.minDistance = 3;
    controls.maxDistance = 22;
    controls.maxPolarAngle = Math.PI * 0.48;
    controls.target.set(0, 0.55, 0);

    // Lights — cooler, less washed-out
    scene.add(new THREE.AmbientLight(0x9aabbd, 0.55));
    scene.add(new THREE.HemisphereLight(0x5a6f88, 0x121820, 0.85));
    const key = new THREE.DirectionalLight(0xe8eef8, 0.95);
    key.position.set(5, 9, 4);
    key.castShadow = true;
    key.shadow.mapSize.set(1024, 1024);
    scene.add(key);
    scene.add(new THREE.DirectionalLight(0x6a7f99, 0.35).translateX(-5).translateY(4).translateZ(-3));

    // Room
    const grid = new THREE.GridHelper(12, 24, 0x243848, 0x152030);
    (grid.material as THREE.Material).opacity = 0.35;
    (grid.material as THREE.Material).transparent = true;
    scene.add(grid);

    const roomWire = new THREE.LineSegments(
      new THREE.EdgesGeometry(new THREE.BoxGeometry(12, 3.6, 10)),
      new THREE.LineBasicMaterial({ color: 0x2a3d52, opacity: 0.45, transparent: true }),
    );
    roomWire.position.y = 1.8;
    scene.add(roomWire);

    const floor = new THREE.Mesh(
      new THREE.PlaneGeometry(12, 10),
      new THREE.MeshStandardMaterial({
        color: 0x0c121a,
        roughness: 0.92,
        metalness: 0.08,
        emissive: 0x04080e,
        emissiveIntensity: 0.2,
      }),
    );
    floor.rotation.x = -Math.PI / 2;
    floor.receiveShadow = true;
    scene.add(floor);

    // Low plinth for master Deco (no toy router)
    const plinth = new THREE.Mesh(
      new THREE.CylinderGeometry(0.42, 0.48, 0.08, 32),
      new THREE.MeshStandardMaterial({
        color: 0x1a222c,
        roughness: 0.7,
        metalness: 0.35,
      }),
    );
    plinth.position.set(-4, 0.04, -3);
    plinth.receiveShadow = true;
    scene.add(plinth);

    const routerGroup = new THREE.Group();
    routerGroup.position.set(-4, 0.12, -3);
    const routerLight = new THREE.PointLight(C.blueSignal, 0.55, 6);
    routerLight.position.set(0, 0.4, 0);
    routerGroup.add(routerLight);
    scene.add(routerGroup);

    const pickables: THREE.Object3D[] = [];

    // Deco AP discs (master at router; satellites at fixed room offsets)
    const masterPos = routerGroup.position.clone();
    const satelliteSlots = [
      new THREE.Vector3(4.5, 0.85, -3.2),
      new THREE.Vector3(-1.5, 0.85, 3.8),
    ];
    const decoGroup = new THREE.Group();
    scene.add(decoGroup);
    const clientGroup = new THREE.Group();
    scene.add(clientGroup);
    const decoMarkers = new Map<string, THREE.Mesh>();
    const clientMarkers = new Map<string, THREE.Mesh>();
    let meshSnapshot: DecoMeshNode[] = [];
    let clientSnapshot: DecoLanClient[] = [];

    const makeLabelSprite = (text: string) => {
      const canvas = document.createElement('canvas');
      canvas.width = 256;
      canvas.height = 64;
      const ctx = canvas.getContext('2d')!;
      ctx.clearRect(0, 0, 256, 64);
      ctx.fillStyle = 'rgba(8,12,18,0.72)';
      ctx.beginPath();
      ctx.roundRect(8, 12, 240, 40, 8);
      ctx.fill();
      ctx.font = '600 22px "IBM Plex Sans", sans-serif';
      ctx.fillStyle = '#d7e2ef';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(text.slice(0, 18), 128, 32);
      const tex = new THREE.CanvasTexture(canvas);
      tex.colorSpace = THREE.SRGBColorSpace;
      const sprite = new THREE.Sprite(
        new THREE.SpriteMaterial({ map: tex, transparent: true, depthWrite: false }),
      );
      sprite.scale.set(1.35, 0.34, 1);
      sprite.raycast = () => {};
      return sprite;
    };

    const makeDecoDisc = (node: DecoMeshNode, pos: THREE.Vector3) => {
      const isMaster = node.role === 'master';
      const color = isMaster ? C.decoMaster : C.decoSat;
      const g = new THREE.Group();
      g.position.copy(pos);
      const disc = new THREE.Mesh(
        new THREE.CylinderGeometry(isMaster ? 0.32 : 0.24, isMaster ? 0.32 : 0.24, 0.04, 36),
        new THREE.MeshStandardMaterial({
          color: 0x1c2430,
          emissive: color,
          emissiveIntensity: node.online ? 0.35 : 0.08,
          metalness: 0.55,
          roughness: 0.4,
        }),
      );
      const ring = new THREE.Mesh(
        new THREE.TorusGeometry(isMaster ? 0.36 : 0.28, 0.018, 8, 48),
        new THREE.MeshBasicMaterial({
          color,
          transparent: true,
          opacity: node.online ? 0.85 : 0.25,
        }),
      );
      ring.rotation.x = Math.PI / 2;
      ring.position.y = 0.02;
      disc.userData.kind = 'deco';
      disc.userData.mac = node.mac;
      disc.name = `deco-${node.mac}`;
      g.add(disc, ring);
      const label = makeLabelSprite(node.name || (isMaster ? 'master' : 'satellite'));
      label.position.set(0, 0.55, 0);
      g.add(label);
      decoGroup.add(g);
      decoMarkers.set(node.mac, disc);
      pickables.push(disc);
      return disc;
    };

    const placeDecos = (mesh: DecoMeshNode[]) => {
      for (const m of decoMarkers.values()) {
        const parent = m.parent;
        if (parent && parent !== decoGroup) {
          decoGroup.remove(parent);
          parent.traverse((obj) => {
            if (obj instanceof THREE.Mesh || obj instanceof THREE.Sprite) {
              obj.geometry?.dispose?.();
              const mat = obj.material as THREE.Material | THREE.Material[];
              if (Array.isArray(mat)) mat.forEach((x) => x.dispose());
              else mat?.dispose?.();
            }
          });
        } else {
          decoGroup.remove(m);
          m.geometry.dispose();
          (m.material as THREE.Material).dispose();
        }
        const idx = pickables.indexOf(m);
        if (idx >= 0) pickables.splice(idx, 1);
      }
      decoMarkers.clear();
      const sorted = [...mesh].sort((a, b) => {
        if (a.role === 'master' && b.role !== 'master') return -1;
        if (b.role === 'master' && a.role !== 'master') return 1;
        return a.mac.localeCompare(b.mac);
      });
      let satIdx = 0;
      for (const node of sorted) {
        const pos =
          node.role === 'master'
            ? masterPos.clone()
            : satelliteSlots[satIdx++ % satelliteSlots.length].clone();
        makeDecoDisc(node, pos);
      }
    };

    // Room box is 12×10 units. Treat 1 unit ≈ 3 ft.
    // Deco gives association (which AP) but not AoA/RSSI ranging — so direction
    // is an inward bearing from each AP toward room center (+ small fan/jitter),
    // not a claimed RF angle-of-arrival.
    const FT_PER_UNIT = 3;
    const ROOM_CENTER = new THREE.Vector3(0, 0, 0);
    const clientOffset = (mac: string, i: number, apPos: THREE.Vector3) => {
      const h = hashMac(mac);
      const inward = new THREE.Vector3(
        ROOM_CENTER.x - apPos.x,
        0,
        ROOM_CENTER.z - apPos.z,
      );
      if (inward.lengthSq() < 0.05) inward.set(1, 0, 0);
      inward.normalize();
      const baseAngle = Math.atan2(inward.z, inward.x);
      // Fan ~±55° around the inward bearing so clients fill the room side of the AP.
      const jitter = (((h % 100) / 100) - 0.5) * (Math.PI * 0.55);
      const slot = ((i % 7) - 3) * 0.16;
      const angle = baseAngle + jitter + slot;
      const distFt = 3.5 + (i % 6) * 0.7 + ((h >> 8) % 25) / 25;
      const r = distFt / FT_PER_UNIT;
      return new THREE.Vector3(
        Math.cos(angle) * r,
        0.14 + ((h >> 16) % 40) / 250,
        Math.sin(angle) * r,
      );
    };

    const clientTargets = new Map<string, THREE.Vector3>();

    const placeClients = (clients: DecoLanClient[]) => {
      const byDeco = new Map<string, DecoLanClient[]>();
      for (const c of clients) {
        const key = c.deco_mac || '_orphan';
        if (!byDeco.has(key)) byDeco.set(key, []);
        byDeco.get(key)!.push(c);
      }
      const seen = new Set<string>();
      for (const [decoMac, list] of byDeco) {
        const disc = decoMarkers.get(decoMac);
        const base = (disc?.parent?.position ?? disc?.position ?? masterPos).clone();
        // Stable order so slot fan doesn't reshuffle every poll
        list.sort((a, b) => a.mac.localeCompare(b.mac));
        list.forEach((c, i) => {
          seen.add(c.mac);
          const target = base.clone().add(clientOffset(c.mac, i, base));
          clientTargets.set(c.mac, target);
          let orb = clientMarkers.get(c.mac);
          if (!orb) {
            orb = new THREE.Mesh(
              new THREE.SphereGeometry(0.055, 12, 12),
              new THREE.MeshStandardMaterial({
                color: C.lanClient,
                emissive: C.lanClient,
                emissiveIntensity: c.online ? 0.22 : 0.05,
                transparent: true,
                opacity: c.online ? 0.75 : 0.25,
                roughness: 0.35,
                metalness: 0.2,
              }),
            );
            orb.position.copy(target);
            orb.userData.kind = 'client';
            orb.userData.mac = c.mac;
            orb.name = `client-${c.mac}`;
            clientGroup.add(orb);
            clientMarkers.set(c.mac, orb);
            pickables.push(orb);
          } else {
            const mat = orb.material as THREE.MeshStandardMaterial;
            mat.opacity = c.online ? 0.85 : 0.3;
            mat.emissiveIntensity = c.online ? 0.4 : 0.08;
          }
        });
      }
      for (const [mac, orb] of [...clientMarkers.entries()]) {
        if (seen.has(mac)) continue;
        clientGroup.remove(orb);
        orb.geometry.dispose();
        (orb.material as THREE.Material).dispose();
        const idx = pickables.indexOf(orb);
        if (idx >= 0) pickables.splice(idx, 1);
        clientMarkers.delete(mac);
        clientTargets.delete(mac);
      }
    };

    // Soft RF ripples from master (rings, not wireframe globes)
    const wifiWaves: { mesh: THREE.Mesh; mat: THREE.MeshBasicMaterial; phase: number }[] = [];
    for (let i = 0; i < 3; i++) {
      const mat = new THREE.MeshBasicMaterial({
        color: C.blueSignal,
        transparent: true,
        opacity: 0,
        side: THREE.DoubleSide,
        blending: THREE.AdditiveBlending,
        depthWrite: false,
      });
      const ring = new THREE.Mesh(new THREE.RingGeometry(0.55, 0.62, 64), mat);
      ring.rotation.x = -Math.PI / 2;
      ring.position.copy(routerGroup.position);
      ring.position.y = 0.05;
      scene.add(ring);
      wifiWaves.push({ mesh: ring, mat, phase: i * 1.1 });
    }

    // Signal field — quiet floor heat
    const gridSize = 20;
    const fieldCount = gridSize * gridSize;
    const fieldPos = new Float32Array(fieldCount * 3);
    const fieldColors = new Float32Array(fieldCount * 3);
    for (let iz = 0; iz < gridSize; iz++) {
      for (let ix = 0; ix < gridSize; ix++) {
        const idx = iz * gridSize + ix;
        fieldPos[idx * 3] = (ix - gridSize / 2) * 0.6;
        fieldPos[idx * 3 + 1] = 0.015;
        fieldPos[idx * 3 + 2] = (iz - gridSize / 2) * 0.5;
      }
    }
    const fieldGeo = new THREE.BufferGeometry();
    fieldGeo.setAttribute('position', new THREE.BufferAttribute(fieldPos, 3));
    fieldGeo.setAttribute('color', new THREE.BufferAttribute(fieldColors, 3));
    const fieldPoints = new THREE.Points(
      fieldGeo,
      new THREE.PointsMaterial({
        size: 0.22,
        vertexColors: true,
        transparent: true,
        opacity: 0.45,
        blending: THREE.AdditiveBlending,
        depthWrite: false,
        sizeAttenuation: true,
      }),
    );
    scene.add(fieldPoints);

    // ESP32 CSI nodes — compact beacons
    const nodeSlots: Record<number, THREE.Vector3> = {
      1: new THREE.Vector3(-5, 1.05, -4),
      2: new THREE.Vector3(5, 1.05, -4),
      3: new THREE.Vector3(0, 1.05, 4),
    };
    const nodeMarkers = new Map<number, THREE.Mesh>();
    for (const [id, pos] of Object.entries(nodeSlots)) {
      const nid = Number(id);
      const color = NODE_COLORS[nid % NODE_COLORS.length];
      const m = new THREE.Mesh(
        new THREE.SphereGeometry(0.14, 20, 20),
        new THREE.MeshStandardMaterial({
          color: 0x121820,
          emissive: color,
          emissiveIntensity: 0.55,
          metalness: 0.4,
          roughness: 0.35,
          transparent: true,
          opacity: 0.9,
        }),
      );
      m.position.copy(pos);
      m.userData.kind = 'esp32';
      m.userData.nodeId = nid;
      m.name = `node-${nid}`;
      scene.add(m);
      nodeMarkers.set(nid, m);
      pickables.push(m);
      const halo = new THREE.Mesh(
        new THREE.RingGeometry(0.22, 0.28, 32),
        new THREE.MeshBasicMaterial({
          color,
          transparent: true,
          opacity: 0.35,
          side: THREE.DoubleSide,
        }),
      );
      halo.rotation.x = -Math.PI / 2;
      halo.position.set(pos.x, 0.03, pos.z);
      halo.raycast = () => {};
      scene.add(halo);
      const pole = new THREE.Mesh(
        new THREE.CylinderGeometry(0.012, 0.012, pos.y),
        new THREE.MeshBasicMaterial({ color: 0x2a3544, transparent: true, opacity: 0.55 }),
      );
      pole.position.set(pos.x, pos.y / 2, pos.z);
      pole.raycast = () => {};
      scene.add(pole);
      const nLabel = makeLabelSprite(`CSI ${nid}`);
      nLabel.position.set(pos.x, pos.y + 0.38, pos.z);
      scene.add(nLabel);
    }

    const raycaster = new THREE.Raycaster();
    const pointer = new THREE.Vector2();
    let hoveredKey: string | null = null;
    let hoveredMesh: THREE.Mesh | null = null;
    let pointerClientX = 0;
    let pointerClientY = 0;
    let nodeRest = new Map<number, NodeRow>();

    const tipXY = () => {
      const rect = hostEl.getBoundingClientRect();
      return {
        x: pointerClientX - rect.left + 12,
        y: pointerClientY - rect.top + 12,
      };
    };

    const clearHoverScale = () => {
      if (hoveredMesh) hoveredMesh.scale.setScalar(1);
      hoveredMesh = null;
      hoveredKey = null;
    };

    const buildEspHover = (nid: number): HoverEsp => {
      const fromWs = latest?.nodes?.find((n) => n.node_id === nid);
      const feats = latest?.node_features?.find((n) => n.node_id === nid);
      const fromRest = nodeRest.get(nid);
      const online = !!(fromWs || (fromRest && (fromRest.last_seen_ms ?? 9999) < 5000));
      const rssiVal = fromWs?.rssi_dbm ?? fromRest?.rssi_dbm ?? feats?.rssi_dbm;
      const motionLvl =
        feats?.classification?.motion_level ?? fromRest?.motion_level ?? (online ? '—' : 'offline');
      const conf = feats?.classification?.confidence;
      const variance = feats?.features?.variance;
      return {
        kind: 'esp32',
        id: nid,
        ...tipXY(),
        online,
        rssi: rssiVal != null ? `${Number(rssiVal).toFixed(1)} dBm` : '—',
        lastSeen: fromRest?.last_seen_ms != null ? `${fromRest.last_seen_ms} ms` : online ? 'live' : '—',
        motion: motionLvl,
        persons: fromRest?.person_count != null ? String(fromRest.person_count) : '—',
        subcarriers: fromWs?.subcarrier_count != null ? String(fromWs.subcarrier_count) : '—',
        variance: variance != null ? Number(variance).toFixed(1) : '—',
        conf: conf != null ? `${(conf * 100).toFixed(0)}%` : '—',
      };
    };

    const buildDecoHover = (mac: string): HoverDeco | null => {
      const node = meshSnapshot.find((n) => n.mac === mac);
      if (!node) return null;
      return {
        kind: 'deco',
        ...tipXY(),
        name: node.name || mac,
        role: node.role,
        ip: node.ip || '—',
        mac: node.mac,
        online: !!node.online,
      };
    };

    const buildClientHover = (mac: string): HoverClient | null => {
      const c = clientSnapshot.find((x) => x.mac === mac);
      if (!c) return null;
      const up = c.up_rate_kbps != null ? `${c.up_rate_kbps}` : '—';
      const down = c.down_rate_kbps != null ? `${c.down_rate_kbps}` : '—';
      return {
        kind: 'client',
        ...tipXY(),
        name: c.name || mac,
        ip: c.ip || '—',
        mac: c.mac,
        band: c.band || c.connection_type || '—',
        deco: c.deco_name || c.deco_mac || '—',
        rates: `↑${up} / ↓${down}`,
        online: !!c.online,
      };
    };

    const buildHoverFor = (obj: THREE.Object3D): HoverInfo | null => {
      const kind = obj.userData.kind as string;
      if (kind === 'esp32') return buildEspHover(obj.userData.nodeId as number);
      if (kind === 'deco') return buildDecoHover(obj.userData.mac as string);
      if (kind === 'client') return buildClientHover(obj.userData.mac as string);
      return null;
    };

    const onPointerMove = (ev: PointerEvent) => {
      pointerClientX = ev.clientX;
      pointerClientY = ev.clientY;
      const rect = canvasEl.getBoundingClientRect();
      pointer.x = ((ev.clientX - rect.left) / rect.width) * 2 - 1;
      pointer.y = -((ev.clientY - rect.top) / rect.height) * 2 + 1;
      raycaster.setFromCamera(pointer, camera);
      const hits = raycaster.intersectObjects(pickables, false);
      if (hits.length === 0) {
        clearHoverScale();
        hover = null;
        canvasEl.style.cursor = 'grab';
        return;
      }
      const obj = hits[0].object as THREE.Mesh;
      const key = obj.name;
      if (hoveredKey !== key) {
        clearHoverScale();
        hoveredKey = key;
        hoveredMesh = obj;
        obj.scale.setScalar(1.35);
      }
      canvasEl.style.cursor = 'pointer';
      hover = buildHoverFor(obj);
    };

    const onPointerLeave = () => {
      clearHoverScale();
      hover = null;
      canvasEl.style.cursor = 'grab';
    };

    canvasEl.addEventListener('pointermove', onPointerMove);
    canvasEl.addEventListener('pointerleave', onPointerLeave);

    // Presence locus — RF activity marker (not a fake human silhouette)
    const presenceGroup = new THREE.Group();
    scene.add(presenceGroup);
    const presenceRingMat = new THREE.MeshBasicMaterial({
      color: C.presence,
      transparent: true,
      opacity: 0,
      side: THREE.DoubleSide,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
    });
    const presenceRing = new THREE.Mesh(new THREE.RingGeometry(0.35, 0.48, 64), presenceRingMat);
    presenceRing.rotation.x = -Math.PI / 2;
    presenceGroup.add(presenceRing);
    const presenceCore = new THREE.Mesh(
      new THREE.CircleGeometry(0.22, 48),
      new THREE.MeshBasicMaterial({
        color: C.presence,
        transparent: true,
        opacity: 0,
        blending: THREE.AdditiveBlending,
        depthWrite: false,
      }),
    );
    presenceCore.rotation.x = -Math.PI / 2;
    presenceCore.position.y = 0.01;
    presenceGroup.add(presenceCore);
    const presenceLight = new THREE.PointLight(C.presence, 0, 4.5, 2);
    presenceLight.position.y = 0.9;
    presenceGroup.add(presenceLight);
    const locusCount = 120;
    const locusPos = new Float32Array(locusCount * 3);
    const locusAlpha = new Float32Array(locusCount);
    for (let i = 0; i < locusCount; i++) {
      const a = (i / locusCount) * Math.PI * 2;
      const r = 0.15 + (i % 7) * 0.06;
      locusPos[i * 3] = Math.cos(a) * r;
      locusPos[i * 3 + 1] = 0.04 + (i % 5) * 0.03;
      locusPos[i * 3 + 2] = Math.sin(a) * r;
      locusAlpha[i] = 0;
    }
    const locusGeo = new THREE.BufferGeometry();
    locusGeo.setAttribute('position', new THREE.BufferAttribute(locusPos, 3));
    locusGeo.setAttribute('alpha', new THREE.BufferAttribute(locusAlpha, 1));
    const locusMat = new THREE.ShaderMaterial({
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      uniforms: { uColor: { value: new THREE.Color(C.presence) } },
      vertexShader: `
        attribute float alpha;
        varying float vAlpha;
        void main() {
          vAlpha = alpha;
          vec4 mv = modelViewMatrix * vec4(position, 1.0);
          gl_PointSize = max(1.0, 4.5 * (90.0 / -mv.z));
          gl_Position = projectionMatrix * mv;
        }
      `,
      fragmentShader: `
        uniform vec3 uColor;
        varying float vAlpha;
        void main() {
          float d = length(gl_PointCoord - 0.5);
          if (d > 0.5) discard;
          gl_FragColor = vec4(uColor, vAlpha * smoothstep(0.5, 0.12, d));
        }
      `,
    });
    presenceGroup.add(new THREE.Points(locusGeo, locusMat));
    let presenceOpacity = 0;

    let latest: SensingUpdate | null = null;
    let closed = false;
    let ws: WebSocket | null = null;
    const clock = new THREE.Clock();

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
            presence = !!data.classification?.presence;
            motion = data.classification?.motion_level ?? '—';
            est = data.estimated_persons ?? null;
            classConf =
              data.classification?.confidence != null
                ? data.classification.confidence
                : null;
            hrBpm = gatedHeartRateBpm(data);
            brBpm = gatedBreathingBpm(data);
            vitalsQuality =
              data.vital_signs?.signal_quality != null
                ? data.vital_signs.signal_quality
                : null;
            breathBand =
              data.features?.breathing_band_power != null
                ? Number(data.features.breathing_band_power)
                : null;
            nodeCount = data.nodes?.length ?? 0;
          }
        } catch {
          /* ignore */
        }
      };
      ws.onclose = () => {
        connected = false;
        ws = null;
        if (!closed) setTimeout(connect, 1500);
      };
    };
    connect();

    const refreshNodes = () => {
      api
        .nodes()
        .then((n) => {
          nodeRest = new Map((n.nodes || []).map((row) => [row.node_id, row]));
          if (hoveredMesh) hover = buildHoverFor(hoveredMesh);
        })
        .catch(() => {});
    };
    refreshNodes();
    const nodePoll = setInterval(refreshNodes, 1000);

    let lanUpdatedAtMs: number | null = null;

    const refreshLan = () => {
      Promise.all([api.lanMesh(), api.lanClients()])
        .then(([meshRes, clientRes]) => {
          const err = (meshRes.error || clientRes.error || '').trim();
          meshSnapshot = meshRes.mesh || [];
          clientSnapshot = clientRes.clients || [];
          // Healthy feed = mesh present and no error string (empty "" is ok).
          decoFeedDown = !!err || meshSnapshot.length === 0;
          lanMeshCount = meshSnapshot.length;
          lanClientCount = clientSnapshot.length;
          const ts = clientRes.updated_at || meshRes.updated_at;
          lanUpdatedAtMs = ts ? Date.parse(ts) : Date.now();
          if (lanUpdatedAtMs && !Number.isNaN(lanUpdatedAtMs)) {
            lanAgeSec = Math.max(0, Math.round((Date.now() - lanUpdatedAtMs) / 1000));
          }
          placeDecos(meshSnapshot);
          placeClients(clientSnapshot);
          if (hoveredKey && hoveredMesh) hover = buildHoverFor(hoveredMesh);
        })
        .catch(() => {
          // Proxy missing / web server needs restart after astro.config proxy change.
          decoFeedDown = true;
          meshSnapshot = [];
          clientSnapshot = [];
          lanMeshCount = 0;
          lanClientCount = 0;
          lanAgeSec = null;
          placeDecos([]);
          placeClients([]);
        });
    };
    refreshLan();
    const lanPoll = setInterval(refreshLan, 2000);

    let raf = 0;
    const animate = () => {
      raf = requestAnimationFrame(animate);
      const elapsed = clock.getElapsedTime();
      const data = latest;

      routerLight.intensity = 0.35 + 0.15 * Math.sin(elapsed * 2.4);

      for (const w of wifiWaves) {
        const t = (elapsed * 0.55 + w.phase) % 5;
        const life = t / 5;
        w.mat.opacity = Math.max(0, 0.18 * (1 - life));
        const scale = 1 + life * 3.2;
        w.mesh.scale.set(scale, scale, 1);
      }

      // Signal field
      const field = data?.signal_field?.values;
      if (field) {
        const count = Math.min(field.length, fieldCount);
        for (let i = 0; i < count; i++) {
          const v = field[i] || 0;
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
          fieldColors[i * 3] = r;
          fieldColors[i * 3 + 1] = g;
          fieldColors[i * 3 + 2] = b;
        }
        fieldGeo.attributes.color.needsUpdate = true;
      }

      // Node markers from live nodes
      const liveIds = new Set((data?.nodes || []).map((n) => n.node_id));
      for (const [id, mesh] of nodeMarkers) {
        const mat = mesh.material as THREE.MeshStandardMaterial;
        const on = liveIds.has(id) || ((nodeRest.get(id)?.last_seen_ms ?? 99999) < 5000);
        mat.opacity = on ? 0.9 : 0.25;
        const boost = hoveredKey === `node-${id}` ? 0.35 : 0;
        mat.emissiveIntensity = on ? 0.7 + 0.2 * Math.sin(elapsed * 4 + id) + boost : 0.15;
      }

      // Keep tooltip fields fresh while hovering
      if (hoveredMesh) hover = buildHoverFor(hoveredMesh);

      // Smooth LAN client drift when they roam between Decos
      for (const [mac, orb] of clientMarkers) {
        const target = clientTargets.get(mac);
        if (!target) continue;
        orb.position.lerp(target, 0.12);
      }
      if (lanUpdatedAtMs && !Number.isNaN(lanUpdatedAtMs)) {
        lanAgeSec = Math.max(0, Math.round((Date.now() - lanUpdatedAtMs) / 1000));
      }

      // Presence locus at field peak / person estimate (floor ring, not a body)
      const isPresent = !!data?.classification?.presence;
      let px = 0,
        pz = 0;
      const person = data?.persons?.[0];
      if (person?.position) {
        px = person.position[0] || 0;
        pz = person.position[2] || 0;
      } else if (field) {
        let best = 0,
          bi = 0;
        for (let i = 0; i < field.length; i++) {
          if (field[i] > best) {
            best = field[i];
            bi = i;
          }
        }
        const ix = bi % gridSize;
        const iz = Math.floor(bi / gridSize);
        px = (ix - gridSize / 2) * 0.6;
        pz = (iz - gridSize / 2) * 0.5;
      }
      presenceOpacity += ((isPresent ? 1 : 0) - presenceOpacity) * 0.08;
      presenceGroup.position.x += (px - presenceGroup.position.x) * 0.06;
      presenceGroup.position.z += (pz - presenceGroup.position.z) * 0.06;
      presenceRingMat.opacity = 0.35 * presenceOpacity;
      (presenceCore.material as THREE.MeshBasicMaterial).opacity = 0.22 * presenceOpacity;
      presenceLight.intensity = 1.1 * presenceOpacity;
      presenceRing.scale.setScalar(1 + 0.08 * Math.sin(elapsed * 2.2));
      const alphaAttr = locusGeo.attributes.alpha as THREE.BufferAttribute;
      for (let i = 0; i < locusCount; i++) {
        const target = isPresent ? 0.22 + 0.1 * Math.sin(elapsed * 2.5 + i * 0.4) : 0;
        alphaAttr.array[i] += (target - Number(alphaAttr.array[i])) * 0.1;
      }
      alphaAttr.needsUpdate = true;

      controls.update();
      renderer.render(scene, camera);
    };
    animate();

    return () => {
      closed = true;
      cancelAnimationFrame(raf);
      clearInterval(nodePoll);
      clearInterval(lanPoll);
      canvasEl.removeEventListener('pointermove', onPointerMove);
      canvasEl.removeEventListener('pointerleave', onPointerLeave);
      ro.disconnect();
      ws?.close();
      renderer.dispose();
      scene.traverse((obj) => {
        if (obj instanceof THREE.Mesh || obj instanceof THREE.Points || obj instanceof THREE.LineSegments) {
          obj.geometry?.dispose?.();
          const m = obj.material;
          if (Array.isArray(m)) m.forEach((x) => x.dispose());
          else m?.dispose?.();
        }
      });
    };
  });
</script>

<div class="obs-wrap" bind:this={hostEl}>
  <canvas class="obs-canvas" bind:this={canvasEl}></canvas>
  <div class="obs-hud">
    <div class="row hud-chips">
      <SourceChip {source} {connected} />
      <span class={`chip fixed w-pres ${presence ? 'ok' : 'muted'}`}>
        <span class="k">pres</span><span class="v">{presence ? 'yes' : 'no'}</span>
      </span>
      <span class="chip muted fixed w-motion" title={motion}>
        <span class="k">mot</span><span class="v">{motion}</span>
      </span>
      <span
        class={`chip fixed w-bpm ${brBpm != null ? 'ok' : 'muted'}`}
        title="Breathing gated until still + confident FFT"
      >
        <span class="k">BR</span><span class="v">{brBpm ?? '—'}</span>
      </span>
      <span
        class={`chip fixed w-bpm ${hrBpm != null ? 'ok' : 'muted'}`}
        title="Heart rate needs stillness"
      >
        <span class="k">HR</span><span class="v">{hrBpm ?? '—'}</span>
      </span>
      <span class="chip muted fixed w-count">
        <span class="k">csi</span><span class="v">{nodeCount}</span>
      </span>
      <span class="chip muted fixed w-count">
        <span class="k">deco</span><span class="v">{lanMeshCount}</span>
      </span>
      <span
        class={`chip fixed w-lan ${decoFeedDown ? 'bad' : 'muted'}`}
        title={decoFeedDown ? 'Deco feed down' : lanAgeSec != null ? `snapshot ${lanAgeSec}s` : ''}
      >
        <span class="k">lan</span>
        <span class="v"
          >{decoFeedDown
            ? 'down'
            : `${lanClientCount}${lanAgeSec != null ? `·${lanAgeSec}s` : ''}`}</span
        >
      </span>
      <button type="button" class="chip muted fixed w-more hud-toggle" onclick={() => (hudOpen = !hudOpen)}>
        {hudOpen ? 'less' : 'more'}
      </button>
    </div>
    {#if hudOpen}
      <div class="row detail hud-chips">
        <span class="chip muted fixed w-count">
          <span class="k">est</span><span class="v">{est ?? '—'}</span>
        </span>
        <span class="chip muted fixed w-pct">
          <span class="k">sense</span
          ><span class="v">{classConf != null ? `${(classConf * 100).toFixed(0)}%` : '—'}</span>
        </span>
        <span class="chip muted fixed w-pct">
          <span class="k">breath</span
          ><span class="v">{breathBand != null ? breathBand.toFixed(1) : '—'}</span>
        </span>
        <span class="chip muted fixed w-pct">
          <span class="k">sq</span
          ><span class="v"
            >{vitalsQuality != null && presence ? `${(vitalsQuality * 100).toFixed(0)}%` : '—'}</span
          >
        </span>
      </div>
      <p class="obs-hint">
        Floor locus = RF presence peak (not an identified person). Hover markers · orbit · zoom.
      </p>
    {/if}
    <ul class="obs-legend">
      <li><span class="swatch csi"></span>CSI</li>
      <li><span class="swatch deco"></span>Deco</li>
      <li><span class="swatch lan"></span>Client (inward of AP · schematic)</li>
      <li><span class="swatch presence"></span>Presence</li>
    </ul>
  </div>
  {#if hover}
    <div
      class="node-tip"
      style={`left:${Math.min(hover.x, (hostEl?.clientWidth ?? 320) - 200)}px;top:${Math.min(hover.y, (hostEl?.clientHeight ?? 200) - 160)}px;`}
      role="tooltip"
    >
      {#if hover.kind === 'esp32'}
        <div class="node-tip-title">
          Node {hover.id}
          <span class={`chip ${hover.online ? 'ok' : 'bad'}`}>{hover.online ? 'online' : 'offline'}</span>
        </div>
        <dl class="node-tip-grid">
          <dt>RSSI</dt><dd class="mono">{hover.rssi}</dd>
          <dt>Last seen</dt><dd class="mono">{hover.lastSeen}</dd>
          <dt>Motion</dt><dd>{hover.motion}</dd>
          <dt>Persons</dt><dd class="mono">{hover.persons}</dd>
          <dt>Subcarriers</dt><dd class="mono">{hover.subcarriers}</dd>
          <dt>Variance</dt><dd class="mono">{hover.variance}</dd>
          <dt>Confidence</dt><dd class="mono">{hover.conf}</dd>
        </dl>
      {:else if hover.kind === 'deco'}
        <div class="node-tip-title">
          {hover.name}
          <span class={`chip ${hover.online ? 'ok' : 'bad'}`}>{hover.online ? 'online' : 'offline'}</span>
        </div>
        <dl class="node-tip-grid">
          <dt>Role</dt><dd>{hover.role}</dd>
          <dt>IP</dt><dd class="mono">{hover.ip}</dd>
          <dt>MAC</dt><dd class="mono">{hover.mac}</dd>
        </dl>
      {:else}
        <div class="node-tip-title">
          {hover.name}
          <span class={`chip ${hover.online ? 'ok' : 'bad'}`}>{hover.online ? 'online' : 'offline'}</span>
        </div>
        <dl class="node-tip-grid">
          <dt>IP</dt><dd class="mono">{hover.ip}</dd>
          <dt>MAC</dt><dd class="mono">{hover.mac}</dd>
          <dt>Band</dt><dd>{hover.band}</dd>
          <dt>Deco</dt><dd>{hover.deco}</dd>
          <dt>Rates</dt><dd class="mono">{hover.rates}</dd>
        </dl>
      {/if}
    </div>
  {/if}
</div>

<style>
  .obs-wrap {
    position: relative;
    width: 100%;
    height: calc(100vh - 3.4rem);
    min-height: 420px;
    border-radius: 10px;
    overflow: hidden;
    border: 1px solid var(--border);
    background: #080c14;
  }
  .obs-canvas {
    width: 100%;
    height: 100%;
    display: block;
    cursor: grab;
  }
  .obs-hud {
    position: absolute;
    left: 0.85rem;
    top: 0.85rem;
    right: 0.85rem;
    pointer-events: none;
    z-index: 2;
  }
  .obs-hud :global(.chip),
  .hud-toggle {
    pointer-events: auto;
  }
  .hud-chips {
    flex-wrap: nowrap;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .hud-chips::-webkit-scrollbar {
    display: none;
  }
  .obs-hud :global(.chip),
  .obs-hud .chip {
    font-variant-numeric: tabular-nums;
  }
  .obs-hud .chip.fixed {
    display: inline-flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.4rem;
    box-sizing: border-box;
    flex: 0 0 auto;
  }
  .obs-hud .chip.fixed .k {
    opacity: 0.72;
    flex: 0 0 auto;
  }
  .obs-hud .chip.fixed .v {
    min-width: 1.6ch;
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .obs-hud .chip.w-pres {
    width: 6.4rem;
  }
  .obs-hud .chip.w-motion {
    width: 7.2rem;
  }
  .obs-hud .chip.w-motion .v {
    min-width: 4.2ch;
  }
  .obs-hud .chip.w-bpm {
    width: 5.2rem;
  }
  .obs-hud .chip.w-bpm .v {
    min-width: 2.5ch;
  }
  .obs-hud .chip.w-count {
    width: 5.4rem;
  }
  .obs-hud .chip.w-count .v {
    min-width: 2ch;
  }
  .obs-hud .chip.w-lan {
    width: 7.6rem;
  }
  .obs-hud .chip.w-lan .v {
    min-width: 4.5ch;
  }
  .obs-hud .chip.w-pct {
    width: 7rem;
  }
  .obs-hud .chip.w-pct .v {
    min-width: 3.2ch;
  }
  .obs-hud .chip.w-more {
    width: 3.6rem;
    justify-content: center;
  }
  .obs-hud :global(.chip:first-child) {
    min-width: 8.2rem;
    justify-content: center;
  }
  /* <button> defaults fight .chip — force same dark pill as the rest of the HUD */
  .obs-hud button.chip.hud-toggle {
    appearance: none;
    -webkit-appearance: none;
    cursor: pointer;
    margin: 0;
    font: inherit;
    font-family: var(--mono);
    font-size: 0.75rem;
    font-weight: 600;
    line-height: inherit;
    color: var(--muted);
    background: rgba(12, 15, 20, 0.55);
    border: 1px solid var(--border);
    border-radius: 999px;
    box-shadow: none;
    outline: none;
  }
  .obs-hud button.chip.hud-toggle:hover {
    color: var(--text);
    border-color: #3a4a5c;
    background: rgba(20, 26, 34, 0.75);
  }
  .obs-hud button.chip.hud-toggle:focus-visible {
    border-color: rgba(61, 156, 240, 0.45);
    box-shadow: 0 0 0 1px rgba(61, 156, 240, 0.25);
  }
  .row.detail {
    margin-top: 0.35rem;
  }
  .obs-hint {
    margin: 0.45rem 0 0;
    color: var(--muted);
    font-size: 0.75rem;
    max-width: 28rem;
    line-height: 1.35;
    text-shadow: 0 1px 2px #000a;
  }
  .obs-legend {
    list-style: none;
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin: 0.45rem 0 0;
    padding: 0;
    font-size: 0.72rem;
    color: var(--muted);
    text-shadow: 0 1px 2px #000a;
  }
  .obs-legend li {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }
  .swatch {
    width: 0.65rem;
    height: 0.65rem;
    border-radius: 999px;
    display: inline-block;
  }
  .swatch.csi {
    background: #3d9cf0;
  }
  .swatch.deco {
    background: #f0a030;
    border-radius: 2px;
  }
  .swatch.lan {
    background: #a0b8d0;
  }
  .swatch.presence {
    background: #5ee0a8;
    box-shadow: 0 0 6px #5ee0a866;
  }
  .chip.bad {
    background: rgba(180, 40, 40, 0.35);
    color: #ffb0b0;
  }
  .node-tip {
    position: absolute;
    z-index: 3;
    min-width: 180px;
    max-width: 220px;
    padding: 0.65rem 0.75rem;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: rgba(12, 15, 20, 0.92);
    backdrop-filter: blur(6px);
    pointer-events: none;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
  }
  .node-tip-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    font-weight: 650;
    font-size: 0.92rem;
    margin-bottom: 0.45rem;
  }
  .node-tip-grid {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.2rem 0.65rem;
    margin: 0;
    font-size: 0.78rem;
  }
  .node-tip-grid dt {
    margin: 0;
    color: var(--muted);
  }
  .node-tip-grid dd {
    margin: 0;
    text-align: right;
  }
</style>
