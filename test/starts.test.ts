// What owners' hw-test runs say about starting a model (scripts/starts.ts)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { findingReports, startReports, startRuns, type OwnerModel } from '../scripts/starts.ts';

const run = (state: string, more: Record<string, unknown> = {}) => ({ state, kernel: '7.3.0-72-omarchy', os: 'Omarchy', created: '2026-11-19T18:47:44.644Z', ...more });
const model = (checks: Record<string, ReturnType<typeof run>[]>): OwnerModel => ({ machines: 1, checks: Object.fromEntries(Object.entries(checks).map(([id, latest]) => [id, { latest }])) });

test('a run on Omarchy that found nothing wrong with starting says the model boots', () => {
  const [report, ...rest] = startReports(model({ installer: [run('works')], 'disk-unlock': [run('works')], speakers: [run('broken')] }));
  assert.equal(rest.length, 0);
  assert.equal(report.state, 'works');
  assert.equal(report.from, 'hw-test');
  assert.equal(report.checked, '2026-11');
  assert.equal(report.note, 'An owner runs Omarchy on it and tested it with hw-test on linux-omarchy 7.3.0-72. There, the installer from a USB stick works, and the disk password prompt works.');
  // Neither start check answered, the owner is still at a running Omarchy
  assert.equal(startReports(model({ wifi: [run('works')] }))[0].state, 'works');
  assert.equal(startReports(model({ wifi: [run('works')] }))[0].note, 'An owner runs Omarchy on it and tested it with hw-test on linux-omarchy 7.3.0-72.');
});

test('a disk password prompt that fails is a workaround, and an installer that fails is a problem', () => {
  assert.equal(startReports(model({ installer: [run('works')], 'disk-unlock': [run('broken')] }))[0].state, 'workaround');
  assert.match(startReports(model({ installer: [run('works')], 'disk-unlock': [run('broken')] }))[0].note, /the disk password prompt doesn't work\.$/);
  assert.equal(startReports(model({ installer: [run('partial')], 'disk-unlock': [run('works')] }))[0].state, 'workaround');
  assert.equal(startReports(model({ installer: [run('broken')], 'disk-unlock': [run('works')] }))[0].state, 'problems');
  // One that works only after a change made on that computer doesn't work on Omarchy as it ships
  assert.equal(startReports(model({ 'disk-unlock': [run('works', { needs: { text: 'modules added by hand' } })] }))[0].state, 'workaround');
});

test('runs on another distro, untested answers and models nobody ran on say nothing', () => {
  assert.deepEqual(startReports(undefined), []);
  assert.deepEqual(startReports({ machines: 0 }), []);
  assert.deepEqual(startReports(model({ installer: [run('untested')] })), []);
  assert.deepEqual(startReports(model({ installer: [run('broken', { os: 'Fedora Linux', kernel: '6.19.0' })] })), []);
  // Before runs said which system they were on, Omarchy's kernel told
  assert.equal(startReports(model({ installer: [run('works', { os: null })] }))[0].state, 'works');
});

test('a check written up from Omarchy on the model says an owner has it running', () => {
  const [report, ...rest] = findingReports({ speakers: { state: 'broken', checked: '2026-10' }, wifi: { state: 'works', basis: 'omarchy', checked: '2026-11' } });
  assert.equal(rest.length, 0);
  assert.equal(report.state, 'works');
  assert.equal(report.from, 'findings');
  assert.equal(report.checked, '2026-11');
  assert.equal(report.note, 'An owner has Omarchy running on it, going by the 2 checks written up from it.');
  // The start checks decide as they do for a run
  assert.equal(findingReports({ 'disk-unlock': { state: 'broken' } })[0].state, 'workaround');
  assert.equal(findingReports({ installer: { state: 'broken' } })[0].state, 'problems');
  assert.match(findingReports({ 'disk-unlock': { state: 'broken' } })[0].note, /going by the check written up from it\. There, the disk password prompt doesn't work\.$/);
});

test("the maker's description, another distro and untested checks say nothing about starting", () => {
  assert.deepEqual(findingReports(undefined), []);
  assert.deepEqual(findingReports({ touchscreen: { state: 'absent', basis: 'maker-spec' }, camera: { state: 'untested', basis: 'maker-spec' } }), []);
  assert.deepEqual(findingReports({ wifi: { state: 'works', basis: 'other-distro' }, gpu: { state: 'works', basis: 'tree-author' } }), []);
  assert.deepEqual(findingReports({ wifi: { state: 'untested' }, lid: { state: 'absent' } }), []);
});

test('a run that has a newer result for the password prompt puts an older finding of it aside', () => {
  const owner = model({ 'disk-unlock': [run('works')], wifi: [run('works')] });
  assert.deepEqual(startRuns(owner), { 'disk-unlock': '2026-11' });
  assert.deepEqual(startRuns(model({ installer: [run('works', { os: 'Fedora Linux', kernel: '6.19.0' })] })), {});
  const findings = { 'disk-unlock': { state: 'partial', checked: '2026-10' }, speakers: { state: 'broken', checked: '2026-10' } };
  // By itself the finding makes it a workaround. With the run it only says an owner has the model running
  assert.equal(findingReports(findings)[0].state, 'workaround');
  assert.equal(findingReports(findings, startRuns(owner))[0].state, 'works');
  assert.equal(findingReports(findings, startRuns(owner))[0].note, 'An owner has Omarchy running on it, going by the 2 checks written up from it.');
  // A finding written after the run still counts, and so does one of a check the run didn't answer
  assert.equal(findingReports({ 'disk-unlock': { state: 'broken', checked: '2026-12' } }, startRuns(owner))[0].state, 'workaround');
  assert.equal(findingReports({ installer: { state: 'broken', checked: '2026-10' } }, startRuns(owner))[0].state, 'problems');
});
