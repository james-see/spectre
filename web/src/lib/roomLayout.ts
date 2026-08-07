/** Shared room geometry for Observatory / Field (scene units). */

/** Room box footprint (matches Observatory wireframe). */
export const ROOM_SIZE = { w: 12, d: 10, h: 3.6 };

/** Approximate feet per scene unit (12-unit width ≈ 36 ft). */
export const FT_PER_UNIT = 3;

/**
 * CSI node marker slots — UI layout only.
 * Do not use sensing-server `nodes[].position` (currently identical placeholders).
 */
export const CSI_NODE_SLOTS: Record<number, { x: number; y: number; z: number }> = {
  1: { x: -5, y: 1.05, z: -4 },
  2: { x: 5, y: 1.05, z: -4 },
  3: { x: 0, y: 1.05, z: 4 },
};

export const CSI_NODE_IDS = [1, 2, 3] as const;

export const NODE_COLORS = [0x3d9cf0, 0xe6b84d, 0x3ecf8e, 0xef6b6b];

/** Signal-field grid mapping used by Observatory + Field floor heat. */
export const FIELD_GRID = 20;
export const FIELD_CELL_X = 0.6;
export const FIELD_CELL_Z = 0.5;

export function fieldCellWorld(ix: number, iz: number, grid = FIELD_GRID) {
  return {
    x: (ix - grid / 2) * FIELD_CELL_X,
    z: (iz - grid / 2) * FIELD_CELL_Z,
  };
}
