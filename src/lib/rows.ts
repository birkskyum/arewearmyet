// Turns models into plain rows for the table island: every label, hover text and sort rank is
// worked out here at build time, so the browser only receives what it shows
import status from '../data/status.json';
import { partCells, type Dot } from './parts';
import { ANNOUNCED, GENERATION, acpiCell, bootableCell, bootloaderCell, deviceTreeCell, googlebookOmarchyCell, idsCell, linuxImageCell, machineCell, nvidiaKernelCell, nvidiaLinuxCell, nvidiaOmarchyCell, omarchyFirmwareCell, omarchyIdsCell, omarchyTreeCell, upstreamFirmwareCell, type Cell, type Model, type NvidiaModel } from './labels';

export type CellData =
  | Pick<Cell, 'key' | 'label' | 'tone' | 'title' | 'href' | 'rank' | 'full'>
  | { key: string; text: string; rank: number }
  // The parts past booting as a row of dots, one per part of the hardware view
  | { key: string; label: string; dots: Dot[]; title: string; rank: number };
export type TableRow = {
  id: string;
  vendor: string;
  name: string;
  href: string | null;
  announced: string | null;
  discussion: string | null;
  subtitle: string;
  generation: string;
  outcome: 'works' | 'should' | 'workaround' | 'problems' | 'notyet';
  variants: { text: string; tone: 'good' | 'close' | 'wanted' | 'muted'; state: string }[];
  notes: string | null;
  search: string;
  // Model number series a full product number starts with, like 14-fe0 for 14-fe0013dx or 83ED for 83ED0049US
  prefixes: string[];
  cells: Record<string, CellData>;
};

const year = (released: string | number | null) => (released ? String(released).slice(0, 4) : '');
const plain = ({ key, label, tone, title, href, rank, full }: Cell) => ({ key, label, tone, title, href, rank, full });
const announcedTitle = (statusText: string | null) => (statusText ? `${ANNOUNCED.meaning} The vendor said: ${statusText}.` : null);

const VARIANT: Record<string, { tone: TableRow['variants'][number]['tone']; state: string }> = {
  works: { tone: 'good', state: 'boots' },
  should: { tone: 'good', state: 'should boot' },
  workaround: { tone: 'close', state: 'workaround' },
  problems: { tone: 'wanted', state: 'problems' },
  manual: { tone: 'wanted', state: 'needs IDs' },
  notCarried: { tone: 'close', state: 'not carried' },
  none: { tone: 'wanted', state: 'needs a tree' },
};

// A variant that boots says so, like the model's cell does: the channel is no verdict on how well
const variantState = (v: Model['variants'][number]) => VARIANT[v.outcome];

const modelRow = (model: Model, cells: Record<string, CellData>): TableRow => ({
  id: model.id,
  vendor: model.vendor,
  name: model.name,
  href: model.sources[0] ?? null,
  announced: announcedTitle(model.status),
  discussion: model.discussion,
  subtitle: [GENERATION[model.generation], year(model.released), model.chips.join(', '), model.modelNumbers.slice(0, 3).join(', ')].filter(Boolean).join(' · '),
  generation: model.generation,
  outcome: (['works', 'should', 'workaround', 'problems'].includes(model.outcome) ? model.outcome : 'notyet') as TableRow['outcome'],
  variants: model.variants.length > 1 ? model.variants.map((v) => ({ text: [v.model, v.chip].filter(Boolean).join(' · '), ...variantState(v) })) : [],
  notes: model.notes,
  search: [model.vendor, model.name, ...model.modelNumbers, ...model.chips, ...model.variants.map((v) => `${v.model ?? ''} ${v.chip ?? ''}`)].join(' ').toLowerCase(),
  prefixes: model.modelNumbers.map((number) => number.toLowerCase().replace(/x{2,}$/, '')).filter((prefix) => prefix.length >= 4),
  cells,
});

// Googlebooks start through Android's bootloader and Chromebooks through ChromeOS's, so they get columns of their own
export function googlebookRows(boot: 'android' | 'depthcharge' = 'android'): TableRow[] {
  return status.models
    .filter((model: Model) => model.boot === boot)
    .map((model: Model) => ({
      ...modelRow(model, {
        omarchy: plain(googlebookOmarchyCell(model)),
        'device-tree': plain(deviceTreeCell(model)),
        tree: plain(omarchyTreeCell(model)),
        bootloader: plain(bootloaderCell(model)),
        image: plain(linuxImageCell(model)),
      }),
      variants: [],
    }));
}

export function snapdragonRows(): TableRow[] {
  return status.models
    .filter((model: Model) => model.boot === 'uefi')
    .map((model: Model) => ({
      // Snapdragon PCs that start through UEFI have a page of their own, with every check past booting
      ...modelRow(model, {
        // The Omarchy view
        bootable: plain(bootableCell(model)),
        acpi: plain(acpiCell(model)),
        tree: plain(omarchyTreeCell(model)),
        hwids: plain(omarchyIdsCell(model)),
        fw: plain(omarchyFirmwareCell(model)),
        machine: plain(machineCell(model)),
        // The upstream view
        'device-tree': plain(deviceTreeCell(model)),
        ids: plain(idsCell(model)),
        firmware: plain(upstreamFirmwareCell(model)),
        // The hardware view, and its row of dots in the Omarchy view
        ...partCells(model),
      }),
      href: `/models/${model.id}`,
    }));
}

const NVIDIA_OUTCOME: Record<string, TableRow['outcome']> = { works: 'works', expected: 'should', problems: 'problems', untested: 'notyet' };

export function nvidiaRows(): TableRow[] {
  return status.nvidia.map((model: NvidiaModel) => ({
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    // NVIDIA machines have a page of their own too, with every check past booting
    href: `/models/${model.id}`,
    announced: announcedTitle(model.status),
    discussion: null,
    subtitle: [GENERATION[model.generation], year(model.released)].filter(Boolean).join(' · '),
    generation: model.generation,
    outcome: NVIDIA_OUTCOME[model.omarchy] ?? 'notyet',
    variants: [],
    notes: null,
    search: `${model.vendor} ${model.name}`.toLowerCase(),
    prefixes: [],
    cells: {
      omarchy: plain(nvidiaOmarchyCell(model)),
      kernel: plain(nvidiaKernelCell(model)),
      linux: plain(nvidiaLinuxCell(model)),
      notes: { key: model.notes ? 'notes' : 'none', text: model.notes ?? '', rank: model.notes ? 0 : 1 },
      ...partCells(model),
    },
  }));
}
