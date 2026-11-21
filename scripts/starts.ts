// What owners' hw-test runs on Omarchy say about starting a model, as reports beside the ones written
// up in data/models.yaml. A run on Omarchy is an owner at a running system, so the model boots, unless
// the run says the installer didn't start it or the disk password prompt didn't work. A prediction
// from the pieces Omarchy ships can't know either of those, so what an owner saw comes first.

// `from` says a report is what owners' hw-test runs found, not one written up in data/models.yaml
export type Report = { state: 'works' | 'workaround' | 'problems'; variant?: string; note: string; checked?: string; from?: 'hw-test' };
export type OwnerResult = { state: string; kernel: string | null; os?: string | null; created?: string; needs?: unknown };
export type OwnerModel = { machines: number; checks?: Record<string, { latest?: OwnerResult[] }> };

const START_CHECKS = [
  ['installer', 'the installer from a USB stick'],
  ['disk-unlock', 'the disk password prompt'],
] as const;
const WORD = { works: 'works', partial: 'partly works', broken: "doesn't work" } as const;
// A result is from Omarchy when its run says so, or, for results kept before runs said, when the kernel is Omarchy's
const onOmarchy = (result: OwnerResult) => (result.os ? /^omarchy/i.test(result.os) : /-omarchy/.test(result.kernel ?? ''));

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
  const [installer, unlock] = START_CHECKS.map(([check]) => found(check));
  const state = installer === 'broken' ? 'problems' : installer === 'partial' || unlock === 'broken' || unlock === 'partial' ? 'workaround' : 'works';
  const said = START_CHECKS.map(([, words], i) => ({ words, state: [installer, unlock][i] })).filter((check) => check.state);
  const newest = [...runs].sort((a, b) => (b.created ?? '').localeCompare(a.created ?? ''))[0];
  const kernel = newest.kernel ? ` on linux-omarchy ${newest.kernel.replace(/-omarchy.*$/, '')}` : '';
  const lines = said.map((check) => `${check.words} ${WORD[check.state as keyof typeof WORD]}`).join(', and ');
  const note = `An owner runs Omarchy on it and tested it with hw-test${kernel}.${lines ? ` There, ${lines}.` : ''}`;
  return [{ state, note, checked: newest.created?.slice(0, 7), from: 'hw-test' }];
}
