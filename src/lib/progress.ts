// The bars at the top of the home page count every model on it, and its buttons narrow them to one kind
// of chip. Every model's state in each bar is worked out here at build time, so the island only counts
import status from '../data/status.json';
import { PARTS, modelParts } from './parts';

export type ProgressGroup = { key: string; label: string; uefi: boolean };
// `uefi` marks the chips Bring-up has bars for. The pieces those bars count, a device tree, hardware IDs and
// firmware, are what a Snapdragon PC that starts through UEFI needs, and NVIDIA's and Google's machines don't
export const PROGRESS_GROUPS: ProgressGroup[] = [
  { key: 'x1', label: 'Snapdragon X', uefi: true },
  { key: 'x2', label: 'Snapdragon X2', uefi: true },
  { key: 'older', label: 'Snapdragon 7c, 8cx, 835 and 850', uefi: true },
  { key: 'c', label: 'Snapdragon C', uefi: true },
  { key: 'n1x', label: 'NVIDIA N1x', uefi: false },
  { key: 'gb10', label: 'NVIDIA GB10', uefi: false },
  { key: 'google', label: 'Chromebooks and Googlebooks', uefi: false },
];

export type PieceKind = 'upstream' | 'added' | 'pull' | 'available' | 'none';
export type ProgressModel = {
  name: string;
  group: string;
  // The table the model is in, so its cells search that table and no other
  prefix: string;
  anchor: string;
  // One per part, null where the model doesn't have the part
  parts: ({ kind: string; tip: string } | null)[];
  // One per piece (device tree, hardware IDs, firmware, all three), for the models Bring-up counts
  pieces: { kind: PieceKind; channel: string | null }[] | null;
};

const OLDER = ['8cx', '8cx-gen1', '850', '835', '7c'];
const PIECES = ['deviceTree', 'ids', 'firmware', 'all'] as const;
// The Snapdragon X, older and entry tables each have their summary, which together cover every UEFI model
const summaries = [status.summary.byPiece, status.summary.byPieceOlder, status.summary.byPieceEntry];
const pieceKind = (piece: (typeof PIECES)[number], id: string): PieceKind => {
  for (const summary of summaries) {
    const { models } = summary[piece];
    if (models.fromUpstream.includes(id)) return 'upstream';
    if (models.added.includes(id)) return 'added';
    if (models.pull.includes(id)) return 'pull';
    if (models.available.includes(id)) return 'available';
  }
  return 'none';
};

const parts = (model: Parameters<typeof modelParts>[0]) =>
  modelParts(model).map((part) => (part.state === 'absent' ? null : { kind: part.key.startsWith('omarchy-') ? part.key : 'unknown', tip: part.title }));

const where = (group: string, boot: string) => {
  if (group === 'x1' || group === 'x2') return { prefix: '', anchor: 'snapdragon' };
  if (group === 'older') return { prefix: '8cx-', anchor: 'snapdragon-7c-8cx-835-850' };
  if (group === 'c') return { prefix: 'c-', anchor: 'snapdragon-c' };
  if (group === 'n1x' || group === 'gb10') return { prefix: 'nvidia-', anchor: 'nvidia' };
  return boot === 'android' ? { prefix: 'googlebook-', anchor: 'googlebooks' } : { prefix: 'chromebook-', anchor: 'chromebooks' };
};

export function progressModels(): ProgressModel[] {
  const snapdragon = status.models.map((model) => {
    const uefi = model.boot === 'uefi';
    const group = !uefi ? 'google' : OLDER.includes(model.generation) ? 'older' : model.generation;
    const channels = [model.omarchy.deviceTree, model.omarchy.ids, model.omarchy.firmwareChannel, null];
    return {
      name: `${model.vendor} ${model.name}`,
      group,
      ...where(group, model.boot),
      parts: parts(model),
      pieces: uefi ? PIECES.map((piece, i) => ({ kind: pieceKind(piece, model.id), channel: channels[i] })) : null,
    };
  });
  const nvidia = status.nvidia.map((model) => ({ name: `${model.vendor} ${model.name}`, group: model.generation, ...where(model.generation, 'uefi'), parts: parts(model), pieces: null }));
  return [...snapdragon, ...nvidia];
}

export const PROGRESS_PARTS = PARTS.map(({ title, question }) => ({ title, question }));
