export type Health = {
  status: string;
  source?: string;
  clients?: number;
  tick?: number;
};

export type NodeRow = {
  node_id: number;
  rssi_dbm: number;
  last_seen_ms?: number;
  motion_level?: string;
  person_count?: number;
  status?: string;
};

export type VitalSigns = {
  breathing_rate_bpm?: number | null;
  heart_rate_bpm?: number | null;
  breathing_confidence?: number;
  heartbeat_confidence?: number;
  signal_quality?: number;
};

export type SensingUpdate = {
  type?: string;
  msg_type?: string;
  source?: string;
  tick?: number;
  timestamp?: number;
  estimated_persons?: number | null;
  features?: Record<string, number>;
  classification?: {
    presence?: boolean;
    motion_level?: string;
    confidence?: number;
  };
  vital_signs?: VitalSigns | null;
  nodes?: Array<{
    node_id: number;
    rssi_dbm: number;
    subcarrier_count?: number;
    position?: number[];
    /** Per-subcarrier CSI amplitudes when present on the wire. */
    amplitude?: number[];
  }>;
  node_features?: Array<{
    node_id: number;
    rssi_dbm?: number;
    stale?: boolean;
    features?: Record<string, number>;
    classification?: {
      presence?: boolean;
      motion_level?: string;
      confidence?: number;
    };
  }>;
  persons?: PersonDetection[];
  pose_keypoints?: number[][];
  signal_field?: {
    grid_size?: number[];
    values?: number[];
  };
};

/** FFT vitals are experimental — only surface when stillness + confidence look real. */
export function gatedBreathingBpm(update: SensingUpdate | null | undefined): number | null {
  if (!update?.classification?.presence) return null;
  const vs = update.vital_signs;
  const bpm = vs?.breathing_rate_bpm;
  const conf = vs?.breathing_confidence ?? 0;
  if (bpm == null || !(bpm >= 6 && bpm <= 30) || conf < 0.35) return null;
  return Math.round(bpm);
}

export function gatedHeartRateBpm(update: SensingUpdate | null | undefined): number | null {
  if (!update?.classification?.presence) return null;
  const motion = (update.classification?.motion_level ?? '').toLowerCase();
  // Gross motion ruins HR SNR — require still / low motion.
  if (motion.includes('active') || motion.includes('moving')) return null;
  const vs = update.vital_signs;
  const bpm = vs?.heart_rate_bpm;
  const conf = vs?.heartbeat_confidence ?? 0;
  if (bpm == null || !(bpm >= 45 && bpm <= 120) || conf < 0.5) return null;
  return Math.round(bpm);
}

export type PoseKeypoint = {
  name: string;
  x: number;
  y: number;
  z?: number;
  confidence: number;
};

export type PersonDetection = {
  id: number;
  confidence: number;
  bbox?: { x: number; y: number; width: number; height: number };
  keypoints?: PoseKeypoint[];
  position?: [number, number, number] | number[];
  motion_score?: number;
  pose?: string;
};

export type PoseCurrent = {
  persons: PersonDetection[];
  total_persons?: number;
  source?: string;
  pose_source?: string;
};

export type DecoMeshNode = {
  id: string;
  name: string;
  mac: string;
  ip: string;
  role: 'master' | 'satellite' | string;
  online: boolean;
};

export type DecoLanClient = {
  name: string;
  mac: string;
  ip: string;
  online: boolean;
  band?: string;
  connection_type?: string;
  deco_mac: string;
  deco_name?: string;
  up_rate_kbps?: number;
  down_rate_kbps?: number;
};

async function getJson<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { Accept: 'application/json' } });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(`${path} → ${res.status} ${text.slice(0, 160)}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  health: () => getJson<Health>('/health'),
  nodes: () => getJson<{ nodes: NodeRow[]; total: number }>('/api/v1/nodes'),
  sensingLatest: () => getJson<SensingUpdate>('/api/v1/sensing/latest'),
  poseCurrent: () => getJson<PoseCurrent>('/api/v1/pose/current'),
  models: () => getJson<{ models: Array<Record<string, unknown>>; total: number }>('/api/v1/models'),
  activeModel: () => getJson<{ active: Record<string, unknown> | null }>('/api/v1/models/active'),
  loadModel: (id: string) =>
    fetch('/api/v1/models/load', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id }),
    }).then(async (r) => {
      const j = await r.json();
      if (!r.ok) throw new Error(j.error || r.statusText);
      return j;
    }),
  unloadModel: () =>
    fetch('/api/v1/models/unload', { method: 'POST' }).then((r) => r.json()),
  listRecordings: () =>
    getJson<{ recordings?: Array<Record<string, unknown>>; total?: number } | Array<Record<string, unknown>>>(
      '/api/v1/recording/list',
    ),
  startRecording: (body: { session_name: string; label: string }) =>
    fetch('/api/v1/recording/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(async (r) => {
      const j = await r.json();
      if (!r.ok || j.success === false) throw new Error(j.error || j.message || r.statusText);
      return j as { success?: boolean; recording_id?: string; session_name?: string; label?: string };
    }),
  stopRecording: () =>
    fetch('/api/v1/recording/stop', { method: 'POST' }).then((r) => r.json()),
  appendRecordingPose: (id: string, samples: Array<Record<string, unknown>>) =>
    fetch(`/api/v1/recording/${encodeURIComponent(id)}/pose`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ samples }),
    }).then(async (r) => {
      const j = await r.json();
      if (!r.ok || j.success === false) throw new Error(j.error || j.message || r.statusText);
      return j as { success?: boolean; written?: number };
    }),
  deleteRecording: (id: string) =>
    fetch(`/api/v1/recording/${encodeURIComponent(id)}`, { method: 'DELETE' }).then((r) => r.json()),
  createMmfiFixture: () =>
    fetch('/api/v1/datasets/mmfi/fixture', { method: 'POST' }).then(async (r) => {
      const j = await r.json();
      if (!r.ok || j.success === false) throw new Error(j.error || j.message || r.statusText);
      return j as { success?: boolean; created?: string[]; note?: string };
    }),
  trainStatus: () => getJson<Record<string, unknown>>('/api/v1/train/status'),
  trainStart: (body: {
    dataset_ids: string[];
    config: {
      epochs: number;
      batch_size: number;
      learning_rate: number;
      weight_decay?: number;
      early_stopping_patience?: number;
      warmup_epochs?: number;
      pretrained_rvf?: string | null;
      lora_profile?: string | null;
      allow_heuristic_targets?: boolean;
    };
  }) =>
    fetch('/api/v1/train/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(async (r) => {
      const j = await r.json();
      if (!r.ok || j?.status === 'error') {
        throw new Error(j?.message || j?.detail || j?.error || r.statusText);
      }
      return j;
    }),
  trainStop: () => fetch('/api/v1/train/stop', { method: 'POST' }).then((r) => r.json()),
  adaptiveStatus: () => getJson<Record<string, unknown>>('/api/v1/adaptive/status'),
  adaptiveTrain: (body: { dataset_ids?: string[] } = {}) =>
    fetch('/api/v1/adaptive/train', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(async (r) => {
      const j = await r.json();
      if (!r.ok) throw new Error(j.error || j.message || r.statusText);
      return j as Record<string, unknown> & { success?: boolean; error?: string };
    }),
  adaptiveUnload: () =>
    fetch('/api/v1/adaptive/unload', { method: 'POST' }).then(async (r) => {
      const j = await r.json();
      if (!r.ok) throw new Error(j.error || j.message || r.statusText);
      return j;
    }),
  lanMesh: () =>
    getJson<{ mesh: DecoMeshNode[]; error?: string; updated_at?: string }>('/api/lan/mesh'),
  lanClients: () =>
    getJson<{ clients: DecoLanClient[]; error?: string; updated_at?: string }>('/api/lan/clients'),
};

export function sensingWsUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  // Dev proxy: same host, /ws → :3001
  return `${proto}//${location.host}/ws/sensing`;
}

/** Max keypoint confidence across persons (0 if none). */
export function maxKeypointConfidence(persons: PersonDetection[] | undefined): number {
  let m = 0;
  for (const p of persons || []) {
    for (const k of p.keypoints || []) {
      if (typeof k.confidence === 'number' && k.confidence > m) m = k.confidence;
    }
  }
  return m;
}

export const POSE_CONF_THRESHOLD = 0.2;

/** COCO-17 names matching sensing-server / pose_keypoints row order. */
export const COCO_KP_NAMES = [
  'nose',
  'left_eye',
  'right_eye',
  'left_ear',
  'right_ear',
  'left_shoulder',
  'right_shoulder',
  'left_elbow',
  'right_elbow',
  'left_wrist',
  'right_wrist',
  'left_hip',
  'right_hip',
  'left_knee',
  'right_knee',
  'left_ankle',
  'right_ankle',
] as const;

/** Build a PersonDetection from model `pose_keypoints` rows `[x,y,z,conf]`. */
export function personFromPoseKeypoints(
  kps: number[][],
  opts?: { id?: number; confidence?: number },
): PersonDetection {
  const keypoints: PoseKeypoint[] = kps.slice(0, 17).map((kp, i) => ({
    name: COCO_KP_NAMES[i] || `kp_${i}`,
    x: Number(kp[0]) || 0,
    y: Number(kp[1]) || 0,
    z: Number(kp[2]) || 0,
    confidence: Number(kp[3]) || 0,
  }));
  return {
    id: opts?.id ?? 1,
    confidence: opts?.confidence ?? 0.9,
    keypoints,
  };
}

/**
 * Prefer REST pose/current persons; if tracker stripped joint confidence, restore
 * from sensing `pose_keypoints[*][3]` (same wire the server already emits).
 */
export function hydrateDrawablePose(
  pose: PoseCurrent | null | undefined,
  sensing: SensingUpdate | null | undefined,
): PoseCurrent {
  const raw = sensing?.pose_keypoints;
  let persons = pose?.persons?.length ? [...pose.persons] : sensing?.persons ? [...sensing.persons] : [];
  const rawUsable = !!raw?.length && raw.some((kp) => (kp[3] ?? 0) > 1e-6);

  if (rawUsable && raw && (!persons.length || maxKeypointConfidence(persons) < POSE_CONF_THRESHOLD)) {
    persons = [
      personFromPoseKeypoints(raw, {
        id: persons[0]?.id ?? 1,
        confidence: sensing?.classification?.confidence ?? persons[0]?.confidence ?? 0.9,
      }),
    ];
  }

  const hasModel =
    rawUsable ||
    pose?.pose_source === 'model_inference' ||
    pose?.pose_source === 'densepose';

  return {
    persons,
    total_persons: persons.length,
    source: pose?.source ?? sensing?.source,
    pose_source: hasModel
      ? pose?.pose_source === 'densepose'
        ? 'densepose'
        : 'model_inference'
      : (pose?.pose_source ?? 'unavailable'),
  };
}

/** Poll pose + sensing and return a UI-drawable PoseCurrent. */
export async function fetchDrawablePose(): Promise<PoseCurrent> {
  const [pose, sensing] = await Promise.all([
    api.poseCurrent().catch(() => null),
    api.sensingLatest().catch(() => null),
  ]);
  return hydrateDrawablePose(pose, sensing);
}
