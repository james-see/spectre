import { FilesetResolver, PoseLandmarker, type NormalizedLandmark } from '@mediapipe/tasks-vision';
import { COCO_KP_NAMES } from './api';

/** MediaPipe Pose landmark indices → COCO-17 names we train on. */
const MP_TO_COCO: Array<{ mp: number; name: (typeof COCO_KP_NAMES)[number] }> = [
  { mp: 0, name: 'nose' },
  { mp: 2, name: 'left_eye' },
  { mp: 5, name: 'right_eye' },
  { mp: 7, name: 'left_ear' },
  { mp: 8, name: 'right_ear' },
  { mp: 11, name: 'left_shoulder' },
  { mp: 12, name: 'right_shoulder' },
  { mp: 13, name: 'left_elbow' },
  { mp: 14, name: 'right_elbow' },
  { mp: 15, name: 'left_wrist' },
  { mp: 16, name: 'right_wrist' },
  { mp: 23, name: 'left_hip' },
  { mp: 24, name: 'right_hip' },
  { mp: 25, name: 'left_knee' },
  { mp: 26, name: 'right_knee' },
  { mp: 27, name: 'left_ankle' },
  { mp: 28, name: 'right_ankle' },
];

export type TeacherKeypoint = {
  name: string;
  x: number;
  y: number;
  z: number;
  confidence: number;
};

export type TeacherPoseSample = {
  t_ms: number;
  keypoints: TeacherKeypoint[];
  source: 'mediapipe';
  video_w?: number;
  video_h?: number;
};

let landmarker: PoseLandmarker | null = null;
let loading: Promise<PoseLandmarker> | null = null;

export async function getPoseLandmarker(): Promise<PoseLandmarker> {
  if (landmarker) return landmarker;
  if (!loading) {
    loading = (async () => {
      const vision = await FilesetResolver.forVisionTasks(
        'https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@0.10.18/wasm',
      );
      landmarker = await PoseLandmarker.createFromOptions(vision, {
        baseOptions: {
          modelAssetPath:
            'https://storage.googleapis.com/mediapipe-models/pose_landmarker/pose_landmarker_lite/float16/1/pose_landmarker_lite.task',
          delegate: 'GPU',
        },
        runningMode: 'VIDEO',
        numPoses: 1,
      });
      return landmarker;
    })();
  }
  return loading;
}

export function landmarksToCoco(
  landmarks: NormalizedLandmark[],
  t_ms: number,
  video_w?: number,
  video_h?: number,
): TeacherPoseSample {
  const keypoints: TeacherKeypoint[] = MP_TO_COCO.map(({ mp, name }) => {
    const lm = landmarks[mp];
    const vis = lm?.visibility ?? lm?.presence ?? 0;
    return {
      name,
      x: lm ? Math.min(1, Math.max(0, lm.x)) : 0,
      y: lm ? Math.min(1, Math.max(0, lm.y)) : 0,
      z: lm?.z ?? 0,
      confidence: typeof vis === 'number' ? vis : 0,
    };
  });
  return { t_ms, keypoints, source: 'mediapipe', video_w, video_h };
}

export async function detectPoseOnVideo(
  video: HTMLVideoElement,
  timestampMs: number,
): Promise<TeacherPoseSample | null> {
  const lm = await getPoseLandmarker();
  const result = lm.detectForVideo(video, timestampMs);
  const pose = result.landmarks?.[0];
  if (!pose?.length) return null;
  return landmarksToCoco(pose, timestampMs, video.videoWidth, video.videoHeight);
}
