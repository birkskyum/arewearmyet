// What owners' hw-test runs on Omarchy say about starting a model, as reports beside the ones written
// up in data/models.yaml. A run on Omarchy is an owner at a running system, so the model boots, unless
// the run says the installer didn't start it or the disk password prompt didn't work. A prediction
// from the pieces Omarchy ships can't know either of those, so what an owner saw comes first.

// `from` says a report is what owners' hw-test runs found, or what the model's written-up checks
// (data/findings.yaml) imply, not one written up in data/models.yaml
export type Report = { state: 'works' | 'workaround' | 'problems'; variant?: string; note: string; checked?: string; from?: 'hw-test' | 'findings' };
export type OwnerResult = { state: string; kernel: string | null; os?: string | null; created?: string; needs?: unknown };
export type OwnerModel = { machines: number; checks?: Record<string, { latest?: OwnerResult[] }> };

const START_CHECKS = [
  ['installer', 'the installer from a USB stick'],
  ['disk-unlock', 'the disk password prompt'],
] as const;
const WORD = { works: 'works', partial: 'partly works', broken: "doesn't work" } as const;
// A result is from Omarchy when its run says so, or, for results kept before runs said, when the kernel is Omarchy's
const onOmarchy = (result: OwnerResult) => (result.os ? /^omarchy/i.test(result.os) : /-omarchy/.test(result.kernel ?? ''));

type Found = 'works' | 'partial' | 'broken' | null;
// An installer that fails is a problem. One that partly works, or a disk password prompt that
// doesn't work as it should, still gets an owner to the desktop
const startState = (installer: Found, unlock: Found): Report['state'] =>
  installer === 'broken' ? 'problems' : installer === 'partial' || unlock === 'broken' || unlock === 'partial' ? 'workaround' : 'works';
const startLines = (installer: Found, unlock: Found) =>
  START_CHECKS.map(([, words], i) => ({ words, state: [installer, unlock][i] }))
    .filter((check) => check.state)
    .map((check) => `${check.words} ${WORD[check.state!]}`)
    .join(', and ');

/** The month of the newest run on Omarchy that has a result for each start check. */
export function startRuns(model: OwnerModel | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [check] of START_CHECKS) {
    const months = (model?.checks?.[check]?.latest ?? []).filter((result) => result.state !== 'untested' && onOmarchy(result)).map((result) => result.created?.slice(0, 7) ?? '').sort();
    if (months.length && months[months.length - 1]) out[check] = months[months.length - 1];
  }
  return out;
}

export function startReports(model: OwnerModel | undefined): Report[] {
  const checks = model?.checks ?? {};
  const here = (check: string) => (checks[check]?.latest ?? []).filter((result) => result.state !== 'untested' && onOmarchy(result));
  const runs = Object.keys(checks).flatMap(here);
  if (!runs.length) return [];
  // A part that works because of a change made on that computer doesn't work on Omarchy as it ships
  const found = (check: string) => {
    const states = here(check).map((result) => (result.needs && result.state === 'works' ? 'partial' : result.state)).filter((state) => state !== 'absent');
    return states.length === 0 ? null : states.every((state) => state === 'works') ? 'works' : states.every((state) => state === 'broken') ? 'broken' : 'partial';
  };
  const [installer, unlock] = START_CHECKS.map(([check]) => found(check)) as Found[];
  const state = startState(installer, unlock);
  const newest = [...runs].sort((a, b) => (b.created ?? '').localeCompare(a.created ?? ''))[0];
  const kernel = newest.kernel ? ` on linux-omarchy ${newest.kernel.replace(/-omarchy.*$/, '')}` : '';
  const lines = startLines(installer, unlock);
  const note = `An owner runs Omarchy on it and tested it with hw-test${kernel}.${lines ? ` There, ${lines}.` : ''}`;
  return [{ state, note, checked: newest.created?.slice(0, 7), from: 'hw-test' }];
}

// A check written up as seen on Omarchy on a model (data/findings.yaml) says an owner had Omarchy running
// on it, whatever the check found. What the maker's description says, or what was seen on another
// distro, says nothing about that.
export type Finding = { state: string; basis?: string; checked?: string };
// `since` holds the month of the newest run that has a result for a start check. A finding of that
// check from the same month or earlier is out of date, as a fix may have come in since it was written
export function findingReports(checks: Record<string, Finding> | undefined, since: Record<string, string> = {}): Report[] {
  const seen = Object.entries(checks ?? {}).filter(([, finding]) => (finding.basis ?? 'omarchy') === 'omarchy' && ['works', 'partial', 'broken'].includes(finding.state));
  if (!seen.length) return [];
  const current = ([id, finding]: [string, Finding]) => !since[id] || (finding.checked ?? '') > since[id];
  const found = (check: string) => (seen.filter(current).find(([id]) => id === check)?.[1].state ?? null) as Found;
  const [installer, unlock] = START_CHECKS.map(([check]) => found(check));
  const lines = startLines(installer, unlock);
  const checked = seen.map(([, finding]) => finding.checked).filter(Boolean).sort().pop();
  const note = `An owner has Omarchy running on it, going by the ${seen.length === 1 ? 'check' : `${seen.length} checks`} written up from it.${lines ? ` There, ${lines}.` : ''}`;
  return [{ state: startState(installer, unlock), note, checked, from: 'findings' }];
}
