// The checks past booting (data/checks.yaml) and what's known about them per model
// (data/findings.yaml), joined with the board's own model data for the model pages
// and the JSON the API serves.
import { parse } from 'yaml';
import checksRaw from '../../data/checks.yaml?raw';
import findingsRaw from '../../data/findings.yaml?raw';
import status from '../data/status.json';
import { googlebookOmarchyCell, type Model, type NvidiaModel } from './labels';

export type CheckState = 'works' | 'partial' | 'broken' | 'untested' | 'absent';
export type Check = { id: string; group: string; title: string; works: string; probe?: string; person?: string; debug?: string; optional?: boolean; desk?: { title?: string; works?: string; person?: string } };
export type Group = { id: string; title: string };
/**
 * What a finding rests on. Only a result on Omarchy says how the model does under Omarchy. One from
 * another distro, or from the device tree's author, is a hint that it can work, and is shown as that.
 */
export type Basis = 'omarchy' | 'other-distro' | 'tree-author' | 'maker-spec';
export type Finding = { state: CheckState; note?: string; version?: string; checked?: string; source?: string; basis?: Basis; where?: string; quote?: string; under?: string };

/** A finding with no basis named was written by hand from Omarchy's own threads. */
export const basisOf = (finding: Finding): Basis => finding.basis ?? 'omarchy';
/** Whether a finding says how the model does under Omarchy, and so counts as its status. */
export const countsAsStatus = (finding: Finding): boolean => basisOf(finding) === 'omarchy' || basisOf(finding) === 'maker-spec';
type FindingsFile = {
  families?: Record<string, { source?: string; checks?: Record<string, Finding> }>;
  models?: Record<string, { details?: string; links?: { title: string; url: string }[]; checks?: Record<string, Finding> }>;
};

const checksFile = parse(checksRaw) as { groups: Group[]; checks: Check[] };
const findings = parse(findingsRaw) as FindingsFile;

export const GROUPS = checksFile.groups;
export const CHECKS = checksFile.checks;

// Generations whose family findings count for a model; the older Windows on Arm chips have none yet
const FAMILY: Record<string, string> = { x1: 'x1', x2: 'x2' };

export const STATE_LABEL: Record<CheckState, string> = { works: 'Works', partial: 'Partly', broken: 'Broken', untested: 'Untested', absent: 'Not on this model' };
export const STATE_TONE: Record<CheckState, 'good' | 'close' | 'wanted' | 'muted'> = { works: 'good', partial: 'close', broken: 'wanted', untested: 'muted', absent: 'muted' };

/** The Snapdragon PCs that start through UEFI, each with a page and checks. */
export function pageModels(): Model[] {
  return status.models.filter((model: Model) => model.boot === 'uefi');
}

/** Chromebooks and Googlebooks, which start through Google's own boot software. They get a page and the same checks. */
export function googlePageModels(): Model[] {
  return status.models.filter((model: Model) => model.boot === 'android' || model.boot === 'depthcharge');
}

/** The NVIDIA machines, which start through UEFI with ACPI and get a page and the same checks. */
export function nvidiaPageModels(): NvidiaModel[] {
  return status.nvidia as NvidiaModel[];
}

/** What the checks need to know about a machine, whichever group it is in. */
// `boot` is how it starts: uefi, or Google's boot software (android, depthcharge)
export type Subject = { id: string; generation: string; form?: string | null; boot?: string | null };

const NVIDIA = new Set(['n1x', 'gb10']);
// Parts a machine can't have, by what it is, so nobody is asked to test them and they don't read as unknown
const QUALCOMM_ONLY = ['adsp', 'cdsp'];
const LAPTOP_ONLY = ['display', 'backlight', 'keyboard', 'touchpad', 'touchscreen', 'battery', 'charging', 'suspend-drain', 'lid', 'microphones', 'camera'];
// An all-in-one is a desktop with a screen of its own, so the screen, its brightness and touch stay
const ALL_IN_ONE_LACKS = ['keyboard', 'touchpad', 'battery', 'charging', 'suspend-drain', 'lid'];
// Parts a desktop has by rule and few laptops do. On other machines the check shows once something is known about it
const DESK_ONLY = ['ethernet'];
const DESKS = new Set(['desktop', 'devkit', 'all-in-one']);
function notOn(model: Subject, check: string): Finding | null {
  if (NVIDIA.has(model.generation) && QUALCOMM_ONLY.includes(check)) return { state: 'absent', note: 'A Qualcomm processor. NVIDIA machines have no such part.', basis: 'maker-spec' };
  if ((model.form === 'desktop' || model.form === 'devkit') && LAPTOP_ONLY.includes(check)) return { state: 'absent', note: 'A desktop or mini PC, without a screen, keyboard, touchpad, battery, camera or microphone of its own.', basis: 'maker-spec' };
  if (model.form === 'all-in-one' && ALL_IN_ONE_LACKS.includes(check)) return { state: 'absent', note: 'An all-in-one, with a screen of its own and no built-in keyboard, touchpad or battery.', basis: 'maker-spec' };
  return null;
}
/** Whether a model is expected to have a check's part, by its form. */
const expectedOn = (model: Subject, check: string): boolean => !DESK_ONLY.includes(check) || DESKS.has(model.form ?? '');

// `given` marks a finding nobody had to write: a part the machine can't have, by what it is
// `expected` is false for a part this kind of machine rarely has, whose row shows only once something is known
export type CheckRow = Check & { finding: Finding | null; from: 'model' | 'family' | null; familySource: string | null; given?: true; expected: boolean };

/** Every check for one model, with the model's own finding, or else its chip family's. */
export function modelChecks(model: Subject): CheckRow[] {
  const own = findings.models?.[model.id]?.checks ?? {};
  // What holds for a chip family was seen on machines that start Omarchy's own image. A Chromebook or
  // Googlebook has the same chip and starts another way, so none of it is taken over for one
  const google = model.boot === 'android' || model.boot === 'depthcharge';
  const family = google ? undefined : findings.families?.[FAMILY[model.generation] ?? ''];
  return CHECKS.map(({ desk, ...plain }) => {
    // A desktop's rows read as they are on a desktop. An all-in-one has no lid either, but its screen is its own
    const check = desk && DESKS.has(model.form ?? '') && !(model.form === 'all-in-one' && plain.id === 'external-display') ? { ...plain, ...desk } : plain;
    // A finding written for the model says it has the part, whatever its kind of machine
    const expected = expectedOn(model, check.id) || Boolean(own[check.id]);
    if (own[check.id]) return { ...check, finding: own[check.id], from: 'model' as const, familySource: null, expected };
    const lacks = notOn(model, check.id);
    if (lacks) return { ...check, finding: lacks, from: 'model' as const, familySource: null, given: true as const, expected };
    const shared = family?.checks?.[check.id];
    if (shared) return { ...check, finding: shared, from: 'family' as const, familySource: family?.source ?? null, expected };
    return { ...check, finding: null, from: null, familySource: null, expected };
  });
}

/** Whether a check is one to ask an owner for: its part is on every model, or on this kind of machine. */
const askedFor = (row: CheckRow): boolean => (DESK_ONLY.includes(row.id) ? row.expected : !row.optional);

export function modelExtra(model: Subject) {
  const entry = findings.models?.[model.id];
  return { details: entry?.details?.trim() ?? null, links: entry?.links ?? [] };
}

/**
 * How many checks a model's own findings cover. Only what was seen on Omarchy counts as known and
 * working. What is reported from elsewhere is counted apart.
 */
export function coverage(model: Subject) {
  // The checks that apply to this machine: the parts it has, and not those few of its kind have
  const rows = modelChecks(model).filter((row) => row.finding?.state !== 'absent' && (row.expected || row.finding));
  const own = rows.filter((row) => row.from === 'model');
  const status = own.filter((row) => countsAsStatus(row.finding!));
  return {
    known: status.length,
    works: status.filter((row) => row.finding!.state === 'works').length,
    elsewhere: own.length - status.length,
    total: rows.length,
  };
}

/**
 * Whether a model boots, as the API says it: the channel whose image starts a Snapdragon PC, or what
 * it still needs. A Chromebook or Googlebook starts another way and needs no hardware IDs, so for one
 * it is what its own Bootable cell says: works, workaround, problems, untested or notyet.
 */
export function bootableOf(model: Model): string {
  return model.boot === 'uefi' ? model.bootable : googlebookOmarchyCell(model).key;
}

/** What a model's page and GET /api/models/<id> say, before owners' live results are added. */
export function modelData(model: Model) {
  const checks = modelChecks(model);
  const extra = modelExtra(model);
  return {
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    generation: model.generation,
    form: model.form,
    released: model.released,
    chips: model.chips,
    modelNumbers: model.modelNumbers,
    page: `https://arewearmyet.com/models/${model.id}`,
    boots: { outcome: model.outcome, bootable: bootableOf(model), pieces: model.pieces, omarchy: model.omarchy },
    upstream: model.upstream,
    variants: model.variants.map((v) => ({ chip: v.chip, compatible: v.compatible, deviceTree: v.deviceTree, firmwareNeeded: v.firmwareNeeded, outcome: v.outcome })),
    notes: model.notes,
    details: extra.details,
    links: [...(model.discussion ? [{ title: 'Discussion', url: model.discussion }] : []), ...extra.links],
    reports: model.reports,
    checks: checks.map(({ id, group, title, finding, from }) => ({ id, group, title, finding, from })),
    // Checks nobody has written up for this model, the ones an owner's run helps most with
    // A check known only from another distro or from the tree's author is still wanted on Omarchy
    wanted: checks.filter((row) => !(row.from === 'model' && countsAsStatus(row.finding!)) && askedFor(row)).map((row) => row.id),
  };
}

/** What an NVIDIA machine's page and GET /api/models/<id> say, before owners' live results are added. */
export function nvidiaData(model: NvidiaModel) {
  const checks = modelChecks(model);
  const extra = modelExtra(model);
  return {
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    generation: model.generation,
    form: model.form,
    released: model.released,
    page: `https://arewearmyet.com/models/${model.id}`,
    // These start through UEFI with ACPI: no device tree, hardware IDs or firmware package per model
    boots: { outcome: model.omarchy, kernel: model.kernel ?? null, linux: model.linux },
    notes: model.notes,
    details: extra.details,
    links: extra.links,
    checks: checks.map(({ id, group, title, finding, from }) => ({ id, group, title, finding, from })),
    wanted: checks.filter((row) => !(row.from === 'model' && countsAsStatus(row.finding!)) && askedFor(row)).map((row) => row.id),
  };
}
