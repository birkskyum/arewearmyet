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

test('says which system each result was seen on, so a result from another distro is not taken for Omarchy', async () => {
  const sent = await call('POST', '/api/runs', { mode: 'quick', model: 'hp-omnibook-x-14-fe', os: 'Ubuntu 25.10', kernel: '7.2.0-18-qcom-x1e', results: [{ check: 'wifi', state: 'works', how: 'measured' }] }, machine('machine-u-'.padEnd(40, 'u')));
  assert.equal(sent.status, 201);
  const page = await call('GET', '/api/models/hp-omnibook-x-14-fe');
  assert.equal(page.body.owners.checks.wifi.latest[0].os, 'Ubuntu 25.10');
  assert.equal(page.body.owners.checks.wifi.latest[0].kernel, '7.2.0-18-qcom-x1e');
});

test('finds an NVIDIA machine by the model number in its firmware strings, and names its family', async () => {
  // These start through ACPI, so no hardware ID list or compatible string names them
  const laptop = await call('GET', '/api/match?product=ProArt%20P16%20H7607BA&vendor=ASUSTeK%20COMPUTER%20INC.');
  assert.equal(laptop.body.model, 'asus-proart-p16-h7607ba');
  assert.equal(laptop.body.by, 'model number');
  assert.equal(laptop.body.family, 'arm-nvidia');
  assert.equal(laptop.body.boot, 'acpi');
  assert.equal((await call('GET', '/api/match?product=NVIDIA%20DGX%20Spark&vendor=NVIDIA')).body.model, 'nvidia-dgx-spark');
  // A run from one is kept under its model, like any other
  const sent = await call('POST', '/api/runs', { mode: 'quick', hardware: { maker: 'NVIDIA', product: 'NVIDIA DGX Spark' }, results: [{ check: 'wifi', state: 'works', how: 'measured' }] }, machine('machine-n-'.padEnd(40, 'n')));
  assert.equal(sent.status, 201);
  assert.equal(sent.body.model, 'nvidia-dgx-spark');
  assert.equal(sent.body.page, 'https://arewearmyet.com/models/nvidia-dgx-spark');
});

test('finds a machine by its device tree, then by its model number', async () => {
  assert.equal((await call('GET', '/api/match?compatible=lenovo,yoga-slim7x')).body.model, YOGA);
  const byNumber = await call('GET', '/api/match?product=83ED&vendor=LENOVO');
  assert.equal(byNumber.body.model, YOGA);
  assert.equal(byNumber.body.by, 'model number');
  assert.equal((await call('GET', '/api/match?product=HP%20OmniBook%20X%20Laptop%2014-fe0xxx&vendor=HP')).body.model, 'hp-omnibook-x-14-fe');
  assert.equal((await call('GET', '/api/match?product=Unheard%20Of')).body.model, null);
  // The family a pull request for the machine names. Chromebooks and Googlebooks are Google's, the rest came with Windows
  assert.equal(byNumber.body.family, 'arm-dragon-msft');
  // The kernel on each of Omarchy's channels comes with the match, to tell a machine that is behind
  assert.match(byNumber.body.kernels.edge, /^\d+\.\d+/);
  const chromebook = await call('GET', '/api/match?compatible=google,lazor-rev1');
  assert.equal(chromebook.body.model, 'acer-chromebook-spin-513-cp513-1h');
  assert.equal(chromebook.body.family, 'arm-dragon-google');
  assert.equal((await call('GET', '/api/match?compatible=google,mica')).body.family, 'arm-dragon-google');
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
    packages: { 'linux-omarchy': '7.3-59', 'hw-test': '0.15.10', secret: 'x' },
    results: [{ check: 'wifi', state: 'works', how: 'measured', evidence: 'wlan0 6c:6e:07:11:22:33 in /home/birk/x, serial: PF0000' }],
  }, machine(TOKEN_A));
  assert.equal(sent.status, 201);
  assert.equal(sent.body.model, YOGA);
  assert.match(sent.body.run, /^RUN-[A-Z0-9]{10}$/);

  const stored = (await call('GET', `/api/runs/${sent.body.run}`)).body;
  assert.deepEqual(stored.hardware, { maker: 'LENOVO', product: '83ED' });
  // The test's own version is kept, so a result can be read knowing which version made it
  assert.deepEqual(stored.packages, { 'linux-omarchy': '7.3-59', 'hw-test': '0.15.10' });
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

test("a later measurement that finds a thing working doesn't take back the owner's failure", async () => {
  const token = 'machine-f-'.padEnd(40, 'f');
  const model = 'hp-omnibook-x-14-fe';
  const send = async (kernel: string, results: unknown[]) => {
    await tick();
    await call('POST', '/api/runs', { ...run(results), model, kernel }, machine(token));
  };
  const hibernate = async () => (await call('GET', `/api/models/${model}`)).body.owners.checks.hibernate;

  // The owner saw it fail, and a quick check the next day only measures that it ran once
  await send('7.3.0-72-omarchy', [{ check: 'hibernate', state: 'partial', how: 'owner', note: 'The second one ended in a restart.' }]);
  await send('7.3.0-72-omarchy', [{ check: 'hibernate', state: 'works', how: 'measured', evidence: '1 since boot' }]);
  assert.deepEqual((await hibernate()).states, { partial: 1 });
  assert.equal((await hibernate()).latest[0].note, 'The second one ended in a restart.');

  // A measured failure does replace it, and so does the owner testing again
  await send('7.3.0-75-omarchy', [{ check: 'hibernate', state: 'broken', how: 'measured', evidence: 'failed' }]);
  assert.deepEqual((await hibernate()).states, { broken: 1 });
  await send('7.3.0-80-omarchy', [{ check: 'hibernate', state: 'works', how: 'owner' }]);
  const fixed = await hibernate();
  assert.deepEqual(fixed.states, { works: 1 });
  assert.equal(fixed.fixed.to, '7.3.0-80-omarchy');
});

test("a look at a part doesn't replace what a test of it found, working or failing", async () => {
  const token = 'machine-p-'.padEnd(40, 'p');
  const model = 'hp-omnibook-x-14-fe';
  const send = async (kernel: string, results: unknown[]) => {
    await tick();
    return call('POST', '/api/runs', { ...run(results), model, kernel }, machine(token));
  };
  const check = async (id: string) => (await call('GET', `/api/models/${model}`)).body.owners.checks[id];

  // The test saw the GPU draw, and a quick check after an update only sees its driver bound
  await send('7.3.0-72-omarchy', [{ check: 'gpu', state: 'works', how: 'measured', evidence: 'Vulkan draws on Adreno X1-85', depth: 'test' }]);
  const looked = await send('7.3.0-75-omarchy', [{ check: 'gpu', state: 'works', how: 'measured', evidence: 'adreno bound to the GPU', depth: 'look' }]);
  let gpu = await check('gpu');
  assert.deepEqual(gpu.states, { works: 1 });
  assert.equal(gpu.latest[0].evidence, 'Vulkan draws on Adreno X1-85');
  assert.equal(gpu.latest[0].depth, 'test');
  assert.equal(gpu.latest[0].kernel, '7.3.0-72-omarchy');
  // The run itself keeps what it was sent
  assert.equal((await call('GET', `/api/runs/${looked.body.run}`)).body.results[0].depth, 'look');

  // A test that ran again replaces it, and so does a look that finds the part gone
  await send('7.3.0-75-omarchy', [{ check: 'gpu', state: 'works', how: 'measured', evidence: 'Vulkan draws on Adreno X1-85, again', depth: 'test' }]);
  assert.equal((await check('gpu')).latest[0].kernel, '7.3.0-75-omarchy');
  await send('7.3.0-80-omarchy', [{ check: 'gpu', state: 'broken', how: 'measured', evidence: 'no GPU', depth: 'look' }]);
  gpu = await check('gpu');
  assert.deepEqual(gpu.states, { broken: 1 });
  assert.equal(gpu.regressed.to, '7.3.0-80-omarchy');

  // Nor does a look take back a failure the test found
  await send('7.3.0-72-omarchy', [{ check: 'webgpu', state: 'partial', how: 'measured', evidence: 'only with switches', depth: 'test' }]);
  await send('7.3.0-75-omarchy', [{ check: 'webgpu', state: 'works', how: 'measured', evidence: 'the GPU is there', depth: 'look' }]);
  assert.deepEqual((await check('webgpu')).states, { partial: 1 });

  // A result from before hw-test said which it was is replaced by the newest, as it always was
  await send('7.3.0-72-omarchy', [{ check: 'wifi', state: 'works', how: 'measured', evidence: 'connected at 5180 MHz' }]);
  await send('7.3.0-75-omarchy', [{ check: 'wifi', state: 'works', how: 'measured', evidence: 'wlan0 connected', depth: 'look' }]);
  const wifi = await check('wifi');
  assert.equal(wifi.latest[0].evidence, 'wlan0 connected');
  assert.equal(wifi.latest[0].depth, 'look');
  // And a depth that isn't one of the two is not kept
  const odd = await send('7.3.0-75-omarchy', [{ check: 'usb', state: 'works', how: 'measured', depth: 'deep' }]);
  assert.equal((await call('GET', `/api/runs/${odd.body.run}`)).body.results[0].depth, null);
});

test('takes a note with logs, answers to it and a solved mark, and cleans what names the owner', async () => {
  const run = await call('POST', '/api/runs', { mode: 'agent', model: YOGA, results: [{ check: 'suspend', state: 'broken', how: 'owner' }] }, { 'x-machine-token': TOKEN_A });
  assert.equal(run.status, 201);

  // What a note needs
  for (const [body, word] of [
    [{ kind: 'problem', title: 'Freezes after sleep with a dock', body: 'Short.' }, 'body'],
    [{ kind: 'gossip', title: 'Freezes after sleep with a dock', body: 'It freezes when woken with a dock plugged in.' }, 'kind'],
    [{ kind: 'problem', title: 'Freeze', body: 'It freezes when woken with a dock plugged in.' }, 'title'],
    [{ kind: 'problem', check: 'nonsense', title: 'Freezes after sleep with a dock', body: 'It freezes when woken with a dock plugged in.' }, 'check'],
    [{ kind: 'upstream', title: 'A patch series for the dock', body: 'Posted to the list, not merged yet.', links: ['https://example.com/patch'] }, 'link'],
    [{ kind: 'problem', title: 'Freezes after sleep with a dock', body: 'It freezes when woken with a dock plugged in.', logs: [{ name: 'Kernel Log', text: 'x' }] }, 'name'],
  ] as const) {
    const refused = await call('POST', '/api/notes', body, { 'x-machine-token': TOKEN_A });
    assert.equal(refused.status, 400, JSON.stringify(body));
    assert.match(refused.body.error, new RegExp(word));
  }
  assert.equal((await call('POST', '/api/notes', { kind: 'note', title: 'No token on this one', body: 'It freezes when woken with a dock plugged in.', model: YOGA })).status, 400);

  // A problem that fits no check, here a freeze after a sequence of events, with the kernel's log.
  // The model is the one this machine's run named.
  const log = ['[  812.4] PM: suspend entry (deep)', '[  815.1] ath12k_pci 0004:01:00.0: failed to resume', '[UFW BLOCK] IN=wlan0 SRC=192.168.1.20 DST=224.0.0.251 MAC=aa:bb:cc:dd:ee:ff', 'usb 1-1: SerialNumber: 0123456789AB', 'Chromium 153.0.8010.36 on /home/alice/.cache',
    'Kernel command line: cryptdevice=PARTUUID=3069aaaa-1111-2222-3333-444455556666:root root=/dev/mapper/root rw', 'BTRFS info (device dm-0): first mount of filesystem a504bbbb-1111-2222-3333-444455556666',
    'Kernel command line: root=PARTUUID=9f8e7d6c-02 rw', 'systemd-journald[775]: File /var/log/journal/6703a82ab8ad4e0a975e10b2a27fa6e1/system.journal corrupted or uncleanly shut down',
    'systemd[1]: Expecting device /dev/disk/by-uuid/1AEE-4C7F...', 'systemd[1]: Found device /dev/disk/by-id/nvme-SAMSUNG_MZVL21T0_S676NX0T123456-part2 then by-label/EFI', 'systemd[1]: dev-disk-by\\x2duuid-1AEE\\x2d4C7F.device: Job timed out',
    'r8152 2-1:1.0 enx00e04c680001: renamed from eth0', 'input: Margrethes Tastatur as /devices/virtual/misc/uhid/0005:004C:0267.0008/input/input25',
    "input: Alice's AirPods (AVRCP) as /devices/virtual/input/input23", 'apple 0005:05AC:0267.0008: input,hidraw5: BLUETOOTH HID v0.67 Keyboard [Kontorets tastatur] on xx',
    "usb 3-1: Product: Alice's iPhone", 'wireguard: wg0: Handshake for peer 3 (203.0.113.5:51820) did not complete', 'wireguard: wg0: Sending keepalive to peer 4 ([2001:db8::7]:51820)',
    'input: hid-over-i2c 048D:8987 Keyboard as /devices/platform/soc@0/bc0000.geniqup/b80000.i2c/i2c-1/1-003a/0018:048D:8987.0001/input/input2', '[mem 0x7a300000-0x7a3fffff]: assigned, firmware 172.4.0.2 kept'].join('\n');
  const posted = await call('POST', '/api/notes', {
    kind: 'problem', check: 'suspend', title: 'Freezes on wake when a USB-C dock was plugged in during sleep',
    body: 'Steps: close the lid, plug the dock in, open the lid.\nThe screen stays black and only a long press on the power button helps. Router at 192.168.1.1. Lenovo\'s dock, disk a504bbbb-1111-2222-3333-444455556666.',
    links: ['https://lore.kernel.org/linux-arm-msm/20261101-dock-resume-v1-0-abc@example.org/'], kernel: '7.3.0-72-omarchy', agent: 'codex',
    logs: [{ name: 'kernel.log', text: log }],
  }, { 'x-machine-token': TOKEN_A });
  assert.equal(posted.status, 201);
  assert.match(posted.body.note, /^NOTE-[A-Z0-9]{10}$/);
  assert.equal(posted.body.model, YOGA);
  const id = posted.body.note as string;

  // Another machine's agent reads it, with the log, and answers with what it found
  const read = await call('GET', `/api/notes/${id}`);
  assert.equal(read.body.kind, 'problem');
  assert.equal(read.body.status, 'open');
  assert.equal(read.body.check, 'suspend');
  assert.match(read.body.body, /Steps: close the lid, plug the dock in, open the lid\.\nThe screen stays black/);
  assert.doesNotMatch(read.body.body, /192\.168|a504bbbb/);
  assert.match(read.body.body, /Lenovo's dock, disk xxxxxxxx-/);
  assert.deepEqual(read.body.logs.map((l: { name: string }) => l.name), ['kernel.log']);
  const text = await (await reports(new Request(`https://x/api/notes/${id}/logs/kernel.log`), env as never, new URL(`https://x/api/notes/${id}/logs/kernel.log`)))!.text();
  assert.match(text, /ath12k_pci 0004:01:00\.0: failed to resume/);
  // Nothing that names the owner's network, devices or home is kept, and a version number is left alone
  for (const personal of ['192.168.1.20', '224.0.0.251', 'aa:bb:cc:dd:ee:ff', '0123456789AB', 'alice']) assert.ok(!text.includes(personal), personal);
  assert.match(text, /Chromium 153\.0\.8010\.36 on \/home\/user/);
  // Nor what names the disk, the install, a network adapter, a Bluetooth device or a VPN's other end
  for (const personal of ['3069aaaa', 'a504bbbb', '9f8e7d6c', '6703a82a', '00e04c680001', '1AEE', 'S676NX0T123456', 'Margrethes', 'Alice', 'Kontorets', '203.0.113.5', '2001:db8']) assert.ok(!text.includes(personal), personal);
  // The lines stay readable, and what tells one device or version from another is kept
  for (const kept of ['cryptdevice=PARTUUID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx:root root=/dev/mapper/root rw', 'root=PARTUUID=xxxxxxxx-xx rw', 'enxxxxxxxxxxxxx: renamed from eth0', 'Expecting device /dev/disk/by-uuid/[removed]', 'Found device /dev/disk/by-id/[removed] then by-label/EFI', 'dev-disk-by\\x2duuid-[removed]', 'input: [name removed] as /devices/virtual/misc/uhid/',
    'input: [name removed] (AVRCP) as', 'Keyboard [name removed] on xx', "Product: Someone's iPhone", 'peer 3 (x.x.x.x:51820)', 'peer 4 ([removed]:51820)', 'input: hid-over-i2c 048D:8987 Keyboard as /devices/platform', '[mem 0x7a300000-0x7a3fffff]: assigned, firmware 172.4.0.2 kept'])
    assert.ok(text.includes(kept), `${kept}\n${text}`);

  const reply = await call('POST', `/api/notes/${id}/replies`, { body: 'Same here on the HP dock. It wakes fine with the dock plugged in before sleep.', agent: 'claude', links: ['https://github.com/omacom/omarchy-pkgs/pull/3000'] }, { 'x-machine-token': TOKEN_B });
  assert.equal(reply.status, 201);
  assert.equal(reply.body.of, id);
  // An answer isn't answered itself, so the thread stays one level deep
  assert.equal((await call('POST', `/api/notes/${reply.body.note}/replies`, { body: 'An answer to an answer, which has no place.' }, { 'x-machine-token': TOKEN_A })).status, 400);

  // It shows on the model, in the list by check and kind, and with its answer when read
  const model = await call('GET', `/api/models/${YOGA}`);
  assert.equal(model.body.notes.length, 1);
  assert.equal(model.body.notes[0].replies, 1);
  assert.equal(model.body.notes[0].page, `https://arewearmyet.com/note?id=${id}`);
  assert.equal((await call('GET', '/api/notes?model=' + YOGA + '&check=suspend&kind=problem&status=open')).body.notes.length, 1);
  assert.equal((await call('GET', '/api/notes?check=camera')).body.notes.length, 0);
  // Without a model it searches every model's notes, by words in the title or the text
  assert.equal((await call('GET', '/api/notes?q=dock')).body.notes.length, 1);
  assert.equal((await call('GET', '/api/notes?q=DOCK%20lid')).body.notes.length, 1);
  assert.equal((await call('GET', '/api/notes?q=dock%20keyboard')).body.notes.length, 0);
  assert.equal((await call('GET', '/api/notes?q=100%25')).body.notes.length, 0);
  assert.equal((await call('GET', '/api/notes')).body.notes.length, 1);
  assert.equal((await call('GET', '/api/notes?kind=gossip')).status, 400);
  const thread = await call('GET', `/api/notes/${id}`);
  assert.equal(thread.body.replies.length, 1);
  assert.equal(thread.body.replies[0].kind, 'reply');
  assert.equal(thread.body.replies[0].agent, 'claude');

  // Only its writer marks it solved
  assert.equal((await call('POST', `/api/notes/${id}/status`, { status: 'solved' }, { 'x-machine-token': TOKEN_B })).status, 404);
  assert.equal((await call('POST', `/api/notes/${id}/status`, { status: 'done' }, { 'x-machine-token': TOKEN_A })).status, 400);
  assert.equal((await call('POST', `/api/notes/${id}/status`, { status: 'solved' }, { 'x-machine-token': TOKEN_A })).body.status, 'solved');
  assert.equal((await call('GET', '/api/notes?model=' + YOGA + '&status=open')).body.notes.length, 0);

  // The admin hides a note, and its log goes with it
  assert.ok((await call('GET', '/api/review', undefined, { authorization: `Bearer ${ADMIN}` })).body.notes.some((n: { id: string }) => n.id === id));
  assert.equal((await call('POST', '/api/review/hide', { kind: 'note', id }, { authorization: `Bearer ${ADMIN}` })).body.changed, 1);
  assert.equal((await call('GET', `/api/notes/${id}`)).status, 404);
  assert.equal((await reports(new Request(`https://x/api/notes/${id}/logs/kernel.log`), env as never, new URL(`https://x/api/notes/${id}/logs/kernel.log`)))!.status, 404);
  assert.equal((await call('GET', `/api/models/${YOGA}`)).body.notes.length, 0);
});

test('ties what a pull request changed to it, as each machine measured before and after', async () => {
  const pr = 'https://github.com/omacom/omarchy-pkgs/pull/3100';
  const changed = (to: string) => ({
    model: YOGA, url: pr, note: 'Right speaker reset GPIO',
    changes: [
      { check: 'speakers', from: { state: 'partial', how: 'measured', evidence: 'left heard at -31 dB, right not heard' }, to: { state: to, how: 'measured', evidence: 'left heard at -31 dB, right heard at -32 dB' } },
      { check: 'microphones', from: { state: 'works', how: 'measured' }, to: { state: 'works', how: 'measured' } },
    ],
    before: { kernel: '7.3.0-72-omarchy', packages: { 'linux-omarchy-dtbs': '7.3-9', 'linux-omarchy': '7.3-72', 'my-own-tool': '1.0' } },
    after: { kernel: '7.3.0-72-omarchy', packages: { 'linux-omarchy': '7.3-72' } },
  });
  // Measured changes count once per machine, so they need the machine's token
  assert.equal((await call('POST', '/api/fixes', changed('works'))).status, 400);
  assert.equal((await call('POST', '/api/fixes', { ...changed('works'), changes: [{ check: 'speakers', from: { state: 'partial' }, to: { state: 'splendid' } }] }, machine(TOKEN_A))).status, 400);
  assert.equal((await call('POST', '/api/fixes', { ...changed('works'), changes: [{ check: 'nonsense', from: { state: 'partial' }, to: { state: 'works' } }] }, machine(TOKEN_A))).status, 400);

  const first = await call('POST', '/api/fixes', changed('works'), machine(TOKEN_A));
  assert.equal(first.status, 201);
  assert.match(first.body.url, /api\/fixes\?url=https%3A%2F%2Fgithub\.com/);
  await call('POST', '/api/fixes', changed('works'), machine(TOKEN_B));

  // A reviewer asks what the pull request did, over every machine that measured it
  let seen = await call('GET', '/api/fixes?url=' + encodeURIComponent(pr));
  assert.equal(seen.body.machines, 2);
  assert.deepEqual(seen.body.improved, ['speakers']);
  assert.deepEqual(seen.body.worse, []);
  assert.deepEqual(seen.body.checks.speakers, { improved: 2, unchanged: 0, worse: 0 });
  assert.deepEqual(seen.body.checks.microphones, { improved: 0, unchanged: 2, worse: 0 });
  assert.equal(seen.body.fixes[0].changes[0].to.evidence, 'left heard at -31 dB, right heard at -32 dB');
  assert.equal(seen.body.fixes[0].versions.before.kernel, '7.3.0-72-omarchy');
  // Only what Omarchy's packages are called is kept of the versions
  assert.deepEqual(seen.body.fixes[0].versions.before.packages, { 'linux-omarchy': '7.3-72', 'linux-omarchy-dtbs': '7.3-9' });

  // A machine that measures again replaces what it said, and one where it got worse shows
  await call('POST', '/api/fixes', { ...changed('broken') }, machine(TOKEN_B));
  seen = await call('GET', '/api/fixes?url=' + encodeURIComponent(pr));
  assert.equal(seen.body.machines, 2);
  assert.deepEqual(seen.body.checks.speakers, { improved: 1, unchanged: 0, worse: 1 });
  assert.deepEqual(seen.body.improved, []);
  assert.deepEqual(seen.body.worse, ['speakers']);

  // It shows with the model's fixes, and a plain link without measurements still works
  const model = await call('GET', `/api/models/${YOGA}`);
  assert.ok(model.body.fixes.some((f: { url: string; changes: unknown[] }) => f.url === pr && f.changes.length === 2));
  assert.equal((await call('POST', '/api/fixes', { model: YOGA, url: 'https://github.com/omacom/omarchy/pull/3101' })).status, 201);
  assert.equal((await call('GET', '/api/fixes?url=' + encodeURIComponent('https://github.com/omacom/omarchy/pull/3101'))).body.machines, 0);
  assert.equal((await call('GET', '/api/fixes?url=https://example.com/x')).status, 400);
});

test('keeps what a computer ran that Omarchy did not install, by name, and shows it beside that computer\'s results', async () => {
  const token = machine('machine-s-'.padEnd(40, 's'));
  const sent = await call('POST', '/api/runs', run([{ check: 'battery', state: 'works', how: 'measured' }], {
    model: 'hp-omnibook-x-14-fe',
    setup: {
      kernel: 'linux-omarchy-mine',
      rebuilt: ['linux-omarchy-dtbs', 'not a name'],
      modules: ['omnibook_fix', 'omnibook_fix', '../../etc/passwd', 7],
      // The second is Omarchy's own and the third names a disk, which an older hw-test might still send
      options: ['regulator_ignore_unused', 'clk_ignore_unused', 'root=UUID=11111111-2222-3333-4444-555555555555', 'luks.key=…', 'a b'],
      packages: ['my-ucm'],
      // What hw-test reads out of the owner's settings files, and anything of another shape is dropped
      moduleOptions: ['snd_soc_x1e80100 i2s_mode=1', 'hid_apple fnmode=…', 'options hid_apple fnmode=2'],
      blocked: ['pcspkr'],
      loaded: ['scmi-cpufreq', 'a module'],
      rules: ['usb sets power/control=on', 'usb runs', 'input env LIBINPUT_IGNORE_DEVICE', 'any permissions', 'usb runs /home/owner/fix', 'usb sets idVendor=="1050"'],
      files: ['firmware/qcom/x1e80100/LENOVO/83ED/qcadsp8380.mbn', 'alsa/ucm2/Qualcomm/mine.conf', '/home/owner/file', 'etc/passwd'],
      altered: ['alsa-ucm-conf'],
      sound: ['api.alsa.headroom', 'node.name = "KEF"'],
      testTree: true,
      note: 'a rule for the speakers, MAC aa:bb:cc:dd:ee:ff',
      serial: 'PF123456',
    },
  }), token);
  assert.equal(sent.status, 201);
  const want = {
    kernel: 'linux-omarchy-mine',
    rebuilt: ['linux-omarchy-dtbs'],
    modules: ['omnibook_fix'],
    options: ['regulator_ignore_unused', 'luks.key=…'],
    packages: ['my-ucm'],
    moduleOptions: ['snd_soc_x1e80100 i2s_mode=1', 'hid_apple fnmode=…'],
    blocked: ['pcspkr'],
    loaded: ['scmi-cpufreq'],
    rules: ['usb sets power/control=on', 'usb runs', 'input env LIBINPUT_IGNORE_DEVICE', 'any permissions'],
    files: ['firmware/qcom/x1e80100/LENOVO/83ED/qcadsp8380.mbn', 'alsa/ucm2/Qualcomm/mine.conf'],
    altered: ['alsa-ucm-conf'],
    sound: ['api.alsa.headroom'],
    testTree: true,
    note: 'a rule for the speakers, MAC xx:xx:xx:xx:xx:xx',
  };
  assert.deepEqual((await call('GET', `/api/runs/${sent.body.run}`)).body.setup, want);
  const page = await call('GET', '/api/models/hp-omnibook-x-14-fe');
  assert.deepEqual(page.body.owners.checks.battery.latest.find((r: { run: string }) => r.run === sent.body.run).setup, want);

  // A computer with nothing of its own has no setup, and neither has one whose only option is Omarchy's
  await tick();
  const plain = await call('POST', '/api/runs', run([{ check: 'battery', state: 'works', how: 'measured' }], { model: 'hp-omnibook-x-14-fe', setup: { options: ['quiet'], modules: [] } }), machine('machine-p-'.padEnd(40, 'p')));
  assert.equal(plain.status, 201);
  assert.equal((await call('GET', `/api/runs/${plain.body.run}`)).body.setup, undefined);
});

test('keeps which change made on a computer a part works because of, under that computer\'s result, until it is taken back', async () => {
  const token = machine('machine-needs-'.padEnd(40, 'w'));
  const need = { check: 'suspend', text: 'The kernel option gpiolib_acpi.ignore_wake, which keeps the lid sensor from waking it. MAC aa:bb:cc:dd:ee:ff', how: 'agent', proved: true, kernel: '7.3.0-73-omarchy' };
  // A computer that has sent nothing can't say it, and neither can anyone without its token
  assert.equal((await call('POST', '/api/needs', need, token)).status, 404);
  assert.equal((await call('POST', '/api/needs', need)).status, 400);

  const sent = await call('POST', '/api/runs', run([{ check: 'suspend', state: 'works', how: 'measured' }, { check: 'battery', state: 'works', how: 'measured' }], { model: 'hp-omnibook-x-14-fe' }), token);
  assert.equal(sent.status, 201);
  assert.equal((await call('POST', '/api/needs', { ...need, check: 'no-such-check' }, token)).status, 400);
  assert.equal((await call('POST', '/api/needs', { ...need, how: 'measured' }, token)).status, 400);
  const said = await call('POST', '/api/needs', need, token);
  assert.equal(said.status, 201);
  assert.equal(said.body.text, 'The kernel option gpiolib_acpi.ignore_wake, which keeps the lid sensor from waking it. MAC xx:xx:xx:xx:xx:xx');

  const latest = async (check: string) => {
    const page = await call('GET', '/api/models/hp-omnibook-x-14-fe');
    return { check: page.body.owners.checks[check], result: page.body.owners.checks[check].latest.find((r: { run: string }) => r.run === sent.body.run) };
  };
  let suspend = await latest('suspend');
  assert.equal(suspend.result.needs.text, said.body.text);
  assert.equal(suspend.result.needs.how, 'agent');
  assert.equal(suspend.result.needs.proved, true);
  assert.equal(suspend.check.changed, 1);
  // Only the part it was said of
  assert.equal((await latest('battery')).result.needs, undefined);
  assert.equal((await latest('battery')).check.changed, undefined);

  // Said again, it replaces what was said, and an empty text takes it back
  assert.equal((await call('POST', '/api/needs', { check: 'suspend', text: 'The owner added a kernel option.', how: 'owner' }, token)).status, 201);
  suspend = await latest('suspend');
  assert.deepEqual([suspend.result.needs.text, suspend.result.needs.how, suspend.result.needs.proved], ['The owner added a kernel option.', 'owner', undefined]);
  const back = await call('POST', '/api/needs', { check: 'suspend', text: '' }, token);
  assert.deepEqual([back.status, back.body.removed], [200, true]);
  assert.equal((await latest('suspend')).result.needs, undefined);

  // A part that stopped working no longer works because of anything
  assert.equal((await call('POST', '/api/needs', need, token)).status, 201);
  await tick();
  assert.equal((await call('POST', '/api/runs', run([{ check: 'suspend', state: 'broken', how: 'owner', note: 'It no longer wakes.' }], { model: 'hp-omnibook-x-14-fe' }), token)).status, 201);
  const page = await call('GET', '/api/models/hp-omnibook-x-14-fe');
  assert.equal(page.body.owners.checks.suspend.latest.some((r: { needs?: unknown }) => r.needs), false);
});
