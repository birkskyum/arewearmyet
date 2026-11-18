// Runs the reports API (worker/reports.ts) against a local D1 database from Wrangler, with the
// schema from migrations/, and checks what it stores, refuses and counts.

import { mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, before, test } from 'node:test';
import assert from 'node:assert/strict';
import { getPlatformProxy } from 'wrangler';
import { reports } from '../worker/reports.ts';

const YOGA = 'lenovo-yoga-slim-7x-14q8x9';
const TOKEN_A = 'machine-a-'.padEnd(40, 'a');
const TOKEN_B = 'machine-b-'.padEnd(40, 'b');
const ADMIN = 'test-admin-token';

let state: string;
let proxy: Awaited<ReturnType<typeof getPlatformProxy>>;
let env: Record<string, unknown>;
let allow = true;

before(async () => {
  state = mkdtempSync(join(tmpdir(), 'api-test-'));
  proxy = await getPlatformProxy({ configPath: 'wrangler.jsonc', persist: { path: state } });
  const db = (proxy.env as { DB: D1Database }).DB;
  for (const file of readdirSync('migrations').sort()) {
    const sql = readFileSync(join('migrations', file), 'utf8').split('\n').filter((line) => !line.trim().startsWith('--')).map((line) => line.replace(/\s--.*$/, '')).join('\n');
    for (const statement of sql.split(';').map((s) => s.trim()).filter(Boolean)) await db.prepare(statement).run();
  }
  env = {
    ...proxy.env,
    REPORTS: 'on',
    ADMIN_TOKEN: ADMIN,
    REPORT_LIMIT: { limit: async () => ({ success: allow }) },
    // The site's static model file, which the API adds owners' results to
    ASSETS: { fetch: async (url: URL) => (url.pathname === `/data/models/${YOGA}.json` ? Response.json({ id: YOGA, name: 'Yoga Slim 7x' }) : new Response('', { status: 404 })) },
  };
});
after(async () => {
  await proxy.dispose();
  rmSync(state, { recursive: true, force: true });
});

async function call(method: string, path: string, body?: unknown, headers: Record<string, string> = {}) {
  const url = new URL(`https://arewearmyet.com${path}`);
  const request = new Request(url, { method, headers: { 'content-type': 'application/json', ...headers }, body: body === undefined ? undefined : JSON.stringify(body) });
  const response = (await reports(request, env as never, url))!;
  return { status: response.status, body: (await response.json()) as any };
}
const machine = (token: string) => ({ 'x-machine-token': token });
// Runs are told apart by their time, to the millisecond
const tick = () => new Promise((resolve) => setTimeout(resolve, 5));
const run = (results: unknown[], extra: Record<string, unknown> = {}) => ({ mode: 'agent', model: YOGA, kernel: '7.3.0-59-omarchy', results, ...extra });

test('finds a machine by its device tree, then by its model number', async () => {
  assert.equal((await call('GET', '/api/match?compatible=lenovo,yoga-slim7x')).body.model, YOGA);
  const byNumber = await call('GET', '/api/match?product=83ED&vendor=LENOVO');
  assert.equal(byNumber.body.model, YOGA);
  assert.equal(byNumber.body.by, 'model number');
  assert.equal((await call('GET', '/api/match?product=HP%20OmniBook%20X%20Laptop%2014-fe0xxx&vendor=HP')).body.model, 'hp-omnibook-x-14-fe');
  assert.equal((await call('GET', '/api/match?product=Unheard%20Of')).body.model, null);
});

test('refuses a run without a token, with an unknown check or with a bad state', async () => {
  assert.equal((await call('POST', '/api/runs', run([{ check: 'wifi', state: 'works', how: 'measured' }]))).status, 400);
  assert.equal((await call('POST', '/api/runs', run([{ check: 'wifi', state: 'works', how: 'measured' }]), machine('short'))).status, 400);
  assert.match((await call('POST', '/api/runs', run([{ check: 'nope', state: 'works', how: 'owner' }]), machine(TOKEN_A))).body.error, /Unknown check/);
  assert.match((await call('POST', '/api/runs', run([{ check: 'wifi', state: 'fine', how: 'owner' }]), machine(TOKEN_A))).body.error, /state must be/);
  assert.match((await call('POST', '/api/runs', run([{ check: 'wifi', state: 'works', how: 'guess' }]), machine(TOKEN_A))).body.error, /how must be/);
  assert.equal((await call('POST', '/api/runs', run([]), machine(TOKEN_A))).status, 400);
});

test('stores a run, matches its model from the hardware and takes out what identifies the owner', async () => {
  const sent = await call('POST', '/api/runs', {
    mode: 'quick',
    compatible: 'lenovo,yoga-slim7x',
    kernel: '7.3.0-59-omarchy',
    hardware: { maker: 'LENOVO', product: '83ED', serial: 'PF0000' },
    packages: { 'linux-omarchy': '7.3-59', secret: 'x' },
    results: [{ check: 'wifi', state: 'works', how: 'measured', evidence: 'wlan0 6c:6e:07:11:22:33 in /home/birk/x, serial: PF0000' }],
  }, machine(TOKEN_A));
  assert.equal(sent.status, 201);
  assert.equal(sent.body.model, YOGA);
  assert.match(sent.body.run, /^RUN-[A-Z0-9]{10}$/);

  const stored = (await call('GET', `/api/runs/${sent.body.run}`)).body;
  assert.deepEqual(stored.hardware, { maker: 'LENOVO', product: '83ED' });
  assert.deepEqual(stored.packages, { 'linux-omarchy': '7.3-59' });
  assert.equal(stored.results[0].evidence, 'wlan0 xx:xx:xx:xx:xx:xx in /home/user/x, serial [removed]');
});

test('only the machine that made a run can add to it', async () => {
  const sent = await call('POST', '/api/runs', run([{ check: 'speakers', state: 'broken', how: 'owner' }]), machine(TOKEN_A));
  const more = { results: [{ check: 'speakers', state: 'works', how: 'owner', note: 'Both sides.' }] };
  assert.equal((await call('POST', `/api/runs/${sent.body.run}/results`, more, machine(TOKEN_B))).status, 404);
  assert.equal((await call('POST', `/api/runs/${sent.body.run}/results`, more, machine(TOKEN_A))).status, 200);
  const stored = (await call('GET', `/api/runs/${sent.body.run}`)).body;
  assert.deepEqual(stored.results.map((r: any) => [r.check, r.state]), [['speakers', 'works']]);
});

test("counts each machine once, by its newest result, and leaves untested out", async () => {
  await tick();
  await call('POST', '/api/runs', run([{ check: 'bluetooth', state: 'broken', how: 'measured' }, { check: 'camera', state: 'untested', how: 'agent' }]), machine(TOKEN_A));
  await tick();
  await call('POST', '/api/runs', run([{ check: 'bluetooth', state: 'works', how: 'measured' }]), machine(TOKEN_A));
  await tick();
  await call('POST', '/api/runs', run([{ check: 'bluetooth', state: 'broken', how: 'owner', note: 'Never pairs.' }]), machine(TOKEN_B));

  const { owners } = (await call('GET', `/api/models/${YOGA}`)).body;
  assert.equal(owners.machines, 2);
  assert.deepEqual(owners.checks.bluetooth.states, { works: 1, broken: 1 });
  assert.equal(owners.checks.bluetooth.machines, 2);
  assert.equal(owners.checks.bluetooth.latest[0].note, 'Never pairs.');
  assert.equal(owners.checks.camera, undefined);
});

test('takes a link to a fix only from where code is reviewed', async () => {
  assert.equal((await call('POST', '/api/fixes', { model: YOGA, url: 'https://example.com/patch' })).status, 400);
  assert.equal((await call('POST', '/api/fixes', { model: YOGA, url: 'http://github.com/omacom/omarchy-pkgs/pull/1' })).status, 400);
  assert.equal((await call('POST', '/api/fixes', { model: 'no-such-model', url: 'https://github.com/omacom/omarchy-pkgs/pull/1' })).status, 400);
  const fix = await call('POST', '/api/fixes', { model: YOGA, check: 'speakers', url: 'https://github.com/omacom/omarchy-pkgs/pull/1276', note: 'Speaker reset' });
  assert.equal(fix.status, 201);
  const { fixes } = (await call('GET', `/api/models/${YOGA}`)).body;
  assert.deepEqual(fixes.map((f: any) => [f.check, f.url]), [['speakers', 'https://github.com/omacom/omarchy-pkgs/pull/1276']]);
});

test('lets the admin hide a run, a fix or a whole machine', async () => {
  assert.equal((await call('GET', '/api/review')).status, 401);
  assert.equal((await call('GET', '/api/review', undefined, { authorization: 'Bearer wrong' })).status, 401);
  const auth = { authorization: `Bearer ${ADMIN}` };
  const listed = (await call('GET', '/api/review', undefined, auth)).body;
  assert.ok(listed.runs.length >= 4);

  const machineB = listed.runs.find((r: any) => r.results.some((x: any) => x.note === 'Never pairs.')).machine;
  assert.equal((await call('POST', '/api/review/hide', { kind: 'machine', id: machineB, hidden: true })).status, 401);
  assert.equal((await call('POST', '/api/review/hide', { kind: 'machine', id: machineB, hidden: true }, auth)).body.changed, 1);
  let { owners, fixes } = (await call('GET', `/api/models/${YOGA}`)).body;
  assert.equal(owners.machines, 1);
  assert.deepEqual(owners.checks.bluetooth.states, { works: 1 });

  await call('POST', '/api/review/hide', { kind: 'fix', id: listed.fixes[0].id, hidden: true }, auth);
  ({ fixes } = (await call('GET', `/api/models/${YOGA}`)).body);
  assert.equal(fixes.length, 0);

  await call('POST', '/api/review/hide', { kind: 'machine', id: machineB, hidden: false }, auth);
  ({ owners } = (await call('GET', `/api/models/${YOGA}`)).body);
  assert.equal(owners.machines, 2);
});

test('holds back when the limit is reached or reports are closed', async () => {
  allow = false;
  assert.equal((await call('POST', '/api/runs', run([{ check: 'wifi', state: 'works', how: 'measured' }]), machine(TOKEN_A))).status, 429);
  allow = true;
  env.REPORTS = 'off';
  assert.equal((await call('POST', '/api/runs', run([{ check: 'wifi', state: 'works', how: 'measured' }]), machine(TOKEN_A))).status, 503);
  // Reading still works while reports are closed
  assert.equal((await call('GET', `/api/models/${YOGA}`)).status, 200);
  env.REPORTS = 'on';
});
