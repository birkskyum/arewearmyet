// Runs public/probe.sh against copies of real machines' /sys, /proc and /etc (test/probe/fixtures,
// made with scripts/probe-fixture.sh) and checks what it reports. The commands the probe runs are
// stood in for by test/probe/bin, which answer from the same fixture.
//
// The probe needs bash 4 or later, so on a Mac's bash 3.2 these tests are skipped; CI runs them.

import { execFileSync } from 'node:child_process';
import { cpSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const PROBE = resolve('public/probe.sh');
const STUBS = resolve('test/probe/bin');
const FIXTURES = resolve('test/probe/fixtures');
const bashMajor = Number(execFileSync('bash', ['-c', 'echo ${BASH_VERSINFO[0]}'], { encoding: 'utf8' }).trim());
const skip = bashMajor < 4 ? `bash ${bashMajor} is too old for the probe` : false;

type Result = { check: string; state: string; how: string; evidence?: string };
type Run = { mode: string; chids: string[]; compatible: string; kernel: string; hardware: Record<string, string>; packages: Record<string, string>; results: Result[] };

/** A private copy of a fixture, changed by `change`, and what the probe says about it. */
function probe(fixture: string, change: (root: string) => void = () => {}): { run: Run; state: (check: string) => string | undefined; evidence: (check: string) => string } {
  const root = mkdtempSync(join(tmpdir(), 'probe-test-'));
  try {
    cpSync(join(FIXTURES, fixture), root, { recursive: true, verbatimSymlinks: true });
    change(root);
    const out = execFileSync('bash', [PROBE, '--json'], {
      encoding: 'utf8',
      env: { PATH: `${STUBS}:${process.env.PATH}`, PROBE_ROOT: root, HOME: root, TMPDIR: tmpdir() },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    const run = JSON.parse(out) as Run;
    const find = (check: string) => run.results.find((r) => r.check === check);
    return { run, state: (check) => find(check)?.state, evidence: (check) => find(check)?.evidence ?? '' };
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}
const write = (root: string, path: string, text: string) => {
  mkdirSync(join(root, path, '..'), { recursive: true });
  writeFileSync(join(root, path), text);
};

const YOGA = 'yoga-slim-7x-gpu-off';

test('names the machine and its software, without anything personal', { skip }, () => {
  const { run } = probe(YOGA);
  assert.equal(run.mode, 'quick');
  assert.equal(run.compatible, 'lenovo,yoga-slim7x');
  assert.equal(run.kernel, '7.3.0-59-omarchy');
  assert.deepEqual(run.hardware, { maker: 'LENOVO', product: '83ED', family: 'Yoga Slim 7 14Q8X9', sku: 'LENOVO_MT_83ED_BU_idea_FM_Yoga Slim 7 14Q8X9', board: 'LNVNB161216', bios: 'NHCN62WW' });
  assert.equal(run.packages['linux-omarchy'], '7.3-59');
  assert.equal(run.packages.omarchy, '4.0.0.r7344.ga001044-1');
  assert.doesNotMatch(JSON.stringify(run), /([0-9a-f]{2}:){5}[0-9a-f]{2}/i);
  // Its hardware IDs, most specific first, with the type systemd gives each
  assert.equal(run.chids.length, 18);
  assert.equal(run.chids[0], '0:5315f3e1-1fa9-5592-bf05-75e8124d1f8a');
  assert.equal(run.chids[17], 'ext2:89a6ea33-85de-52aa-ad21-64eba70c0823');
});

test('measures a Yoga Slim 7x booted with its GPU off', { skip }, () => {
  const { state, evidence, run } = probe(YOGA);
  assert.equal(state('display'), 'works');
  assert.match(evidence('display'), /eDP-1 connected, 2944x1840/);
  assert.equal(state('gpu'), 'broken');
  assert.equal(state('wifi'), 'works');
  assert.equal(state('wifi-address'), 'works');
  assert.equal(state('bluetooth'), 'works');
  assert.equal(state('adsp'), 'works');
  assert.match(evidence('adsp'), /qcadsp8380\.mbn/);
  assert.equal(state('cdsp'), 'works');
  assert.equal(state('battery'), 'works');
  assert.equal(state('charging'), 'works');
  assert.match(evidence('charging'), /PD/);
  assert.equal(state('hibernate'), 'broken');
  // Every result of a quick run is a measurement
  assert.ok(run.results.every((r) => r.how === 'measured'));
  // What needs a person is left out, even though the hardware is there
  for (const check of ['keyboard', 'touchpad', 'speakers', 'microphones', 'camera', 'backlight', 'lid']) assert.equal(state(check), undefined, check);
});

test('sees the GPU once the adreno driver is bound', { skip }, () => {
  const { state, evidence } = probe(YOGA, (root) => {
    mkdirSync(join(root, 'sys/bus/platform/drivers/adreno/3d00000.gpu'), { recursive: true });
    write(root, 'cmd/klog', '');
  });
  assert.equal(state('gpu'), 'works');
  assert.match(evidence('gpu'), /3d00000\.gpu/);
});

test('calls the GPU partly working when software rendering is forced', { skip }, () => {
  const { state } = probe(YOGA, (root) => {
    mkdirSync(join(root, 'sys/bus/platform/drivers/adreno/3d00000.gpu'), { recursive: true });
    write(root, 'cmd/klog', '');
    write(root, 'etc/environment', 'LIBGL_ALWAYS_SOFTWARE=1\n');
  });
  assert.equal(state('gpu'), 'partial');
});

test('reports missing and failed hardware as broken', { skip }, () => {
  const { state, evidence } = probe(YOGA, (root) => {
    rmSync(join(root, 'sys/class/net/wlan0'), { recursive: true });
    rmSync(join(root, 'sys/class/bluetooth'), { recursive: true });
    rmSync(join(root, 'proc/asound/cards'));
    for (const n of ['0', '1', '2', '3']) {
      try { write(root, `sys/class/remoteproc/remoteproc${n}/state`, 'crashed\n'); } catch { /* fewer processors */ }
    }
  });
  assert.equal(state('wifi'), 'broken');
  assert.equal(state('wifi-address'), undefined);
  assert.equal(state('bluetooth'), 'broken');
  assert.equal(state('speakers'), 'broken');
  assert.equal(state('microphones'), 'broken');
  assert.equal(state('adsp'), 'broken');
  assert.match(evidence('adsp'), /crashed/);
});

test('tells a random Wi-Fi address from a fixed one', { skip }, () => {
  const { state } = probe(YOGA, (root) => write(root, 'sys/class/net/wlan0/addr_assign_type', '1\n'));
  assert.equal(state('wifi-address'), 'broken');
});

test('calls charging at 5 V partly working', { skip }, () => {
  const { state, evidence } = probe(YOGA, (root) => {
    const supply = 'sys/class/power_supply/ucsi-source-psy-pmic_glink.ucsi.02';
    write(root, `${supply}/usb_type`, '[C] PD PD_PPS\n');
    write(root, `${supply}/voltage_now`, '5000000\n');
  });
  assert.equal(state('charging'), 'partial');
  assert.match(evidence('charging'), /5 V only/);
});

test('counts sleeps since boot', { skip }, () => {
  const slept = probe(YOGA, (root) => write(root, 'sys/power/suspend_stats/success', '4\n'));
  assert.equal(slept.state('suspend'), 'works');
  const failed = probe(YOGA, (root) => {
    write(root, 'sys/power/suspend_stats/success', '4\n');
    write(root, 'sys/power/suspend_stats/fail', '2\n');
  });
  assert.equal(failed.state('suspend'), 'partial');
});

test("doesn't expect a battery, lid or camera on a desktop", { skip }, () => {
  const { state } = probe(YOGA, (root) => {
    write(root, 'sys/class/dmi/id/chassis_type', '3\n');
    rmSync(join(root, 'sys/class/power_supply'), { recursive: true });
    rmSync(join(root, 'sys/class/video4linux'), { recursive: true });
    rmSync(join(root, 'sys/class/backlight'), { recursive: true, force: true });
  });
  assert.equal(state('battery'), undefined);
  assert.equal(state('camera'), undefined);
  assert.equal(state('backlight'), undefined);
  // The video decoder went with video4linux, and every machine should have one
  assert.equal(state('video-decode'), 'broken');
});

test('finds the camera by its sensor, and misses it when only the camera subsystem is there', { skip }, () => {
  assert.equal(probe(YOGA).state('camera'), undefined);
  const { state, evidence } = probe(YOGA, (root) => {
    for (const n of Array.from({ length: 40 }, (_, i) => i)) rmSync(join(root, `sys/class/video4linux/v4l-subdev${n}`), { recursive: true, force: true });
  });
  assert.equal(state('camera'), 'broken');
  assert.match(evidence('camera'), /no sensor/);
});

test('sends with a token it keeps, and offers the add-model form when the board doesn\'t know the machine', { skip }, async () => {
  const { spawnSync } = await import('node:child_process');
  const { readFileSync } = await import('node:fs');
  const root = mkdtempSync(join(tmpdir(), 'probe-test-'));
  try {
    cpSync(join(FIXTURES, YOGA), root, { recursive: true, verbatimSymlinks: true });
    const env = { PATH: `${STUBS}:${process.env.PATH}`, PROBE_ROOT: root, HOME: root, TMPDIR: tmpdir() };
    // No terminal, as when an agent runs it: it sends without asking
    const sent = spawnSync('bash', [PROBE, '--agent', 'test agent'], { encoding: 'utf8', env, stdio: ['ignore', 'pipe', 'pipe'] });
    assert.equal(sent.status, 0, sent.stderr);

    const token = readFileSync(join(root, '.local/state/arewearmyet/token'), 'utf8');
    assert.match(token, /^[A-Za-z0-9_-]{40,}$/);
    const args = readFileSync(join(root, 'curl-args'), 'utf8');
    assert.ok(args.includes(`x-machine-token: ${token}`));
    assert.ok(args.includes('https://arewearmyet.com/api/runs'));
    const body = JSON.parse(readFileSync(join(root, 'curl-body'), 'utf8'));
    assert.equal(body.agent, 'test agent');
    assert.equal(body.results.length, 10);

    assert.match(sent.stderr, /Sent as RUN-TESTTESTAA/);
    const form = sent.stderr.match(/https:\/\/github\.com\/birkskyum\/arewearmyet\/issues\/new\S+/)![0];
    const params = new URL(form).searchParams;
    assert.equal(params.get('template'), 'model.yml');
    assert.equal(params.get('model'), 'LENOVO Yoga Slim 7 14Q8X9');
    assert.equal(params.get('model-number'), '83ED');
    assert.equal(params.get('probe'), 'RUN-TESTTESTAA');
    assert.match(params.get('hwids')!, /^0 5315f3e1-1fa9-5592-bf05-75e8124d1f8a\n1 26ebc60f/);

    // The next run uses the same token, and --no-send sends nothing
    rmSync(join(root, 'curl-args'));
    spawnSync('bash', [PROBE, '--no-send'], { encoding: 'utf8', env, stdio: ['ignore', 'pipe', 'pipe'] });
    assert.throws(() => readFileSync(join(root, 'curl-args')));
    spawnSync('bash', [PROBE], { encoding: 'utf8', env, stdio: ['ignore', 'pipe', 'pipe'] });
    assert.ok(readFileSync(join(root, 'curl-args'), 'utf8').includes(`x-machine-token: ${token}`));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
