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
// Omarchy's package channels: its own repository carries the kernel, its Arch Linux
// ARM mirror carries systemd, whose hardware ID list decides which device tree boots
const OMARCHY_CHANNELS = [
  { name: 'stable', packages: 'https://pkgs.omarchy.org/stable/aarch64/omarchy.db', core: 'https://arm-stable-mirror.omarchy.org/aarch64/core/core.db' },
  { name: 'rc', packages: 'https://pkgs.omarchy.org/rc/aarch64/omarchy.db', core: 'https://arm-rc-mirror.omarchy.org/aarch64/core/core.db' },
  { name: 'edge', packages: 'https://pkgs.omarchy.org/edge/aarch64/omarchy.db', core: 'https://arm-mirror.omarchy.org/aarch64/core/core.db' },
];
const OMARCHY_KERNEL_RECIPE = 'pkgbuilds/linux-omarchy-aarch64';

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

// The vendor-specific firmware a device tree loads, from its firmware-name lines and those of
// the .dtsi files it includes from the same folder
function dtFirmware(dir: string, file: string, seen = new Set<string>()): string[] {
  if (seen.has(file) || !existsSync(join(dir, file))) return [];
  seen.add(file);
  const source = readFileSync(join(dir, file), 'utf8');
  const names = [...source.matchAll(/firmware-name\s*=\s*([^;]+);/g)].flatMap((m) => [...m[1].matchAll(/"([^"]+)"/g)].map((n) => n[1]));
  const included = [...source.matchAll(/^#include "([^"]+\.dtsi)"/gm)].flatMap((m) => dtFirmware(dir, m[1], seen));
  return [...new Set([...names, ...included])].filter((name) => name.startsWith('qcom/') && name.split('/').length >= 4);
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
      const firmware = inNext ? dtFirmware(join(next, DT_DIR), file) : [];
      return { file, firstRelease: firstRelease.get(file) ?? null, inNext, ...parseDts(source), firmware };
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

const LINUX_FIRMWARE = 'https://gitlab.com/api/v4/projects/kernel-firmware%2Flinux-firmware';

// Every Qualcomm laptop file at one linux-firmware ref, counting the symlinks WHENCE creates
async function firmwareFiles(ref: string): Promise<string[]> {
  const files: string[] = [];
  for (const soc of FIRMWARE_SOCS) {
    for (let page = 1; ; page++) {
      const batch: { type: string; path: string }[] = await getJson(`${LINUX_FIRMWARE}/repository/tree?path=qcom/${soc}&recursive=true&per_page=100&page=${page}&ref=${ref}`);
      if (!batch?.length) break;
      files.push(...batch.filter((entry) => entry.type === 'blob').map((entry) => entry.path));
    }
  }
  const whence = await (await fetch(`https://gitlab.com/kernel-firmware/linux-firmware/-/raw/${ref}/WHENCE`)).text();
  const links = [...whence.matchAll(/^Link: (qcom\/\S+) ->/gm)].map((m) => m[1]).filter((path) => FIRMWARE_SOCS.some((soc) => path.startsWith(`qcom/${soc}/`)));
  return [...new Set([...files, ...links])].sort();
}

async function collectFirmware() {
  // Distributions ship linux-firmware releases, which are dated tags
  const tags: { name: string }[] = await getJson(`${LINUX_FIRMWARE}/repository/tags?order_by=name&sort=desc&per_page=20`);
  const release = tags.map((tag) => tag.name).find((name) => /^\d{8}$/.test(name))!;
  const [main, released] = await Promise.all([firmwareFiles('main'), firmwareFiles(release)]);
  // A model's folder is one that holds files, below qcom/<soc>/<vendor>/
  const folders = [...new Set(main.map((path) => path.slice(0, path.lastIndexOf('/'))))]
    .filter((dir) => dir.split('/').length >= 4)
    .map((dir) => ({ path: dir, files: main.filter((path) => path.startsWith(`${dir}/`)).map((path) => path.slice(dir.length + 1)) }));
  return { release, firmwareMain: main, firmwareReleased: released, folders: folders.sort((a, b) => a.path.localeCompare(b.path)) };
}

// The version of a package in a pacman repository database, or undefined if it isn't there
async function packageVersion(dbUrl: string, name: string): Promise<string | undefined> {
  const response = await fetch(dbUrl);
  if (!response.ok) return undefined;
  const file = join(tmpdir(), `arewearmyet-${Buffer.from(dbUrl).toString('base64url')}.db`);
  writeFileSync(file, Buffer.from(await response.arrayBuffer()));
  const entries = execFileSync('tar', ['-tf', file], { encoding: 'utf8', maxBuffer: 64 << 20 }).split('\n');
  const match = entries.map((entry) => entry.match(new RegExp(`^${name}-(\\d[^/]*)/$`))).find(Boolean);
  return match?.[1];
}

// Device trees the linux-omarchy recipe adds on top of the Linux release it builds
function omarchyKernelDeviceTrees() {
  const dir = join(tmpdir(), 'arewearmyet-omarchy-pkgs');
  if (existsSync(join(dir, '.git'))) {
    execFileSync('git', ['-C', dir, 'fetch', '-q', '--depth', '1', 'origin', 'master'], { stdio: 'inherit' });
    execFileSync('git', ['-C', dir, 'reset', '-q', '--hard', 'FETCH_HEAD'], { stdio: 'inherit' });
  } else {
    execFileSync('git', ['clone', '-q', '--depth', '1', '--filter=blob:none', '--sparse', 'https://github.com/omacom/omarchy-pkgs.git', dir], { stdio: 'inherit' });
    execFileSync('git', ['-C', dir, 'sparse-checkout', 'set', OMARCHY_KERNEL_RECIPE], { stdio: 'inherit' });
  }
  const recipe = join(dir, OMARCHY_KERNEL_RECIPE);
  const added = [];
  for (const patch of readdirSync(recipe).filter((name) => name.endsWith('.patch')).sort()) {
    // A new file shows up in a patch as a section from /dev/null to b/<path>
    for (const section of readFileSync(join(recipe, patch), 'utf8').split(/^diff --git /m)) {
      const file = section.match(/^--- \/dev\/null\n\+\+\+ b\/arch\/arm64\/boot\/dts\/qcom\/(\S+)$/m)?.[1];
      if (!file || !DT_PATTERN.test(file)) continue;
      const source = section.split('\n').filter((line) => line.startsWith('+') && !line.startsWith('+++')).map((line) => line.slice(1)).join('\n');
      added.push({ file, patch, ...parseDts(source) });
    }
  }
  return added;
}

async function collectOmarchy() {
  const channels = [];
  for (const channel of OMARCHY_CHANNELS) {
    const kernel = await packageVersion(channel.packages, 'linux-omarchy');
    const systemd = await packageVersion(channel.core, 'systemd');
    channels.push({ name: channel.name, kernel: kernel ?? null, systemd: systemd ?? null });
  }
  return { channels, kernelAddedDeviceTrees: omarchyKernelDeviceTrees() };
}

async function collectAcpiDumps() {
  const tree = await github('repos/aarch64-laptops/build/git/trees/HEAD?recursive=1');
  const paths: string[] = tree.tree.map((entry: { path: string }) => entry.path);
  const folders = new Set(paths.filter((path) => /^misc\/[^/]+\/.*dsdt\.(dat|bin|aml)$/i.test(path)).map((path) => path.split('/')[1]));
  return [...folders].sort().map((folder) => ({ folder, url: `https://github.com/aarch64-laptops/build/tree/HEAD/misc/${folder}` }));
}

const [kernel, hardwareIds, firmware, acpiDumps, omarchy] = await Promise.all([collectDeviceTrees(), collectHardwareIds(), collectFirmware(), collectAcpiDumps(), collectOmarchy()]);

mkdirSync('data', { recursive: true });
writeFileSync(
  'data/sources.json',
  JSON.stringify({ collected: new Date().toISOString(), kernelReleases: KERNEL_RELEASES, systemdReleases: SYSTEMD_RELEASES, ...kernel, ...hardwareIds, firmwareRelease: firmware.release, firmware: firmware.folders, firmwareMain: firmware.firmwareMain, firmwareReleased: firmware.firmwareReleased, acpiDumps, omarchy }, null, 2) + '\n',
);
console.log(`Omarchy ${omarchy.channels.map((c) => `${c.name} ${c.kernel}/${c.systemd}`).join(', ')}, recipe device trees ${omarchy.kernelAddedDeviceTrees.length}; device trees ${kernel.deviceTrees.length}, systemd IDs ${hardwareIds.systemd.length}, stubble ${hardwareIds.stubble.length}, firmware folders ${firmware.folders.length} (release ${firmware.release}), ACPI dumps ${acpiDumps.length}`);
