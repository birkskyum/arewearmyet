// Reads the public sources that decide whether a Snapdragon PC can run Linux and
// writes what they say to data/sources.json. Run with `pnpm collect`.
//
// Sources: device trees in Linux releases and linux-next, hardware IDs in systemd
// releases and Ubuntu's stubble, firmware folders in linux-firmware, and the ACPI
// dumps collected by the aarch64-laptops project.

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const KERNEL_RELEASES = ['v6.8', 'v6.9', 'v6.10', 'v6.11', 'v6.12', 'v6.13', 'v6.14', 'v6.15', 'v6.16', 'v6.17', 'v6.18', 'v6.19', 'v7.0', 'v7.1', 'v7.2', 'v7.3'];
const SYSTEMD_RELEASES = ['v257', 'v258', 'v259', 'v260', 'v261', 'v262'];
const DT_PATTERN = /^(x1|hamoa|purwa|glymur|sc8280xp).*\.dts$/;
const DT_DIR = 'arch/arm64/boot/dts/qcom';
const FIRMWARE_SOCS = ['x1e80100', 'x1p42100', 'glymur', 'sc8280xp'];

function githubToken(): string | undefined {
  if (process.env.GITHUB_TOKEN) return process.env.GITHUB_TOKEN;
  try {
    return execFileSync('gh', ['auth', 'token'], { encoding: 'utf8' }).trim();
  } catch {
    return undefined;
  }
}

const token = githubToken();

async function getJson(url: string, headers: Record<string, string> = {}): Promise<any> {
  const response = await fetch(url, { headers });
  if (response.status === 404) return undefined;
  if (!response.ok) throw new Error(`${response.status} ${url}`);
  return response.json();
}

function github(path: string): Promise<any> {
  const headers: Record<string, string> = { Accept: 'application/vnd.github+json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  return getJson(`https://api.github.com/${path}`, headers);
}

async function raw(repo: string, ref: string, path: string): Promise<string> {
  const response = await fetch(`https://raw.githubusercontent.com/${repo}/${ref}/${path}`);
  if (!response.ok) throw new Error(`${response.status} ${repo}@${ref}:${path}`);
  return response.text();
}

async function listDir(repo: string, ref: string, path: string): Promise<string[] | undefined> {
  const entries = await github(`repos/${repo}/contents/${path}?ref=${ref}`);
  return entries?.map((entry: { name: string }) => entry.name);
}

// The root node's model and compatible strings of a device tree source
function parseDts(source: string): { model?: string; compatible: string[] } {
  const model = source.match(/^\tmodel\s*=\s*"([^"]+)"/m)?.[1];
  const compatibleList = source.match(/^\tcompatible\s*=\s*([^;]+);/m)?.[1] ?? '';
  return { model, compatible: [...compatibleList.matchAll(/"([^"]+)"/g)].map((m) => m[1]) };
}

// linux-next has no GitHub mirror, so take its device tree folder from a sparse clone
function linuxNextCheckout(): string {
  const dir = join(tmpdir(), 'arewearmyet-linux-next');
  if (existsSync(join(dir, '.git'))) {
    execFileSync('git', ['-C', dir, 'fetch', '-q', '--depth', '1', 'origin', 'master'], { stdio: 'inherit' });
    execFileSync('git', ['-C', dir, 'reset', '-q', '--hard', 'FETCH_HEAD'], { stdio: 'inherit' });
  } else {
    execFileSync('git', ['clone', '-q', '--depth', '1', '--filter=blob:none', '--sparse', 'https://git.kernel.org/pub/scm/linux/kernel/git/next/linux-next.git', dir], { stdio: 'inherit' });
    execFileSync('git', ['-C', dir, 'sparse-checkout', 'set', DT_DIR], { stdio: 'inherit' });
  }
  return dir;
}

async function collectDeviceTrees() {
  const firstRelease = new Map<string, string>();
  for (const release of KERNEL_RELEASES) {
    const names = await listDir('torvalds/linux', release, DT_DIR);
    for (const name of names ?? []) {
      if (DT_PATTERN.test(name) && !firstRelease.has(name)) firstRelease.set(name, release);
    }
  }
  const next = linuxNextCheckout();
  const nextTag = execFileSync('git', ['-C', next, 'log', '-1', '--format=%s'], { encoding: 'utf8' }).match(/\d{8}/)?.[0];
  const nextNames = readdirSync(join(next, DT_DIR)).filter((name) => DT_PATTERN.test(name));
  const names = [...new Set([...firstRelease.keys(), ...nextNames])].sort();
  return {
    linuxNext: nextTag,
    deviceTrees: names.map((file) => {
      const inNext = nextNames.includes(file);
      const source = inNext ? readFileSync(join(next, DT_DIR, file), 'utf8') : '';
      return { file, firstRelease: firstRelease.get(file) ?? null, inNext, ...parseDts(source) };
    }),
  };
}

async function collectHardwareIds() {
  const firstRelease = new Map<string, string>();
  for (const release of SYSTEMD_RELEASES) {
    for (const name of (await listDir('systemd/systemd', release, 'src/boot/hwids/aa64')) ?? []) {
      if (!firstRelease.has(name)) firstRelease.set(name, release);
    }
  }
  const mainNames = (await listDir('systemd/systemd', 'main', 'src/boot/hwids/aa64')) ?? [];
  const systemd = [];
  for (const file of [...new Set([...firstRelease.keys(), ...mainNames])].sort()) {
    const ref = mainNames.includes(file) ? 'main' : firstRelease.get(file)!;
    const entry = JSON.parse(await raw('systemd/systemd', ref, `src/boot/hwids/aa64/${file}`));
    systemd.push({ file, firstRelease: firstRelease.get(file) ?? null, inMain: mainNames.includes(file), name: entry.name, compatible: entry.compatible, hwids: entry.hwids });
  }

  // stubble keeps the SMBIOS strings each entry was made from, next to the IDs
  const stubble = [];
  for (const file of (await listDir('ubuntu/stubble', 'main', 'hwids/txt')) ?? []) {
    const text = await raw('ubuntu/stubble', 'main', `hwids/txt/${file}`);
    const field = (key: string) => text.match(new RegExp(`^${key}: (.*)$`, 'm'))?.[1]?.trim();
    stubble.push({ file, manufacturer: field('Manufacturer'), family: field('Family'), productName: field('ProductName'), productSku: field('ProductSku'), baseboardProduct: field('BaseboardProduct') });
  }
  return { systemd, stubble };
}

async function collectFirmware() {
  const folders: { path: string; files: string[] }[] = [];
  for (const soc of FIRMWARE_SOCS) {
    const entries: { type: string; path: string }[] = [];
    for (let page = 1; ; page++) {
      const batch = await getJson(`https://gitlab.com/api/v4/projects/kernel-firmware%2Flinux-firmware/repository/tree?path=qcom/${soc}&recursive=true&per_page=100&page=${page}`);
      if (!batch?.length) break;
      entries.push(...batch);
    }
    const blobs = entries.filter((entry) => entry.type === 'blob').map((entry) => entry.path);
    // A model's folder is one that holds files, below qcom/<soc>/<vendor>/
    for (const dir of new Set(blobs.map((path) => path.slice(0, path.lastIndexOf('/'))))) {
      if (dir.split('/').length < 4) continue;
      folders.push({ path: dir, files: blobs.filter((path) => path.startsWith(`${dir}/`)).map((path) => path.slice(dir.length + 1)) });
    }
  }
  return folders.sort((a, b) => a.path.localeCompare(b.path));
}

async function collectAcpiDumps() {
  const tree = await github('repos/aarch64-laptops/build/git/trees/HEAD?recursive=1');
  const paths: string[] = tree.tree.map((entry: { path: string }) => entry.path);
  const folders = new Set(paths.filter((path) => /^misc\/[^/]+\/.*dsdt\.(dat|bin|aml)$/i.test(path)).map((path) => path.split('/')[1]));
  return [...folders].sort().map((folder) => ({ folder, url: `https://github.com/aarch64-laptops/build/tree/HEAD/misc/${folder}` }));
}

const [kernel, hardwareIds, firmware, acpiDumps] = await Promise.all([collectDeviceTrees(), collectHardwareIds(), collectFirmware(), collectAcpiDumps()]);

mkdirSync('data', { recursive: true });
writeFileSync(
  'data/sources.json',
  JSON.stringify({ collected: new Date().toISOString(), kernelReleases: KERNEL_RELEASES, systemdReleases: SYSTEMD_RELEASES, ...kernel, ...hardwareIds, firmware, acpiDumps }, null, 2) + '\n',
);
console.log(`device trees ${kernel.deviceTrees.length}, systemd IDs ${hardwareIds.systemd.length}, stubble ${hardwareIds.stubble.length}, firmware folders ${firmware.length}, ACPI dumps ${acpiDumps.length}`);
