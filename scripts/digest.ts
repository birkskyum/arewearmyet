// Compares owners' reports (GET /api/owners) with what's written up in data/findings.yaml and
// writes a work list of regressions, findings owners contradict, parts that work elsewhere and not
// on Omarchy, checks with enough agreeing reports to write up, first reports and machines the board
// doesn't know. Run with `pnpm digest`.
//
// Only Omarchy counts on both sides, as on the board. An owner's result from another distro is left
// out, and a finding from another distro or from a device tree's author is a lead, never a status.
//
// `pnpm digest --issue` keeps one GitHub issue (label "digest") up to date with it, and comments
// on that issue only when something new turned up, so it notifies when there's news and not nightly.

import { execFileSync } from 'node:child_process';
import { readFileSync, realpathSync } from 'node:fs';
import { parse } from 'yaml';

const SITE = 'https://arewearmyet.com';
const REPO = 'birkskyum/arewearmyet';

type Change = { machines: number; from: string | null; to: string | null; since: string };
type OwnerCheck = { machines: number; states: Record<string, number>; how: Record<string, number>; latest: { state: string; note: string | null; kernel: string | null; os?: string | null }[]; regressed?: Change; fixed?: Change };
export type Owners = {
  models: Record<string, { machines: number; checks: Record<string, OwnerCheck> }>;
  unmatched: { hardware: Record<string, string>; compatible: string | null; chids?: string[]; machines: number; runs: number; latest: string }[];
};
type Finding = { state: string; note?: string; basis?: string; where?: string };
export type Findings = { families?: Record<string, { checks?: Record<string, Finding> }>; models?: Record<string, { checks?: Record<string, Finding> }> };
export type Known = { checks: { id: string; title: string }[]; models: { id: string; vendor: string; name: string; generation: string }[] };

export type Digest = {
  machines: number;
  models: number;
  regressions: string[];
  contradictions: string[];
  fixed: string[];
  elsewhere: string[];
  ready: string[];
  first: string[];
  unmatched: string[];
};

const bad = (states: Record<string, number>) => (states.broken ?? 0) + (states.partial ?? 0);
const counts = (states: Record<string, number>) => Object.entries(states).map(([state, n]) => `${n} ${state}`).join(', ');
// A result is from Omarchy when its run says so, or, for results kept before runs said, when the kernel is Omarchy's
const onOmarchy = (result: { os?: string | null; kernel: string | null }) => (result.os ? /^omarchy/i.test(result.os) : /-omarchy/.test(result.kernel ?? ''));
// A finding that says how the part does on Omarchy. One that only says the maker lists the part doesn't
const status = (finding?: Finding) => (finding && (!finding.basis || finding.basis === 'omarchy' || finding.basis === 'maker-spec') && finding.state !== 'untested' ? finding : undefined);
// A finding from another distro or from the device tree's author
const lead = (finding?: Finding) => (finding && (finding.basis === 'other-distro' || finding.basis === 'tree-author') ? finding : undefined);
const kernels = (change: Change) => `${change.from ?? 'an earlier kernel'} to ${change.to ?? 'a newer kernel'}`;

export function digest(owners: Owners, findings: Findings, known: Known): Digest {
  const title = new Map(known.checks.map((check) => [check.id, check.title]));
  const out: Digest = { machines: 0, models: 0, regressions: [], contradictions: [], fixed: [], elsewhere: [], ready: [], first: [], unmatched: [] };

  for (const model of known.models) {
    const reported = owners.models[model.id];
    // A model whose only results are from another distro has nothing to compare
    const tested = Math.max(0, ...Object.values(reported?.checks ?? {}).map((check) => check.latest.filter(onOmarchy).length));
    if (!reported || !tested) continue;
    out.machines += tested;
    out.models++;
    const name = `[${model.vendor} ${model.name}](${SITE}/models/${model.id})`;
    const own = findings.models?.[model.id]?.checks ?? {};
    const firsts: string[] = [];

    for (const [id, check] of Object.entries(reported.checks)) {
      const what = title.get(id) ?? id;
      // What the machines saw on Omarchy, one result a machine
      const here = check.latest.filter((result) => onOmarchy(result) && result.state !== 'untested');
      if (!here.length) continue;
      const states: Record<string, number> = {};
      for (const result of here) states[result.state] = (states[result.state] ?? 0) + 1;
      const finding = status(own[id]);
      const hint = lead(own[id]);

      if (check.regressed) out.regressions.push(`- ${name}, **${what}** stopped working from ${kernels(check.regressed)} on ${check.regressed.machines} machine${check.regressed.machines === 1 ? '' : 's'} (${check.regressed.since.slice(0, 10)})`);
      if (check.fixed && finding && finding.state !== 'works') out.fixed.push(`- ${name}, **${what}** works since ${check.fixed.to ?? 'a newer kernel'} on ${check.fixed.machines} machine${check.fixed.machines === 1 ? '' : 's'}, but its finding still says ${finding.state}`);

      if (finding) {
        // A finding and the owners disagree. It says works and someone has it broken, or it says broken and it works for them
        if (finding.state === 'works' && bad(states) > 0) out.contradictions.push(`- ${name}, **${what}** is written up as working, owners report ${counts(states)}${here[0]?.note ? ` ("${here[0].note}")` : ''}`);
        else if (finding.state === 'broken' && (states.works ?? 0) > 0 && !check.fixed) out.contradictions.push(`- ${name}, **${what}** is written up as broken, owners report ${counts(states)}`);
        continue;
      }
      // Working somewhere else and not here is where a fix to bring over may be
      if (hint?.state === 'works' && bad(states) > 0) out.elsewhere.push(`- ${name}, **${what}** works elsewhere (${hint.where ?? (hint.basis === 'tree-author' ? "the device tree author's machine" : 'another distro')}), and owners on Omarchy report ${counts(states)}`);
      // Nothing written up for the model on Omarchy yet
      if (here.length >= 2 && Object.keys(states).length === 1) out.ready.push(`- ${name}, **${what}** ${here[0].state} on ${here.length} machines. \`${id}: { state: ${here[0].state}, checked: ${new Date().toISOString().slice(0, 7)} }\``);
      else firsts.push(`${what.toLowerCase()} (${counts(states)})`);
    }
    if (firsts.length) out.first.push(`- ${name} (${tested} machine${tested === 1 ? '' : 's'}) with ${firsts.join(', ')}`);
  }

  for (const machine of owners.unmatched) {
    const label = [machine.hardware.maker, machine.hardware.product, machine.hardware.family].filter(Boolean).join(' ') || 'An unnamed machine';
    out.unmatched.push(`- ${label}${machine.hardware.sku ? `, SKU ${machine.hardware.sku}` : ''}${machine.compatible ? `, device tree \`${machine.compatible}\`` : ''}${machine.chids?.length ? `, ${machine.chids.length} hardware IDs sent` : ''}, with ${machine.runs} run${machine.runs === 1 ? '' : 's'} from ${machine.machines} machine${machine.machines === 1 ? '' : 's'}`);
  }
  return out;
}

const SECTIONS: [keyof Digest, string, string][] = [
  ['regressions', 'Regressions', 'A machine reported the check working, then not working on a newer kernel.'],
  ['contradictions', 'Findings owners contradict', 'What `data/findings.yaml` says and what owners report disagree.'],
  ['fixed', 'Fixed, but still written up as broken', 'Owners report it working again, so the finding wants an update.'],
  ['elsewhere', 'Working elsewhere, not on Omarchy', "Someone has it working on another distro or on the device tree author's machine, and owners on Omarchy don't. A fix may exist to bring over."],
  ['ready', 'Ready to write up', 'Two or more machines agree and the model has no finding from Omarchy for the check. The line is ready for `data/findings.yaml`.'],
  ['first', 'First reports', 'One machine so far, or machines that disagree, for checks the model has no finding from Omarchy for.'],
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
    console.log(body || "Nothing to report. No owners' reports add to or disagree with the findings.");
    return;
  }
  const open = JSON.parse(gh('issue', 'list', '--repo', REPO, '--label', 'digest', '--state', 'open', '--json', 'number,body', '--limit', '1')) as { number: number; body: string }[];
  if (!open.length) {
    // The first digest opens the issue, and nothing to say means no issue yet
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
