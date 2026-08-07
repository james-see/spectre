import { POSE_CONF_THRESHOLD, type PersonDetection, type PoseKeypoint } from './api';

/** COCO-17 limbs used across Pose + Camera tabs. */
export const POSE_BONES: [string, string][] = [
  ['left_shoulder', 'right_shoulder'],
  ['left_shoulder', 'left_elbow'],
  ['left_elbow', 'left_wrist'],
  ['right_shoulder', 'right_elbow'],
  ['right_elbow', 'right_wrist'],
  ['left_shoulder', 'left_hip'],
  ['right_shoulder', 'right_hip'],
  ['left_hip', 'right_hip'],
  ['left_hip', 'left_knee'],
  ['left_knee', 'left_ankle'],
  ['right_hip', 'right_knee'],
  ['right_knee', 'right_ankle'],
  ['nose', 'left_shoulder'],
  ['nose', 'right_shoulder'],
];

export type PoseDrawOpts = {
  minConf?: number;
  /** Clear + fill before draw. Default true. */
  clear?: boolean;
  fillStyle?: string | null;
  /** Inset padding inside the destination rect. */
  pad?: number;
  boneColor?: string;
  jointColor?: (conf: number) => string;
  lineWidth?: number;
  jointRadius?: number;
  /** Require both bone endpoints above minConf (honest default). */
  bothEnds?: boolean;
  /** Logical draw size when canvas buffer is DPR-scaled (after setTransform). */
  destW?: number;
  destH?: number;
};

function trustedKeypoints(persons: PersonDetection[], minConf: number): PoseKeypoint[] {
  return persons.flatMap((p) => p.keypoints || []).filter((k) => k.confidence >= minConf);
}

/** Auto-fit unbounded model coords into a destination rect. */
export function fitMap(
  persons: PersonDetection[],
  destW: number,
  destH: number,
  minConf = POSE_CONF_THRESHOLD,
  pad = 40,
) {
  const all = trustedKeypoints(persons, minConf);
  let minX = Infinity,
    minY = Infinity,
    maxX = -Infinity,
    maxY = -Infinity;
  for (const k of all) {
    minX = Math.min(minX, k.x);
    minY = Math.min(minY, k.y);
    maxX = Math.max(maxX, k.x);
    maxY = Math.max(maxY, k.y);
  }
  if (!Number.isFinite(minX)) {
    return (_x: number, _y: number) => ({ x: destW / 2, y: destH / 2 });
  }
  const spanX = Math.max(maxX - minX, 1);
  const spanY = Math.max(maxY - minY, 1);
  const scale = Math.min((destW - pad * 2) / spanX, (destH - pad * 2) / spanY);
  const ox = pad + (destW - pad * 2 - spanX * scale) / 2;
  const oy = pad + (destH - pad * 2 - spanY * scale) / 2;
  return (x: number, y: number) => ({
    x: ox + (x - minX) * scale,
    y: oy + (y - minY) * scale,
  });
}

export function drawPoseSkeleton(
  ctx: CanvasRenderingContext2D,
  persons: PersonDetection[],
  opts: PoseDrawOpts = {},
) {
  const minConf = opts.minConf ?? POSE_CONF_THRESHOLD;
  const bothEnds = opts.bothEnds ?? true;
  const w = opts.destW ?? ctx.canvas.width;
  const h = opts.destH ?? ctx.canvas.height;

  if (opts.clear !== false) {
    ctx.clearRect(0, 0, w, h);
    if (opts.fillStyle) {
      ctx.fillStyle = opts.fillStyle;
      ctx.fillRect(0, 0, w, h);
    }
  }

  const trusted = trustedKeypoints(persons, minConf);
  if (trusted.length === 0) return false;

  const mapPt = fitMap(persons, w, h, minConf, opts.pad ?? 40);
  const boneColor = opts.boneColor ?? '#3d9cf0';
  const jointColor =
    opts.jointColor ?? ((c: number) => `rgba(62, 207, 142, ${0.4 + c * 0.6})`);
  const lineWidth = opts.lineWidth ?? 2.5;
  const jointRadius = opts.jointRadius ?? 4;

  for (const p of persons) {
    const map = new Map((p.keypoints || []).map((k) => [k.name, k]));
    ctx.strokeStyle = boneColor;
    ctx.lineWidth = lineWidth;
    ctx.lineCap = 'round';
    for (const [a, b] of POSE_BONES) {
      const ka = map.get(a);
      const kb = map.get(b);
      if (!ka || !kb) continue;
      if (bothEnds) {
        if (ka.confidence < minConf || kb.confidence < minConf) continue;
      } else if (ka.confidence < minConf && kb.confidence < minConf) {
        continue;
      }
      const pa = mapPt(ka.x, ka.y);
      const pb = mapPt(kb.x, kb.y);
      ctx.beginPath();
      ctx.moveTo(pa.x, pa.y);
      ctx.lineTo(pb.x, pb.y);
      ctx.stroke();
    }
    for (const k of p.keypoints || []) {
      if (k.confidence < minConf) continue;
      const pt = mapPt(k.x, k.y);
      ctx.fillStyle = jointColor(k.confidence);
      ctx.beginPath();
      ctx.arc(pt.x, pt.y, jointRadius, 0, Math.PI * 2);
      ctx.fill();
    }
  }
  return true;
}
