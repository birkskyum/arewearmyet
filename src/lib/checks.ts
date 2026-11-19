// The checks past booting (data/checks.yaml) and what's known about them per model
// (data/findings.yaml), joined with the board's own model data for the model pages
// and the JSON the API serves.
import { parse } from 'yaml';
import checksRaw from '../../data/checks.yaml?raw';
import findingsRaw from '../../data/findings.yaml?raw';
import status from '../data/status.json';
import type { Model } from './labels';

export type CheckState = 'works' | 'partial' | 'broken' | 'untested' | 'absent';
export type Check = { id: string; group: string; title: string; works: string; probe?: string; person?: string; debug?: string; optional?: boolean };
export type Group = { id: string; title: string };
/**
 * What a finding rests on. Only a result on Omarchy says how the model does under Omarchy. One from
 * another distro, or from the device tree's author, is a hint that it can work, and is shown as that.
 */
export type Basis = 'omarchy' | 'other-distro' | 'tree-author' | 'maker-spec';
export type Finding = { state: CheckState; note?: string; version?: string; checked?: string; source?: string; basis?: Basis; where?: string; quote?: string };

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

/** The models that get pages and checks: Snapdragon PCs that start through UEFI. */
export function pageModels(): Model[] {
  return status.models.filter((model: Model) => model.boot === 'uefi');
}

export type CheckRow = Check & { finding: Finding | null; from: 'model' | 'family' | null; familySource: string | null };

/** Every check for one model, with the model's own finding, or else its chip family's. */
export function modelChecks(model: Model): CheckRow[] {
  const own = findings.models?.[model.id]?.checks ?? {};
  const family = findings.families?.[FAMILY[model.generation] ?? ''];
  return CHECKS.map((check) => {
    if (own[check.id]) return { ...check, finding: own[check.id], from: 'model' as const, familySource: null };
    const shared = family?.checks?.[check.id];
    if (shared) return { ...check, finding: shared, from: 'family' as const, familySource: family?.source ?? null };
    return { ...check, finding: null, from: null, familySource: null };
  });
}

export function modelExtra(model: Model) {
  const entry = findings.models?.[model.id];
  return { details: entry?.details?.trim() ?? null, links: entry?.links ?? [] };
}

/**
 * How many checks a model's own findings cover. Only what was seen on Omarchy counts as known and
 * working. What is reported from elsewhere is counted apart.
 */
export function coverage(model: Model) {
  const rows = modelChecks(model);
  const own = rows.filter((row) => row.from === 'model');
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
