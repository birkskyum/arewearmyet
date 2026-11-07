// Reads the public sources that decide whether a Snapdragon PC can run Linux and
// writes what they say to data/sources.json. Run with `pnpm collect`.
//
// Sources: device trees in Linux releases and linux-next, hardware IDs in systemd
// releases and Ubuntu's stubble, firmware folders in linux-firmware, and the ACPI
// dumps collected by the aarch64-laptops project.

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const KERNEL_RELEASES = ['v6.8', 'v6.9', 'v6.10', 'v6.11', 'v6.12', 'v6.13', 'v6.14', 'v6.15', 'v6.16', 'v6.17', 'v6.18', 'v6.19', 'v7.0', 'v7.1', 'v7.2', 'v7.3'];
const SYSTEMD_RELEASES = ['v257', 'v258', 'v259', 'v260', 'v261', 'v262'];
const DT_PATTERN = /^(x1|hamoa|purwa|glymur|mahua|sc8280xp).*\.dts$/;
const DT_DIR = 'arch/arm64/boot/dts/qcom';
const FIRMWARE_SOCS = ['x1e80100', 'x1p42100', 'x126100', 'glymur', 'sc8280xp'];
// Omarchy's package channels: its own repository carries the kernel and the makers' firmware
// packages, its Arch Linux ARM mirror carries systemd, whose hardware ID list decides which device
// tree boots, and linux-firmware. Their files databases list what each package installs.
const OMARCHY_CHANNELS = [
  { name: 'stable', packages: 'https://pkgs.omarchy.org/stable/aarch64/omarchy.files', core: 'https://arm-stable-mirror.omarchy.org/aarch64/core/core.files' },
  { name: 'rc', packages: 'https://pkgs.omarchy.org/rc/aarch64/omarchy.files', core: 'https://arm-rc-mirror.omarchy.org/aarch64/core/core.files' },
  { name: 'edge', packages: 'https://pkgs.omarchy.org/edge/aarch64/omarchy.files', core: 'https://arm-mirror.omarchy.org/aarch64/core/core.files' },
];
const OMARCHY_PKGS = 'omacom/omarchy-pkgs';
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

// GitHub's API and raw hosts answer the odd request with a 5xx, so a server
// error or a dropped connection is tried again before the run gives up.
async function fetchRetrying(url: string, init: RequestInit = {}): Promise<Response> {
  for (let attempt = 1; ; attempt++) {
    try {
      const response = await fetch(url, init);
      if (response.status < 500 || attempt === 3) return response;
    } catch (error) {
      if (attempt === 3) throw error;
    }
    await new Promise((resolve) => setTimeout(resolve, 2000 * attempt));
  }
}

async function getJson(url: string, headers: Record<string, string> = {}): Promise<any> {
  // GitLab rate-limits unauthenticated API calls per address; a token lifts that
  if (process.env.GITLAB_TOKEN && url.startsWith('https://gitlab.com/')) headers = { ...headers, 'PRIVATE-TOKEN': process.env.GITLAB_TOKEN };
  const response = await fetchRetrying(url, { headers });
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
  const response = await fetchRetrying(`https://raw.githubusercontent.com/${repo}/${ref}/${path}`);
  if (!response.ok) throw new Error(`${response.status} ${repo}@${ref}:${path}`);
  return response.text();
}

async function listDir(repo: string, ref: string, path: string): Promise<string[] | undefined> {
  const entries = await github(`repos/${repo}/contents/${path}?ref=${ref}`);
  return entries?.map((entry: { name: string }) => entry.name);
}

// The root node's model and compatible strings of a device tree source: the first ones after "/ {", however
// they are indented, since some trees indent with spaces
function parseDts(source: string): { model?: string; compatible: string[] } {
  const root = source.slice(Math.max(0, source.search(/^\/ \{/m)));
  const model = root.match(/^[ \t]+model\s*=\s*"([^"]+)"/m)?.[1];
  const compatibleList = root.match(/^[ \t]+compatible\s*=\s*([^;]+);/m)?.[1] ?? '';
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

  // systemd copies its list in batches from Ubuntu's stubble, so an entry merged there or open as a pull
  // request there is on its way
  const stubble = [];
  for (const file of (await listDir('ubuntu/stubble', 'main', 'hwids/json')) ?? []) {
    const entry = JSON.parse(await raw('ubuntu/stubble', 'main', `hwids/json/${file}`));
    stubble.push({ file, state: 'merged', url: `https://github.com/ubuntu/stubble/blob/main/hwids/json/${file}`, name: entry.name, compatible: entry.compatible, hwids: entry.hwids });
  }
  for (const pull of (await github('repos/ubuntu/stubble/pulls?state=open&per_page=100')) ?? []) {
    for (const change of (await github(`repos/ubuntu/stubble/pulls/${pull.number}/files?per_page=100`)) ?? []) {
      if (!/^hwids\/json\/[^/]+\.json$/.test(change.filename) || change.status === 'removed') continue;
      const entry = JSON.parse(await raw(pull.head.repo?.full_name ?? 'ubuntu/stubble', pull.head.sha, change.filename));
      stubble.push({ file: change.filename.split('/').pop(), state: 'open', url: pull.html_url, name: entry.name, compatible: entry.compatible, hwids: entry.hwids });
    }
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
  const whence = await (await fetchRetrying(`https://gitlab.com/kernel-firmware/linux-firmware/-/raw/${ref}/WHENCE`)).text();
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

// A pacman repository's files database, unpacked into a folder per package (<name>-<version>)
// that holds its desc and files lists
async function repository(dbUrl: string): Promise<string | undefined> {
  const response = await fetchRetrying(dbUrl);
  if (!response.ok) return undefined;
  const id = Buffer.from(dbUrl).toString('base64url');
  const file = join(tmpdir(), `arewearmyet-${id}.files`);
  const dir = join(tmpdir(), `arewearmyet-${id}`);
  writeFileSync(file, Buffer.from(await response.arrayBuffer()));
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(dir, { recursive: true });
  execFileSync('tar', ['-xf', file, '-C', dir]);
  return dir;
}

type Package = { name: string; version: string; files: string[] };

function packages(dir: string | undefined): Package[] {
  if (!dir) return [];
  return readdirSync(dir).map((folder) => {
    const field = (text: string, key: string) => text.match(new RegExp(`^%${key}%\n([^\n]+)`, 'm'))?.[1] ?? '';
    const desc = readFileSync(join(dir, folder, 'desc'), 'utf8');
    const files = existsSync(join(dir, folder, 'files')) ? readFileSync(join(dir, folder, 'files'), 'utf8').split('\n').filter((line) => line && !line.startsWith('%') && !line.endsWith('/')) : [];
    return { name: field(desc, 'NAME'), version: field(desc, 'VERSION'), files };
  });
}

// What a channel ships for Snapdragon PCs: the device trees in its kernel, the hardware ID files
// in systemd and the ones the kernel package adds, and every Qualcomm laptop firmware file by the
// package it comes in, under the name the kernel asks for
function channelContents(repo: Package[], core: Package[]) {
  const kernel = repo.find((pkg) => pkg.name === 'linux-omarchy');
  const systemd = core.find((pkg) => pkg.name === 'systemd');
  const names = (files: string[] | undefined, pattern: RegExp) => (files ?? []).map((path) => path.match(pattern)?.[1]).filter((name): name is string => Boolean(name)).sort();
  const firmware: Record<string, string[]> = {};
  // Omarchy's repository comes first in pacman.conf, so a package it builds, like its
  // linux-firmware-qcom with backported firmware, replaces Arch Linux ARM's of the same name
  const own = new Set(repo.map((pkg) => pkg.name));
  for (const pkg of [...repo, ...core.filter((pkg) => pkg.name.startsWith('linux-firmware') && !own.has(pkg.name))]) {
    const files = names(pkg.files, new RegExp(`^usr/lib/firmware/(qcom/(?:${FIRMWARE_SOCS.join('|')})/.+?)(?:\\.zst|\\.xz)?$`));
    if (files.length) firmware[pkg.name] = files;
  }
  return {
    kernel: kernel?.version ?? null,
    systemd: systemd?.version ?? null,
    deviceTrees: names(kernel?.files, /^boot\/dtbs\/linux-omarchy\/qcom\/([^/]+)\.dtb$/).map((name) => `${name}.dts`).filter((file) => DT_PATTERN.test(file)),
    hwids: names(systemd?.files, /^usr\/lib\/systemd\/boot\/hwids\/aa64\/([^/]+\.json)$/),
    carriedIds: names(kernel?.files, /^usr\/lib\/systemd\/boot\/hwids\/aa64\/linux-omarchy\/([^/]+\.json)$/),
    firmware,
  };
}

// The device trees a kernel patch adds: a new file shows up as a section from /dev/null to b/<path>
function patchDeviceTrees(text: string) {
  const added = new Map<string, string>();
  for (const section of text.split(/^diff --git /m)) {
    const file = section.match(/^--- \/dev\/null\n\+\+\+ b\/arch\/arm64\/boot\/dts\/qcom\/(\S+)$/m)?.[1];
    if (!file) continue;
    added.set(file, section.split('\n').filter((line) => line.startsWith('+') && !line.startsWith('+++')).map((line) => line.slice(1)).join('\n'));
  }
  return [...added].filter(([file]) => DT_PATTERN.test(file)).map(([file, source]) => ({ file, ...patchRoot(source, added) }));
}

// A tree whose root node sits in a .dtsi the same patch adds, like the Vivobook 16's, takes its model and
// compatible from the first included .dtsi that has a model, since a SoC .dtsi has compatibles but no model
function patchRoot(source: string, added: Map<string, string>) {
  const own = parseDts(source);
  if (own.compatible.length) return own;
  for (const [, include] of source.matchAll(/^#include "([^"]+\.dtsi)"/gm)) {
    const root = added.has(include) ? parseDts(added.get(include)!) : undefined;
    if (root?.model && root.compatible.length) return root;
  }
  return own;
}

// Device trees the linux-omarchy recipe adds on top of the Linux release it builds, and the hardware
// ID files it installs next to systemd's
function omarchyKernelRecipe() {
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
    added.push(...patchDeviceTrees(readFileSync(join(recipe, patch), 'utf8')).map((dt) => ({ ...dt, patch })));
  }
  const ids = readdirSync(recipe).filter((name) => name.endsWith('.json')).sort().map((file) => {
    const entry = JSON.parse(readFileSync(join(recipe, file), 'utf8'));
    return { file, name: entry.name, compatible: entry.compatible, hwids: entry.hwids };
  });
  return { added, ids };
}

// Open pull requests on omarchy-pkgs that add what a model needs: a kernel patch with its device tree,
// a hardware ID file for the kernel package, or a maker's firmware package, by the files it installs
async function collectOmarchyPulls() {
  const pulls: any[] = [];
  for (let page = 1; ; page++) {
    const batch = (await github(`repos/${OMARCHY_PKGS}/pulls?state=open&per_page=100&page=${page}`)) ?? [];
    pulls.push(...batch);
    if (batch.length < 100) break;
  }
  const found = [];
  for (const pull of pulls) {
    const repo = pull.head.repo?.full_name;
    if (!repo) continue;
    const changes: { filename: string; status: string }[] = [];
    for (let page = 1; ; page++) {
      const batch = (await github(`repos/${OMARCHY_PKGS}/pulls/${pull.number}/files?per_page=100&page=${page}`)) ?? [];
      changes.push(...batch);
      if (batch.length < 100) break;
    }
    const deviceTrees = [];
    const ids = [];
    const firmware: string[] = [];
    for (const { filename, status } of changes) {
      if (status === 'removed') continue;
      if (filename.startsWith(`${OMARCHY_KERNEL_RECIPE}/`) && /(dts|arm64-qcom).*\.patch$/i.test(filename)) {
        deviceTrees.push(...patchDeviceTrees(await raw(repo, pull.head.sha, filename)));
      } else if (filename.startsWith(`${OMARCHY_KERNEL_RECIPE}/`) && filename.endsWith('.json')) {
        const entry = JSON.parse(await raw(repo, pull.head.sha, filename));
        ids.push({ file: filename.split('/').pop(), compatible: entry.compatible });
      } else if (/^pkgbuilds\/[^/]+-firmware\/PKGBUILD$/.test(filename)) {
        const recipe = await raw(repo, pull.head.sha, filename);
        firmware.push(...[...recipe.matchAll(/"\$fw"\/'(qcom\/[^']+)'/g)].map((m) => m[1]));
      }
    }
    if (deviceTrees.length || ids.length || firmware.length) {
      found.push({ number: pull.number, title: pull.title, url: pull.html_url, draft: pull.draft, deviceTrees, ids, firmware: [...new Set(firmware)].sort() });
    }
  }
  return found;
}

async function collectOmarchy() {
  const channels = [];
  for (const channel of OMARCHY_CHANNELS) {
    const [repo, core] = await Promise.all([repository(channel.packages), repository(channel.core)]);
    channels.push({ name: channel.name, ...channelContents(packages(repo), packages(core)) });
  }
  const recipe = omarchyKernelRecipe();
  // The edge nightly image, which testers install: it picks up every tree and ID the edge kernel has
  const latest = await getJson('https://nightly.omarchy.org/edge-aarch64/latest.json').catch(() => undefined);
  const image = latest?.url ? { version: latest.version, url: latest.url, kernel: latest.kernel ?? null } : null;
  return { channels, kernelAddedDeviceTrees: recipe.added, carriedIds: recipe.ids, image, pulls: await collectOmarchyPulls() };
}

// ACPI dumps from aarch64-laptops, by misc/ folder, from the ARM installer's hardware reports, by
// reports/<vendor>/<model> folder as hardware-reports/<vendor>/<model>, since one model can get several reports,
// and from this repository's acpi/<folder> as arewearmyet/<folder>, for dumps sent elsewhere but not merged yet
async function collectAcpiDumps() {
  const [laptops, reports] = await Promise.all([
    github('repos/aarch64-laptops/build/git/trees/HEAD?recursive=1'),
    github('repos/turbineBMW/omarchy-hardware-reports/git/trees/HEAD?recursive=1'),
  ]);
  const paths = (tree: { tree: { path: string }[] }) => tree.tree.map((entry) => entry.path);
  const misc = new Set(paths(laptops).filter((path) => /^misc\/[^/]+\/.*dsdt\.(dat|bin|aml|dsl)$/i.test(path)).map((path) => path.split('/')[1]));
  const reported = new Set(paths(reports).map((path) => path.match(/^reports\/([^/]+\/[^/]+)\/[^/]+\/acpi\/dsdt\.(dat|bin|aml)$/i)?.[1]).filter(Boolean) as string[]);
  return [
    ...[...misc].sort().map((folder) => ({ folder, url: `https://github.com/aarch64-laptops/build/tree/HEAD/misc/${folder}` })),
    ...[...reported].sort().map((model) => ({ folder: `hardware-reports/${model}`, url: `https://github.com/turbineBMW/omarchy-hardware-reports/tree/main/reports/${model}` })),
    ...(existsSync('acpi') ? readdirSync('acpi').filter((folder) => existsSync(join('acpi', folder, 'dsdt.dat'))).sort() : [])
      .map((folder) => ({ folder: `arewearmyet/${folder}`, url: `https://github.com/birkskyum/arewearmyet/tree/main/acpi/${folder}` })),
  ];
}

const [kernel, hardwareIds, firmware, acpiDumps, omarchy] = await Promise.all([collectDeviceTrees(), collectHardwareIds(), collectFirmware(), collectAcpiDumps(), collectOmarchy()]);

mkdirSync('data', { recursive: true });
writeFileSync(
  'data/sources.json',
  JSON.stringify({ collected: new Date().toISOString(), kernelReleases: KERNEL_RELEASES, systemdReleases: SYSTEMD_RELEASES, ...kernel, ...hardwareIds, firmwareRelease: firmware.release, firmware: firmware.folders, firmwareMain: firmware.firmwareMain, firmwareReleased: firmware.firmwareReleased, acpiDumps, omarchy }, null, 2) + '\n',
);
console.log(`Omarchy ${omarchy.channels.map((c) => `${c.name} ${c.kernel}/${c.systemd} (${c.deviceTrees.length} trees, ${c.hwids.length}+${c.carriedIds.length} IDs, ${Object.values(c.firmware).flat().length} firmware files)`).join(', ')}, recipe device trees ${omarchy.kernelAddedDeviceTrees.length}, open PRs ${omarchy.pulls.map((pull) => `#${pull.number}`).join(' ')}; device trees ${kernel.deviceTrees.length}, systemd IDs ${hardwareIds.systemd.length}, stubble ${hardwareIds.stubble.length}, firmware folders ${firmware.folders.length} (release ${firmware.release}), ACPI dumps ${acpiDumps.length}`);
