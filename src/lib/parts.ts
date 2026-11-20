// The hardware view: the checks past booting rolled up into the parts an owner thinks in, per
// model. A part's colour is its state and its fill is the evidence. Solid means seen on Omarchy,
// by hw-test or an owner. Nothing else counts: not what another distro's users saw, and not
// what was found on another model of the same chip.
import owners from '../../data/owners.json';
import { countsAsStatus, modelChecks, type CheckState, type Subject } from './checks';
import type { Tone } from './tones';

export type Part = { id: string; title: string; question: string; checks: string[] };

// Hibernation, the two Qualcomm processors, the fingerprint reader and the modem stay on the model's
// page: no kernel offers hibernation on these chips yet, and the others are parts few models have
export const PARTS: Part[] = [
  { id: 'start', title: 'Starting', question: 'The installer from a USB stick, and the disk password prompt at startup.', checks: ['installer', 'disk-unlock'] },
  { id: 'screen', title: 'Screen and graphics', question: 'The built-in screen and its brightness, graphics acceleration, WebGL and WebGPU in the browser, and hardware video decoding.', checks: ['display', 'backlight', 'gpu', 'webgl', 'webgpu', 'video-decode'] },
  { id: 'input', title: 'Keyboard and touch', question: 'The built-in keyboard, the touchpad, and the touchscreen and pen where the model has them.', checks: ['keyboard', 'touchpad', 'touchscreen'] },
  { id: 'wireless', title: 'Wi-Fi and Bluetooth', question: 'Wi-Fi, its fixed address, and Bluetooth.', checks: ['wifi', 'wifi-address', 'bluetooth'] },
  { id: 'sound', title: 'Sound', question: 'Speakers, built-in microphones and the headphone jack.', checks: ['speakers', 'microphones', 'headphones'] },
  { id: 'camera', title: 'Camera', question: 'The webcam, in the browser and in video calls.', checks: ['camera'] },
  { id: 'power', title: 'Battery and sleep', question: 'Battery readings, charging, sleep and wake, battery use while asleep, the lid switch, and fans and temperatures. Hibernation is on the model\'s page.', checks: ['battery', 'charging', 'suspend', 'suspend-drain', 'lid', 'fans'] },
  { id: 'ports', title: 'Ports', question: 'USB ports, an external screen, USB4, and the network socket where the model has one.', checks: ['usb', 'external-display', 'usb4', 'ethernet'] },
];

type Level = 'omarchy' | 'elsewhere';
type Seen = { check: string; title: string; state: Exclude<CheckState, 'untested'>; level: Level; how: string };
type Latest = { state: string; kernel: string | null; os?: string | null };
type OwnerCheck = { machines: number; states: Record<string, number>; latest?: Latest[] };
// A result is from Omarchy when its run says so, or, for results kept before runs said, when the kernel is Omarchy's
const onOmarchy = (result: Latest) => (result.os ? /^omarchy/i.test(result.os) : /-omarchy/.test(result.kernel ?? ''));
const OWNERS = (owners as { models: Record<string, { checks: Record<string, OwnerCheck> }> }).models;

const overall = (states: string[]): 'works' | 'partial' | 'broken' => (states.every((s) => s === 'works') ? 'works' : states.every((s) => s === 'broken') ? 'broken' : 'partial');

/** What is known about each check of a model, from the strongest evidence there is for it. */
// A machine as the roll-up needs it. `bootable` is the channel whose image starts a Snapdragon PC
type Machine = Subject & { bootable?: string };

function seen(model: Machine): Map<string, Seen> {
  const out = new Map<string, Seen>();
  const measured = OWNERS[model.id]?.checks ?? {};
  const boots = ['stable', 'rc', 'edge'].includes(model.bootable ?? '');
  for (const row of modelChecks(model)) {
    const title = row.title;
    // Owners' results come from hw-test, on Omarchy and now and then on another distro. Only those from
    // Omarchy say how the part does there. The others are a hint, like any finding from elsewhere
    const latest = (measured[row.id]?.latest ?? []).filter((r) => r.state !== 'untested');
    const here = latest.filter(onOmarchy);
    if (here.length) {
      const states = here.map((r) => r.state);
      const state = states.every((s) => s === 'absent') ? 'absent' : overall(states.filter((s) => s !== 'absent'));
      // The kernel says how current the result is: a fault seen on an older kernel may be fixed since
      const kernel = here[0].kernel ? `, on ${here[0].kernel.replace(/-omarchy$/, '')}` : '';
      out.set(row.id, { check: row.id, title, state, level: 'omarchy', how: (here.length === 1 ? `an owner's test` : `${here.length} owners' tests`) + kernel });
      continue;
    }
    const finding = row.finding;
    if (!finding || finding.state === 'untested') continue;
    // What was found on one machine of a chip family is no result for another: the tests have
    // already shown a part working on one model and not on its sibling. So only the model's own
    // findings and tests count here. The family's note still shows on the model's page
    if (row.from === 'family') continue;
    if (countsAsStatus(finding)) {
      out.set(row.id, { check: row.id, title, state: finding.state, level: 'omarchy', how: finding.basis === 'maker-spec' ? `the maker's description` : 'reported on Omarchy' + (finding.version ? `, on ${finding.version}` : '') });
    }
    // What another distro's users or a device tree's author reported is a lead for whoever
    // fixes the part, shown in the check's details on the model's page. It is no status here
  }
  return out;
}

export type PartState = { part: Part; key: string; state: 'works' | 'partial' | 'broken' | 'absent' | 'unknown'; level: Level | null; covered: number; of: number; title: string };

const WORD = { works: 'works', partial: 'partly works', broken: 'is broken', absent: 'is not on this model' } as const;

export function modelParts(model: Machine): PartState[] {
  const known = seen(model);
  const expected = new Set(modelChecks(model).filter((row) => row.expected).map((row) => row.id));
  return PARTS.map((part) => {
    // A part few machines of this kind have counts only once something is known about it
    const checks = part.checks.filter((check) => known.has(check) || expected.has(check));
    const rows = checks.map((check) => known.get(check)).filter((row): row is Seen => Boolean(row));
    const there = rows.filter((row) => row.state !== 'absent');
    const of = checks.length - (rows.length - there.length);
    const lines = (list: Seen[]) => list.map((row) => `${row.title} ${WORD[row.state]} (${row.how})`).join('. ');
    if (there.length === 0) {
      if (rows.length === checks.length) return { part, key: 'absent', state: 'absent', level: null, covered: 0, of: 0, title: 'This model does not have the part.' };
      return { part, key: 'unknown', state: 'unknown', level: null, covered: 0, of, title: 'Nobody has reported on it yet.' };
    }
    const onOmarchy = there.filter((row) => row.level === 'omarchy');
    const level: Level = onOmarchy.length ? 'omarchy' : 'elsewhere';
    const counted = onOmarchy.length ? onOmarchy : there;
    const state = overall(counted.map((row) => row.state));
    const others = there.filter((row) => !counted.includes(row));
    const title = [
      `${counted.length} of ${of} ${of === 1 ? 'check' : 'checks'} ${level === 'omarchy' ? 'seen on Omarchy' : 'known for the chip family, none seen on this model yet'}. ${lines(counted)}.`,
      others.length ? `The chip family has ${lines(others)}.` : '',
    ].filter(Boolean).join(' ');
    return { part, key: `${level}-${state}`, state, level, covered: counted.length, of, title };
  });
}

const LABEL = { works: 'Works', partial: 'Partly', broken: 'Broken' } as const;
const SOLID: Record<string, Tone> = { works: 'good', partial: 'close', broken: 'wanted' };
const OUTLINED: Record<string, Tone> = { works: 'good-seen', partial: 'close-seen', broken: 'wanted-seen' };
// Best first: what is seen on Omarchy before what is known for the chip family, then what nobody knows
const RANK = ['omarchy-works', 'omarchy-partial', 'omarchy-broken', 'elsewhere-works', 'elsewhere-partial', 'elsewhere-broken', 'absent', 'unknown'];

export const PART_OPTIONS: { key: string; label: string; tone?: Tone }[] = [
  { key: 'omarchy-works', label: 'Works', tone: 'good' },
  { key: 'omarchy-partial', label: 'Partly', tone: 'close' },
  { key: 'omarchy-broken', label: 'Broken', tone: 'wanted' },
  { key: 'absent', label: 'Not on this model' },
  { key: 'unknown', label: 'Not known yet' },
];

export type Dot = { tone: 'good' | 'close' | 'wanted' | 'none' | 'absent'; solid: boolean };
type PillCell = { key: string; label: string; tone: Tone; title: string; href?: string; rank: number };
type TextCell = { key: string; text: string; rank: number };
type DotsCell = { key: string; label: string; dots: Dot[]; title: string; rank: number };

/** The cells of the hardware view for one model, and the row of dots the booting view shows. */
export function partCells(model: Machine): Record<string, PillCell | TextCell | DotsCell> {
  const parts = modelParts(model);
  const cells: Record<string, PillCell | TextCell | DotsCell> = {};
  for (const p of parts) {
    const rank = RANK.indexOf(p.key);
    if (p.state === 'unknown') cells[`part-${p.part.id}`] = { key: p.key, text: '', rank };
    else if (p.state === 'absent') cells[`part-${p.part.id}`] = { key: p.key, text: '–', rank };
    else {
      const label = LABEL[p.state] + (p.covered < p.of ? ` ${p.covered}/${p.of}` : '');
      cells[`part-${p.part.id}`] = { key: p.key, label, tone: (p.level === 'omarchy' ? SOLID : OUTLINED)[p.state], title: p.title, href: `/models/${model.id}`, rank };
    }
  }
  const real = parts.filter((p) => p.state !== 'absent');
  const onOmarchy = real.filter((p) => p.level === 'omarchy');
  const working = onOmarchy.filter((p) => p.state === 'works').length;
  const elsewhere = real.filter((p) => p.level === 'elsewhere').length;
  // Models with the most seen on Omarchy first, then those known from elsewhere
  const rank = 1000 - onOmarchy.length * 100 - working * 10 - elsewhere;
  const summary = onOmarchy.length ? `${onOmarchy.length} of ${real.length} parts seen on Omarchy, ${working} working` : elsewhere ? `${elsewhere} of ${real.length} parts known for the chip family, none seen on Omarchy yet` : 'Nothing reported yet';
  cells.seen = { key: onOmarchy.length ? 'some' : elsewhere ? 'elsewhere' : 'none', text: onOmarchy.length ? `${onOmarchy.length} of ${real.length}` : '', rank };
  cells.parts = {
    key: onOmarchy.length ? 'some' : elsewhere ? 'elsewhere' : 'none',
    label: summary,
    dots: parts.map((p) => ({ tone: p.state === 'works' ? 'good' : p.state === 'partial' ? 'close' : p.state === 'broken' ? 'wanted' : p.state === 'absent' ? 'absent' : 'none', solid: p.level === 'omarchy' })),
    title: `${summary}. In the columns' order, ${parts.map((p) => `${p.part.title} ${p.state === 'unknown' ? 'not known' : p.state === 'absent' ? 'not on this model' : (p.level === 'omarchy' ? '' : 'for the chip family ') + LABEL[p.state].toLowerCase()}`).join(', ')}.`,
    rank,
  };
  return cells;
}

/** For the bars above the table: per part, how many models have it seen on Omarchy, known for the chip family, or not at all. */
export function partTotals(models: Machine[]) {
  const all = models.map(modelParts);
  return PARTS.map((part, i) => {
    const states = all.map((parts) => parts[i]).filter((p) => p.state !== 'absent');
    const on = states.filter((p) => p.level === 'omarchy');
    return {
      part,
      models: states.length,
      works: on.filter((p) => p.state === 'works').length,
      partial: on.filter((p) => p.state === 'partial').length,
      broken: on.filter((p) => p.state === 'broken').length,
      elsewhere: states.filter((p) => p.level === 'elsewhere').length,
      unknown: states.filter((p) => p.state === 'unknown').length,
    };
  });
}
