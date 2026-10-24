import status from '../data/status.json';

export type Model = (typeof status.models)[number];
export type NvidiaModel = (typeof status.nvidia)[number];
export type Tone = 'good' | 'close' | 'wanted' | 'muted' | 'info';

export const TONE: Record<Tone, string> = {
  good: 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30',
  close: 'bg-amber-500/15 text-amber-300 ring-amber-500/30',
  wanted: 'bg-rose-500/15 text-rose-300 ring-rose-500/30',
  muted: 'bg-zinc-500/10 text-zinc-400 ring-zinc-500/20',
  info: 'bg-sky-500/15 text-sky-300 ring-sky-500/30',
};

// A label as the legend shows it; cells reuse its text as their hover title
type State = { label: string; tone: Tone; meaning: string };
export type Cell = State & { title: string; href?: string };

const kernel = status.omarchy.kernelBase.replace(/^v/, '');

const STATES = {
  omarchy: {
    works: { label: 'Boots', tone: 'good', meaning: `Omarchy's kernel has its device tree, and the install image picks it by itself.` },
    manual: { label: 'Not picked', tone: 'close', meaning: `Omarchy's kernel has its device tree, but the install image can't tell it's this model, because systemd has no hardware IDs for it.` },
    newer: { label: 'Newer kernel', tone: 'close', meaning: `Linux has its device tree, but Omarchy's kernel (${kernel}) doesn't yet. It boots once Omarchy moves to a newer kernel.` },
    none: { label: 'No device tree', tone: 'wanted', meaning: 'Nobody has written a device tree for it yet, so Linux can\'t start on it.' },
  },
  deviceTree: {
    released: { label: 'Linux 6.x', tone: 'good', meaning: 'In Linux since that release.' },
    next: { label: 'linux-next', tone: 'close', meaning: 'Accepted, and coming in the next Linux release.' },
    posted: { label: 'Posted', tone: 'close', meaning: 'Sent to the kernel mailing list, not accepted yet.' },
    wanted: { label: 'Wanted', tone: 'wanted', meaning: `Doesn't exist yet.` },
  },
  ids: {
    listed: { label: 'systemd 2xx', tone: 'good', meaning: `In systemd's list since that release, so an install image recognises the model.` },
    mismatch: { label: 'Mismatch', tone: 'close', meaning: `systemd's entry names a different device tree than the kernel's, so it never matches.` },
    wanted: { label: 'Wanted', tone: 'wanted', meaning: `Not in systemd's list yet.` },
  },
  firmware: {
    'linux-firmware': { label: 'linux-firmware', tone: 'good', meaning: 'The signed firmware for its audio, sensor and graphics chips ships in linux-firmware, so nothing has to be copied.' },
    audio: { label: 'Audio only', tone: 'close', meaning: 'linux-firmware has its audio setup file, but the signed firmware for its chips still comes from Windows.' },
    download: { label: 'Vendor download', tone: 'close', meaning: `Not in linux-firmware, but the vendor's driver package has it.` },
    missing: { label: 'From Windows', tone: 'muted', meaning: `Not in linux-firmware. Today it's copied from the laptop's own Windows.` },
  },
  acpi: {
    collected: { label: 'Collected', tone: 'good', meaning: 'Its ACPI tables are in the aarch64-laptops collection.' },
    notNeeded: { label: 'Not needed', tone: 'muted', meaning: 'It already has a device tree.' },
    wanted: { label: 'Wanted', tone: 'wanted', meaning: 'The first thing to send for a model without a device tree.' },
  },
  nvidiaOmarchy: {
    works: { label: 'Boots', tone: 'good', meaning: 'Someone booted Omarchy on it.' },
    expected: { label: 'Should boot', tone: 'close', meaning: `Same platform as a model that boots, but nobody has tried this one yet.` },
    untested: { label: 'Not tested', tone: 'muted', meaning: 'Nobody has tried Omarchy on it yet.' },
    problems: { label: 'Problems', tone: 'wanted', meaning: 'Someone booted it and hit problems.' },
  },
  nvidiaLinux: {
    omarchy: { label: `Omarchy's kernel`, tone: 'close', meaning: `Needs patches that Omarchy's kernel carries and Linux doesn't have yet.` },
    mainline: { label: 'Mainline', tone: 'good', meaning: 'Plain Linux supports it.' },
  },
  announced: { label: 'Announced', tone: 'info', meaning: 'Announced by the vendor, but not confirmed in shops yet.' },
} satisfies Record<string, Record<string, State> | State>;

export const ANNOUNCED = STATES.announced as State;

export const LEGEND: { column: string; question: string; states: State[] }[] = [
  { column: 'Omarchy', question: 'Does the Omarchy install image boot on it today?', states: Object.values(STATES.omarchy) as State[] },
  { column: 'Device tree', question: 'The description of the hardware that Linux needs for each model.', states: Object.values(STATES.deviceTree) as State[] },
  { column: 'Hardware IDs', question: 'How an install image recognises the model and picks its device tree.', states: Object.values(STATES.ids) as State[] },
  { column: 'Firmware', question: 'Files for the chips inside, which only the vendor can license.', states: Object.values(STATES.firmware) as State[] },
  { column: 'ACPI dump', question: `The laptop's own hardware tables, which a device tree is written from.`, states: Object.values(STATES.acpi) as State[] },
];

export const NVIDIA_LEGEND: { column: string; question: string; states: State[] }[] = [
  { column: 'Omarchy', question: 'Has someone booted Omarchy on it?', states: Object.values(STATES.nvidiaOmarchy) as State[] },
  { column: 'Linux', question: 'Whether plain Linux runs it, or it needs Omarchy\'s kernel.', states: Object.values(STATES.nvidiaLinux) as State[] },
];

const cell = (state: State, extra: { label?: string; title?: string; href?: string } = {}): Cell => ({ ...state, label: extra.label ?? state.label, title: extra.title ?? state.meaning, href: extra.href });
const release = (tag: string) => tag.replace(/^v/, '');
const hasUpstreamTree = (model: Model) => model.upstream.deviceTree === 'released' || model.upstream.deviceTree === 'next';

export function omarchyCell(model: Model): Cell {
  if (model.omarchy === 'works') return cell(STATES.omarchy.works as State);
  if (model.omarchy === 'manual') return cell(STATES.omarchy.manual as State);
  if (hasUpstreamTree(model)) return cell(STATES.omarchy.newer as State);
  return cell(STATES.omarchy.none as State);
}

export function deviceTreeCell(model: Model): Cell {
  const { deviceTree, firstRelease, devicetreePosted } = model.upstream;
  if (deviceTree === 'released') return cell(STATES.deviceTree.released as State, { label: `Linux ${release(firstRelease!)}`, title: `In Linux since ${release(firstRelease!)}.` });
  if (deviceTree === 'next') return cell(STATES.deviceTree.next as State);
  if (deviceTree === 'posted') return cell(STATES.deviceTree.posted as State, { href: devicetreePosted ?? undefined });
  return cell(STATES.deviceTree.wanted as State);
}

export function idsCell(model: Model): Cell {
  if (model.upstream.ids === 'listed') {
    const first = model.variants.flatMap((variant) => variant.ids).filter((entry) => entry.matches).map((entry) => entry.firstRelease).filter(Boolean).sort()[0];
    const label = first ? `systemd ${release(first)}` : 'systemd main';
    return cell(STATES.ids.listed as State, { label, title: first ? `In systemd's list since ${release(first)}, so an install image recognises the model.` : `In systemd's development branch, coming in its next release.` });
  }
  if (model.upstream.ids === 'mismatch') return cell(STATES.ids.mismatch as State);
  return cell(STATES.ids.wanted as State);
}

export function firmwareCell(model: Model): Cell {
  const state = STATES.firmware[model.upstream.firmware as keyof typeof STATES.firmware] as State;
  return cell(state, { href: model.upstream.firmware === 'download' ? (model.upstream.firmwareDownload ?? undefined) : undefined });
}

export function acpiCell(model: Model): Cell {
  if (model.upstream.acpi === 'collected') return cell(STATES.acpi.collected as State, { href: model.upstream.acpiUrls[0] });
  if (hasUpstreamTree(model)) return cell(STATES.acpi.notNeeded as State);
  return cell(STATES.acpi.wanted as State);
}

export function nvidiaOmarchyCell(model: NvidiaModel): Cell {
  return cell((STATES.nvidiaOmarchy[model.omarchy as keyof typeof STATES.nvidiaOmarchy] ?? STATES.nvidiaOmarchy.untested) as State);
}

export function nvidiaLinuxCell(model: NvidiaModel): Cell {
  return cell(STATES.nvidiaLinux[model.linux as keyof typeof STATES.nvidiaLinux] as State);
}

export const GENERATION: Record<string, string> = { x1: 'Snapdragon X', x2: 'Snapdragon X2', '8cx': 'Snapdragon 8cx Gen 3', n1x: 'NVIDIA N1x', gb10: 'NVIDIA GB10' };
