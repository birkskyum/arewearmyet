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
// The models the stand-in for the site's static files knows
const KNOWN = [YOGA, 'hp-omnibook-x-14-fe'];
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
    ASSETS: { fetch: async (url: URL) => (KNOWN.some((id) => url.pathname === `/data/models/${id}.json`) ? Response.json({ id: url.pathname.split('/').pop()!.replace('.json', '') }) : new Response('', { status: 404 })) },
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

test('notices a check that turned from working to broken on a newer kernel, and one that got fixed', async () => {
  const token = 'machine-c-'.padEnd(40, 'c');
  const send = async (kernel: string, results: unknown[]) => {
    await tick();
    await call('POST', '/api/runs', { ...run(results), model: 'hp-omnibook-x-14-fe', kernel }, machine(token));
  };
  await send('7.3.0-59-omarchy', [{ check: 'suspend', state: 'works', how: 'owner' }, { check: 'camera', state: 'broken', how: 'owner' }]);
  await send('7.3.0-67-omarchy', [{ check: 'suspend', state: 'works', how: 'owner' }, { check: 'camera', state: 'broken', how: 'owner' }]);
  await send('7.3.0-72-omarchy', [{ check: 'suspend', state: 'broken', how: 'owner' }, { check: 'camera', state: 'works', how: 'owner' }]);
  await send('7.3.0-75-omarchy', [{ check: 'suspend', state: 'broken', how: 'owner' }, { check: 'wifi', state: 'works', how: 'measured' }]);

  const { owners } = (await call('GET', '/api/models/hp-omnibook-x-14-fe')).body;
  assert.deepEqual(owners.checks.suspend.states, { broken: 1 });
  assert.equal(owners.checks.suspend.regressed.machines, 1);
  assert.equal(owners.checks.suspend.regressed.from, '7.3.0-67-omarchy');
  assert.equal(owners.checks.suspend.regressed.to, '7.3.0-72-omarchy');
  assert.equal(owners.checks.suspend.fixed, undefined);
  assert.equal(owners.checks.camera.fixed.from, '7.3.0-67-omarchy');
  assert.equal(owners.checks.camera.fixed.to, '7.3.0-72-omarchy');
  assert.equal(owners.checks.wifi.regressed, undefined);
});

test("gives every model's owners' results at once, with the machines that matched no model", async () => {
  await call('POST', '/api/runs', { mode: 'quick', hardware: { maker: 'Acme', product: 'Unheard Of 14' }, compatible: 'acme,unheard', results: [{ check: 'wifi', state: 'works', how: 'measured' }] }, machine('machine-d-'.padEnd(40, 'd')));
  const { models, unmatched } = (await call('GET', '/api/owners')).body;
  assert.ok(models[YOGA].machines >= 2);
  assert.equal(models['hp-omnibook-x-14-fe'].checks.suspend.regressed.machines, 1);
  assert.deepEqual(unmatched.map((u: any) => [u.hardware.maker, u.hardware.product, u.compatible, u.machines]), [['Acme', 'Unheard Of 14', 'acme,unheard', 1]]);
});

test('names a machine by its hardware IDs, also when it booted with another model\'s tree', async () => {
  const index = JSON.parse(readFileSync('worker/models.json', 'utf8')) as { chids: Record<string, number[]>; models: { id: string }[] };
  const only = (id: string) => Object.entries(index.chids).find(([, owners]) => owners.length === 1 && index.models[owners[0]].id === id)![0];

  // The Yoga's own, as systemd-analyze chid gives them on one
  const yoga = (await call('GET', '/api/match?chid=0:5315f3e1-1fa9-5592-bf05-75e8124d1f8a&chid=4:d27cf20e-e185-578e-bd46-f4cc3a718bb2')).body;
  assert.equal(yoga.model, YOGA);
  assert.equal(yoga.by, 'hardware IDs');

  // The ProBook 4 G1q boots the EliteBook 6 G1q's tree, so only its hardware IDs tell them apart
  const probook = only('hp-probook-4-g1q');
  assert.equal((await call('GET', `/api/match?chid=3:${probook}&compatible=hp,elitebook-6-g1q`)).body.model, 'hp-probook-4-g1q');
  assert.equal((await call('GET', '/api/match?compatible=hp,elitebook-6-g1q')).body.model, 'hp-elitebook-6-g1q');

  // An ID made from the maker alone says nothing about the model
  const lenovo = '6de5d951-d755-576b-bd09-c5cf66b27234';
  assert.ok(index.chids[lenovo].length > 1);
  assert.equal((await call('GET', `/api/match?chid=14:${lenovo}`)).body.model, null);
  assert.equal((await call('GET', `/api/match?chid=14:${only(YOGA)}`)).body.model, null);

  // A run is matched and stored the same way
  const sent = await call('POST', '/api/runs', { mode: 'quick', compatible: 'hp,elitebook-6-g1q', chids: [`3:${probook}`, 'not-an-id'], results: [{ check: 'wifi', state: 'works', how: 'measured' }] }, machine('machine-e-'.padEnd(40, 'e')));
  assert.equal(sent.body.model, 'hp-probook-4-g1q');
  assert.deepEqual((await call('GET', `/api/runs/${sent.body.run}`)).body.chids, [`3:${probook}`]);
});

test('copies the database to the bucket, and drops the copy from a month before unless it is a first of the month', async () => {
  const { backup } = await import('../worker/backup.ts');
  const bucket = (env as { FIRMWARE: R2Bucket }).FIRMWARE;
  await bucket.put('backups/reports/2026-10-18.json', '{}');
  await bucket.put('backups/reports/2026-10-01.json', '{}');

  const done = await backup(env as never, new Date('2026-11-18T04:23:00Z'));
  assert.equal(done.key, 'backups/reports/2026-11-18.json');
  assert.ok(done.rows.runs >= 5 && done.rows.results >= 5 && done.rows.machines >= 3 && done.rows.fixes === 1);
  const stored = JSON.parse(await (await bucket.get(done.key))!.text());
  assert.equal(stored.tables.runs.length, done.rows.runs);
  assert.equal(await bucket.head('backups/reports/2026-10-18.json'), null);
  assert.notEqual(await bucket.head('backups/reports/2026-10-01.json'), null);

  // The first of a month isn't dropped when its turn comes
  await backup(env as never, new Date('2026-11-01T04:23:00Z'));
  await backup(env as never, new Date('2026-12-02T04:23:00Z'));
  assert.notEqual(await bucket.head('backups/reports/2026-11-01.json'), null);
});
