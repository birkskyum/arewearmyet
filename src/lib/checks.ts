// The checks past booting (data/checks.yaml) and what's known about them per model
// (data/findings.yaml), joined with the board's own model data for the model pages
// and the JSON the API serves.
import { parse } from 'yaml';
import checksRaw from '../../data/checks.yaml?raw';
import findingsRaw from '../../data/findings.yaml?raw';
import status from '../data/status.json';
import type { Model, NvidiaModel } from './labels';

export type CheckState = 'works' | 'partial' | 'broken' | 'untested' | 'absent';
export type Check = { id: string; group: string; title: string; works: string; probe?: string; person?: string; debug?: string; optional?: boolean };
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

/** The NVIDIA machines, which start through UEFI with ACPI and get a page and the same checks. */
export function nvidiaPageModels(): NvidiaModel[] {
  return status.nvidia as NvidiaModel[];
}

/** What the checks need to know about a machine, whichever group it is in. */
export type Subject = { id: string; generation: string; form?: string | null };

const NVIDIA = new Set(['n1x', 'gb10']);
// Parts a machine can't have, by what it is, so nobody is asked to test them and they don't read as unknown
const QUALCOMM_ONLY = ['adsp', 'cdsp'];
const LAPTOP_ONLY = ['display', 'backlight', 'keyboard', 'touchpad', 'touchscreen', 'battery', 'charging', 'suspend-drain', 'lid'];
function notOn(model: Subject, check: string): Finding | null {
  if (!NVIDIA.has(model.generation)) return null;
  if (QUALCOMM_ONLY.includes(check)) return { state: 'absent', note: 'A Qualcomm processor. NVIDIA machines have no such part.', basis: 'maker-spec' };
  if ((model.form === 'desktop' || model.form === 'devkit') && LAPTOP_ONLY.includes(check)) return { state: 'absent', note: 'A desktop, without a built-in screen, keyboard, touchpad or battery.', basis: 'maker-spec' };
  return null;
}

// `given` marks a finding nobody had to write: a part the machine can't have, by what it is
export type CheckRow = Check & { finding: Finding | null; from: 'model' | 'family' | null; familySource: string | null; given?: true };

/** Every check for one model, with the model's own finding, or else its chip family's. */
export function modelChecks(model: Subject): CheckRow[] {
  const own = findings.models?.[model.id]?.checks ?? {};
  const family = findings.families?.[FAMILY[model.generation] ?? ''];
  return CHECKS.map((check) => {
    if (own[check.id]) return { ...check, finding: own[check.id], from: 'model' as const, familySource: null };
    const lacks = notOn(model, check.id);
    if (lacks) return { ...check, finding: lacks, from: 'model' as const, familySource: null, given: true as const };
    const shared = family?.checks?.[check.id];
    if (shared) return { ...check, finding: shared, from: 'family' as const, familySource: family?.source ?? null };
    return { ...check, finding: null, from: null, familySource: null };
  });
}

export function modelExtra(model: Subject) {
  const entry = findings.models?.[model.id];
  return { details: entry?.details?.trim() ?? null, links: entry?.links ?? [] };
}

/**
 * How many checks a model's own findings cover. Only what was seen on Omarchy counts as known and
 * working. What is reported from elsewhere is counted apart.
 */
export function coverage(model: Subject) {
  const rows = modelChecks(model);
  const own = rows.filter((row) => row.from === 'model' && !row.given);
  const status = own.filter((row) => countsAsStatus(row.finding!));
  return {
    known: status.length,
    works: status.filter((row) => row.finding!.state === 'works').length,
    elsewhere: own.length - status.length,
    total: CHECKS.length,
  };
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
    boots: { outcome: model.outcome, bootable: model.bootable, pieces: model.pieces, omarchy: model.omarchy },
    upstream: model.upstream,
    variants: model.variants.map((v) => ({ chip: v.chip, compatible: v.compatible, deviceTree: v.deviceTree, firmwareNeeded: v.firmwareNeeded, outcome: v.outcome })),
    notes: model.notes,
    details: extra.details,
    links: [...(model.discussion ? [{ title: 'Discussion', url: model.discussion }] : []), ...extra.links],
    reports: model.reports,
    checks: checks.map(({ id, group, title, finding, from }) => ({ id, group, title, finding, from })),
    // Checks nobody has written up for this model, the ones an owner's run helps most with
    // A check known only from another distro or from the tree's author is still wanted on Omarchy
    wanted: checks.filter((row) => !(row.from === 'model' && countsAsStatus(row.finding!)) && !row.optional).map((row) => row.id),
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
    wanted: checks.filter((row) => !(row.from === 'model' && countsAsStatus(row.finding!)) && !row.optional).map((row) => row.id),
  };
}
