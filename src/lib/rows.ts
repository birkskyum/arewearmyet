// Turns models into plain rows for the table island: every label, hover text and sort rank is
// worked out here at build time, so the browser only receives what it shows
import status from '../data/status.json';
import { ANNOUNCED, GENERATION, acpiCell, deviceTreeCell, firmwareCell, idsCell, nvidiaLinuxCell, nvidiaOmarchyCell, omarchyCell, type Cell, type Model, type NvidiaModel } from './labels';

export type CellData = Pick<Cell, 'label' | 'tone' | 'title' | 'href' | 'rank'> | { text: string; rank: number };
export type TableRow = {
  id: string;
  vendor: string;
  name: string;
  href: string | null;
  announced: string | null;
  subtitle: string;
  generation: string;
  outcome: 'works' | 'should' | 'problems' | 'notyet';
  variants: { text: string; tone: 'good' | 'close' | 'wanted' | 'muted'; state: string }[];
  notes: string | null;
  search: string;
  cells: Record<string, CellData>;
};

const year = (released: string | number | null) => (released ? String(released).slice(0, 4) : '');
const plain = ({ label, tone, title, href, rank }: Cell) => ({ label, tone, title, href, rank });
const announcedTitle = (statusText: string | null) => (statusText ? `${ANNOUNCED.meaning} The vendor said: ${statusText}.` : null);

const VARIANT: Record<string, { tone: TableRow['variants'][number]['tone']; state: string }> = {
  works: { tone: 'good', state: 'works' },
  should: { tone: 'close', state: 'should work' },
  problems: { tone: 'wanted', state: 'problems' },
  manual: { tone: 'close', state: 'not picked' },
  newer: { tone: 'close', state: 'newer kernel' },
  none: { tone: 'muted', state: 'no device tree' },
};

export function snapdragonRows(): TableRow[] {
  return status.models.map((model: Model) => ({
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    href: model.sources[0] ?? null,
    announced: announcedTitle(model.status),
    subtitle: [GENERATION[model.generation], year(model.released), model.chips.join(', '), model.modelNumbers.slice(0, 3).join(', ')].filter(Boolean).join(' · '),
    generation: model.generation,
    outcome: (['works', 'should', 'problems'].includes(model.outcome) ? model.outcome : 'notyet') as TableRow['outcome'],
    variants: model.variants.length > 1 ? model.variants.map((v) => ({ text: [v.model, v.chip].filter(Boolean).join(' · '), ...VARIANT[v.outcome] })) : [],
    notes: model.notes,
    search: [model.vendor, model.name, ...model.modelNumbers, ...model.chips, ...model.variants.map((v) => `${v.model ?? ''} ${v.chip ?? ''}`)].join(' ').toLowerCase(),
    cells: {
      omarchy: plain(omarchyCell(model)),
      'device-tree': plain(deviceTreeCell(model)),
      ids: plain(idsCell(model)),
      firmware: plain(firmwareCell(model)),
      acpi: plain(acpiCell(model)),
    },
  }));
}

const NVIDIA_OUTCOME: Record<string, TableRow['outcome']> = { works: 'works', expected: 'should', problems: 'problems', untested: 'notyet' };

export function nvidiaRows(): TableRow[] {
  return status.nvidia.map((model: NvidiaModel) => ({
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    href: model.sources[0] ?? null,
    announced: announcedTitle(model.status),
    subtitle: [GENERATION[model.generation], year(model.released)].filter(Boolean).join(' · '),
    generation: model.generation,
    outcome: NVIDIA_OUTCOME[model.omarchy] ?? 'notyet',
    variants: [],
    notes: null,
    search: `${model.vendor} ${model.name}`.toLowerCase(),
    cells: {
      omarchy: plain(nvidiaOmarchyCell(model)),
      linux: plain(nvidiaLinuxCell(model)),
      notes: { text: model.notes ?? '', rank: model.notes ? 0 : 1 },
    },
  }));
}
