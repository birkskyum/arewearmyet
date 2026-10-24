import status from '../data/status.json';
import { TONE, type Tone } from './tones';

export { TONE, type Tone };
export type Model = (typeof status.models)[number];
export type NvidiaModel = (typeof status.nvidia)[number];

// A label as the legend shows it; cells reuse its text as their hover title and sort by rank
type State = { label: string; tone: Tone; meaning: string };
export type Cell = State & { key: string; title: string; href?: string; rank: number };

const kernel = status.omarchy.kernelBase.replace(/^v/, '');

const STATES = {
  omarchy: {
    works: { label: 'Works', tone: 'good', meaning: 'An owner confirmed that the current Omarchy release runs on it.' },
    should: { label: 'Should work', tone: 'close', meaning: `Omarchy's kernel has its device tree and the install image picks it by itself, but no owner has confirmed it yet.` },
    problems: { label: 'Problems', tone: 'wanted', meaning: 'An owner reported it failing on the current Omarchy release.' },
    manual: { label: 'Not picked', tone: 'close', meaning: `Omarchy's kernel has its device tree, but the install image can't tell it's this model, because systemd has no hardware IDs for it.` },
    newer: { label: 'Newer kernel', tone: 'close', meaning: `Linux has its device tree, but Omarchy's kernel (${kernel}) doesn't yet. It should work once Omarchy moves to a newer kernel.` },
    none: { label: 'No device tree', tone: 'wanted', meaning: `Nobody has written a device tree for it yet, so Linux can't start on it.` },
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
    'linux-firmware': { label: 'linux-firmware', tone: 'good', meaning: `Every file its device tree loads ships in linux-firmware's newest release, so nothing has to be copied.` },
    download: { label: 'Vendor download', tone: 'good', meaning: `The vendor's driver download has it, so it works even after Windows is wiped.` },
    'next-release': { label: 'Next release', tone: 'close', meaning: `Merged into linux-firmware, and coming with its next release.` },
    partial: { label: 'Partly', tone: 'close', meaning: 'linux-firmware has some of the files its device tree loads; the rest still come from Windows.' },
    windows: { label: 'Windows only', tone: 'close', meaning: `Copied from the laptop's own Windows today. A link to the vendor's driver download would cover laptops whose Windows is gone.` },
  },
  acpi: {
    collected: { label: 'Collected', tone: 'good', meaning: 'Its ACPI tables are in the aarch64-laptops collection.' },
    notNeeded: { label: 'Not needed', tone: 'muted', meaning: 'It already has a device tree.' },
    wanted: { label: 'Wanted', tone: 'wanted', meaning: 'The first thing to send for a model without a device tree.' },
  },
  nvidiaOmarchy: {
    works: { label: 'Works', tone: 'good', meaning: 'Someone ran Omarchy on it.' },
    expected: { label: 'Should work', tone: 'close', meaning: 'Same platform as a model that works, but nobody has tried this one yet.' },
    problems: { label: 'Problems', tone: 'wanted', meaning: 'Someone tried it and hit problems.' },
    untested: { label: 'Not tested', tone: 'muted', meaning: 'Nobody has tried Omarchy on it yet.' },
  },
  nvidiaLinux: {
    omarchy: { label: `Omarchy's kernel`, tone: 'close', meaning: `Needs patches that Omarchy's kernel carries and Linux doesn't have yet.` },
    mainline: { label: 'Mainline', tone: 'good', meaning: 'Plain Linux supports it.' },
  },
  announced: { label: 'Announced', tone: 'info', meaning: 'Announced by the vendor, but not confirmed in shops yet.' },
} satisfies Record<string, Record<string, State> | State>;

export const ANNOUNCED = STATES.announced as State;

type Group = { column: string; question: string; states: State[] };

export const LEGEND: Group[] = [
  { column: 'Omarchy', question: 'Does the current Omarchy release run on it?', states: Object.values(STATES.omarchy) as State[] },
  { column: 'Device tree', question: 'The description of the hardware that Linux needs for each model.', states: Object.values(STATES.deviceTree) as State[] },
  { column: 'Hardware IDs', question: 'How an install image recognises the model and picks its device tree.', states: Object.values(STATES.ids) as State[] },
  { column: 'Firmware', question: `Signed files for its audio, sensor and graphics chips. Booting doesn't need them, but without them there's no sound, battery status, USB-C display, sensors or graphics acceleration. The goal is getting them without the laptop's own Windows.`, states: Object.values(STATES.firmware) as State[] },
  { column: 'ACPI dump', question: `The laptop's own hardware tables, which a device tree is written from.`, states: Object.values(STATES.acpi) as State[] },
];

export const NVIDIA_LEGEND: Group[] = [
  { column: 'Omarchy', question: 'Has someone run Omarchy on it?', states: Object.values(STATES.nvidiaOmarchy) as State[] },
  { column: 'Linux', question: `Whether plain Linux runs it, or it needs Omarchy's kernel.`, states: Object.values(STATES.nvidiaLinux) as State[] },
];

// Ranks sort best first: a state's position in its group, or a release number within "done"
const cell = (group: Record<string, State>, key: string, extra: { label?: string; title?: string; href?: string; rank?: number } = {}): Cell => {
  const state = group[key];
  return { ...state, key, label: extra.label ?? state.label, title: extra.title ?? state.meaning, href: extra.href, rank: extra.rank ?? 1000 + Object.keys(group).indexOf(key) };
};
const release = (tag: string) => tag.replace(/^v/, '');
const releaseRank = (tag: string) => {
  const [major, minor] = release(tag).split('.').map(Number);
  return major * 100 + (minor || 0);
};
const hasUpstreamTree = (model: Model) => model.upstream.deviceTree === 'released' || model.upstream.deviceTree === 'next';
const reportNotes = (reports: { note: string; variant?: string }[]) => reports.map((r) => (r.variant ? `${r.variant}: ${r.note}` : r.note)).join(' ');

export function omarchyCell(model: Model): Cell {
  const meaning = (STATES.omarchy[model.outcome as keyof typeof STATES.omarchy] as State).meaning;
  const notes = model.outcome === 'works' || model.outcome === 'problems' ? reportNotes(model.reports) : '';
  return cell(STATES.omarchy, model.outcome, { title: notes ? `${meaning} ${notes}` : meaning });
}

export function deviceTreeCell(model: Model): Cell {
  const { deviceTree, firstRelease, devicetreePosted } = model.upstream;
  if (deviceTree === 'released') return { ...cell(STATES.deviceTree, 'released', { label: `Linux ${release(firstRelease!)}`, title: `In Linux since ${release(firstRelease!)}.`, rank: releaseRank(firstRelease!) }), key: `linux-${release(firstRelease!)}` };
  if (deviceTree === 'posted') return cell(STATES.deviceTree, 'posted', { href: devicetreePosted ?? undefined });
  return cell(STATES.deviceTree, deviceTree === 'next' ? 'next' : 'wanted');
}

export function idsCell(model: Model): Cell {
  if (model.upstream.ids === 'listed') {
    const first = model.variants.flatMap((variant) => variant.ids).filter((entry) => entry.matches).map((entry) => entry.firstRelease).filter(Boolean).sort()[0];
    const listed = cell(STATES.ids, 'listed', {
      label: first ? `systemd ${release(first)}` : 'systemd main',
      title: first ? `In systemd's list since ${release(first)}, so an install image recognises the model.` : `In systemd's development branch, coming in its next release.`,
      rank: first ? Number(release(first)) : 999,
    });
    return { ...listed, key: first ? `systemd-${release(first)}` : 'systemd-main' };
  }
  return cell(STATES.ids, model.upstream.ids === 'mismatch' ? 'mismatch' : 'wanted');
}

export function firmwareCell(model: Model): Cell {
  const key = model.upstream.firmware;
  const meaning = (STATES.firmware[key as keyof typeof STATES.firmware] as State).meaning;
  const missing = model.upstream.firmwareMissing;
  const detail =
    key === 'next-release' ? ` The release after ${status.firmwareRelease} adds ${missing.join(', ')}.` :
    key === 'partial' ? ` Still missing: ${missing.join(', ')}.` :
    key === 'linux-firmware' ? ` Checked against release ${status.firmwareRelease}.` :
    model.upstream.firmwareAudioOnly ? ' linux-firmware already has its audio setup file.' : '';
  return cell(STATES.firmware, key, { title: meaning + detail, href: key === 'download' ? (model.upstream.firmwareDownload ?? undefined) : undefined });
}

export function acpiCell(model: Model): Cell {
  if (model.upstream.acpi === 'collected') return cell(STATES.acpi, 'collected', { href: model.upstream.acpiUrls[0] });
  return cell(STATES.acpi, hasUpstreamTree(model) ? 'notNeeded' : 'wanted');
}

export function nvidiaOmarchyCell(model: NvidiaModel): Cell {
  return cell(STATES.nvidiaOmarchy, model.omarchy in STATES.nvidiaOmarchy ? model.omarchy : 'untested');
}

export function nvidiaLinuxCell(model: NvidiaModel): Cell {
  return cell(STATES.nvidiaLinux, model.linux);
}

export const GENERATION: Record<string, string> = { x1: 'Snapdragon X', x2: 'Snapdragon X2', '8cx': 'Snapdragon 8cx Gen 3', n1x: 'NVIDIA N1x', gb10: 'NVIDIA GB10' };

// The choices a column header's filter offers, one per state, in legend order. "Linux 6.x" and
// "systemd 2xx" stand for any release, so their filter choices say so.
const options = (group: Record<string, State>, skip?: string) => Object.entries(group).filter(([key]) => key !== skip).map(([key, state]) => ({ key, label: state.label, tone: state.tone }));
// Device trees and hardware IDs offer each release that occurs, oldest first, before their other states
const releases = (cells: { key: string; label: string; rank: number }[], prefix: string) =>
  [...new Map(cells.filter((c) => c.key.startsWith(prefix)).sort((a, b) => a.rank - b.rank).map((c) => [c.key, { key: c.key, label: c.label, tone: 'good' as Tone }])).values()];
export const FILTERS = {
  omarchy: options(STATES.omarchy),
  'device-tree': [...releases(status.models.map(deviceTreeCell), 'linux-'), ...options(STATES.deviceTree, 'released')],
  ids: [...releases(status.models.map(idsCell), 'systemd-'), ...options(STATES.ids, 'listed')],
  firmware: options(STATES.firmware),
  acpi: options(STATES.acpi),
  nvidiaOmarchy: options(STATES.nvidiaOmarchy),
  nvidiaLinux: options(STATES.nvidiaLinux),
};
