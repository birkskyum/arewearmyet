// Compares owners' reports (GET /api/owners) with what's written up in data/findings.yaml and
// writes a work list: regressions, findings owners contradict, checks with enough agreeing reports
// to write up, first reports and machines the board doesn't know. Run with `pnpm digest`.
//
// `pnpm digest --issue` keeps one GitHub issue (label "digest") up to date with it, and comments
// on that issue only when something new turned up, so it notifies when there's news and not nightly.

import { execFileSync } from 'node:child_process';
import { readFileSync, realpathSync } from 'node:fs';
import { parse } from 'yaml';

const SITE = 'https://arewearmyet.com';
const REPO = 'birkskyum/arewearmyet';

type Change = { machines: number; from: string | null; to: string | null; since: string };
type OwnerCheck = { machines: number; states: Record<string, number>; how: Record<string, number>; latest: { state: string; note: string | null; kernel: string | null }[]; regressed?: Change; fixed?: Change };
export type Owners = {
  models: Record<string, { machines: number; checks: Record<string, OwnerCheck> }>;
  unmatched: { hardware: Record<string, string>; compatible: string | null; machines: number; runs: number; latest: string }[];
};
type Finding = { state: string; note?: string };
export type Findings = { families?: Record<string, { checks?: Record<string, Finding> }>; models?: Record<string, { checks?: Record<string, Finding> }> };
export type Known = { checks: { id: string; title: string }[]; models: { id: string; vendor: string; name: string; generation: string }[] };

export type Digest = {
  machines: number;
  models: number;
  regressions: string[];
  contradictions: string[];
  fixed: string[];
  ready: string[];
  first: string[];
  unmatched: string[];
};

const bad = (states: Record<string, number>) => (states.broken ?? 0) + (states.partial ?? 0);
const counts = (states: Record<string, number>) => Object.entries(states).map(([state, n]) => `${n} ${state}`).join(', ');
const kernels = (change: Change) => `${change.from ?? 'an earlier kernel'} to ${change.to ?? 'a newer kernel'}`;

export function digest(owners: Owners, findings: Findings, known: Known): Digest {
  const title = new Map(known.checks.map((check) => [check.id, check.title]));
  const out: Digest = { machines: 0, models: 0, regressions: [], contradictions: [], fixed: [], ready: [], first: [], unmatched: [] };

  for (const model of known.models) {
    const reported = owners.models[model.id];
    if (!reported) continue;
    out.machines += reported.machines;
    out.models++;
    const name = `[${model.vendor} ${model.name}](${SITE}/models/${model.id})`;
    const own = findings.models?.[model.id]?.checks ?? {};
    const family = findings.families?.[model.generation]?.checks ?? {};
    const firsts: string[] = [];

    for (const [id, check] of Object.entries(reported.checks)) {
      const what = title.get(id) ?? id;
      const finding = own[id] ?? family[id];
      const where = own[id] ? 'its finding' : 'its chip family\'s finding';
      const works = check.states.works ?? 0;

      if (check.regressed) out.regressions.push(`- ${name}: **${what}** stopped working from ${kernels(check.regressed)} on ${check.regressed.machines} machine${check.regressed.machines === 1 ? '' : 's'} (${check.regressed.since.slice(0, 10)})`);
      if (check.fixed && finding && finding.state !== 'works') out.fixed.push(`- ${name}: **${what}** works since ${check.fixed.to ?? 'a newer kernel'} on ${check.fixed.machines} machine${check.fixed.machines === 1 ? '' : 's'}, but ${where} still says ${finding.state}`);

      if (finding) {
        // A finding and the owners disagree: it says works and someone has it broken, or it says broken and it works for them
        if (finding.state === 'works' && bad(check.states) > 0) out.contradictions.push(`- ${name}: **${what}** is written up as working, owners report ${counts(check.states)}${check.latest[0]?.note ? ` ("${check.latest[0].note}")` : ''}`);
        else if (finding.state === 'broken' && works > 0 && !check.fixed) out.contradictions.push(`- ${name}: **${what}** is written up as broken in ${where}, owners report ${counts(check.states)}`);
        if (own[id]) continue;
      }
      // Nothing written up for the model itself yet
      const states = Object.keys(check.states);
      if (check.machines >= 2 && states.length === 1) out.ready.push(`- ${name}: **${what}** ${states[0]} on ${check.machines} machines. \`${id}: { state: ${states[0]}, checked: ${new Date().toISOString().slice(0, 7)} }\``);
      else firsts.push(`${what.toLowerCase()} ${counts(check.states)}`);
    }
    if (firsts.length) out.first.push(`- ${name} (${reported.machines} machine${reported.machines === 1 ? '' : 's'}): ${firsts.join('; ')}`);
  }

  for (const machine of owners.unmatched) {
    const label = [machine.hardware.maker, machine.hardware.product, machine.hardware.family].filter(Boolean).join(' ') || 'An unnamed machine';
    out.unmatched.push(`- ${label}${machine.hardware.sku ? `, SKU ${machine.hardware.sku}` : ''}${machine.compatible ? `, device tree \`${machine.compatible}\`` : ''}: ${machine.runs} run${machine.runs === 1 ? '' : 's'} from ${machine.machines} machine${machine.machines === 1 ? '' : 's'}`);
  }
  return out;
}

const SECTIONS: [keyof Digest, string, string][] = [
  ['regressions', 'Regressions', 'A machine reported the check working, then not working on a newer kernel.'],
  ['contradictions', 'Findings owners contradict', 'What `data/findings.yaml` says and what owners report disagree.'],
  ['fixed', 'Fixed, but still written up as broken', 'Owners report it working again; the finding wants an update.'],
  ['ready', 'Ready to write up', 'Two or more machines agree and the model has no finding of its own for the check. The line is ready for `data/findings.yaml`.'],
  ['first', 'First reports', 'One machine so far, or machines that disagree, for checks the model has no finding for.'],
  ['unmatched', "Machines the board doesn't know", 'Runs that matched no model. They need a model entry, a model number or a hardware report.'],
];

export function render(result: Digest): string {
  const items = SECTIONS.reduce((n, [key]) => n + (result[key] as string[]).length, 0);
  if (!items) return '';
  const lines = [`Owners' reports from ${result.machines} machine${result.machines === 1 ? '' : 's'} across ${result.models} model${result.models === 1 ? '' : 's'}, compared with \`data/findings.yaml\`. This issue is rewritten when the reports change. [Review reports](${SITE}/review).`];
  for (const [key, heading, about] of SECTIONS) {
    const list = result[key] as string[];
    if (!list.length) continue;
    lines.push('', `## ${heading}`, '', about, '', ...list);
  }
  return lines.join('\n') + '\n';
}

/** The list items of a digest that an earlier digest didn't have. */
export function added(body: string, before: string): string[] {
  const old = new Set(before.split('\n').filter((line) => line.startsWith('- ')));
  return body.split('\n').filter((line) => line.startsWith('- ') && !old.has(line));
}

const gh = (...args: string[]) => execFileSync('gh', args, { encoding: 'utf8' });

async function main() {
  const response = await fetch(`${SITE}/api/owners`);
  if (!response.ok) throw new Error(`${response.status} from ${SITE}/api/owners`);
  const owners = (await response.json()) as Owners;
  const status = JSON.parse(readFileSync('src/data/status.json', 'utf8'));
  const known: Known = { checks: parse(readFileSync('data/checks.yaml', 'utf8')).checks, models: status.models };
  const body = render(digest(owners, parse(readFileSync('data/findings.yaml', 'utf8')), known));

  if (!process.argv.includes('--issue')) {
    console.log(body || "Nothing to report: no owners' reports that add to or disagree with the findings.");
    return;
  }
  const open = JSON.parse(gh('issue', 'list', '--repo', REPO, '--label', 'digest', '--state', 'open', '--json', 'number,body', '--limit', '1')) as { number: number; body: string }[];
  if (!open.length) {
    // The first digest opens the issue; nothing to say means no issue yet
    if (!body) return console.log('Nothing to report, and no digest issue yet.');
    gh('label', 'create', 'digest', '--repo', REPO, '--description', "The nightly comparison of owners' reports with the findings", '--color', '9ece6a', '--force');
    console.log(gh('issue', 'create', '--repo', REPO, '--title', "Owners' reports digest", '--label', 'digest', '--body', body).trim());
    return;
  }
  const issue = open[0];
  const text = body || "Owners' reports add nothing to the findings right now, and disagree with none of them.\n";
  if (text.trim() === issue.body.trim()) return console.log(`Digest #${issue.number} is up to date.`);
  gh('issue', 'edit', String(issue.number), '--repo', REPO, '--body', text);
  const news = added(text, issue.body);
  if (news.length) gh('issue', 'comment', String(issue.number), '--repo', REPO, '--body', `New since the last digest:\n\n${news.join('\n')}\n`);
  console.log(`Digest #${issue.number} updated, ${news.length} new item${news.length === 1 ? '' : 's'}.`);
}

if (process.argv[1] && realpathSync(process.argv[1]) === import.meta.filename) await main();
