// Reads the public sources that decide whether a Snapdragon PC can run Linux and
// writes what they say to data/sources.json. Run with `pnpm collect`.
//
// The sources are device trees in Linux releases and linux-next, hardware IDs in systemd
// releases and Ubuntu's stubble, firmware folders in linux-firmware, and the ACPI
// dumps collected by the aarch64-laptops project, linux-surface and the ARM installer's reports.
//
// `pnpm collect omarchy acpi` refreshes only those parts (kernel, ids, firmware, acpi, omarchy) and keeps the
// rest of data/sources.json. What a fixed ref holds is kept in .cache/collect, so a run fetches only what
// moves. `--fresh` fetches everything again.

import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';

// From v5.10, for the Snapdragon 7c Chromebooks' trees, which Linux has had since then
const KERNEL_RELEASES = ['v5.10', 'v5.11', 'v5.12', 'v5.13', 'v5.14', 'v5.15', 'v5.16', 'v5.17', 'v5.18', 'v5.19', 'v6.0', 'v6.1', 'v6.2', 'v6.3', 'v6.4', 'v6.5', 'v6.6', 'v6.7', 'v6.8', 'v6.9', 'v6.10', 'v6.11', 'v6.12', 'v6.13', 'v6.14', 'v6.15', 'v6.16', 'v6.17', 'v6.18', 'v6.19', 'v7.0', 'v7.1', 'v7.2', 'v7.3'];
const SYSTEMD_RELEASES = ['v257', 'v258', 'v259', 'v260', 'v261', 'v262'];
// Snapdragon X, X2 and 8cx Gen 3, the 7c Chromebooks, and the older Windows laptops' trees
const DT_PATTERN = /^(x1|hamoa|purwa|glymur|mahua|kalambo|sc8280xp|sc7180-trogdor|sc7180-(acer|ecs|samsung|lenovo|tcl|apcsilmic)|sc8180x-(lenovo|microsoft|samsung)|sc8180xp-(dell|acer|lenovo|samsung|xiaomi)|sc7280-(samsung|hp)|sdm850|msm8998-(asus|hp|lenovo)).*\.dts$/;
const DT_DIR = 'arch/arm64/boot/dts/qcom';
// samsung is the Galaxy Book S's folder (qcom/samsung/w767), which its tree has always named
const FIRMWARE_SOCS = ['x1e80100', 'x1p42100', 'x126100', 'glymur', 'kalambo', 'sc8280xp', 'sc8180x', 'sc8180xp', 'sc7180', 'sc7180-trogdor', 'sc7280', 'sdm850', 'msm8998', 'samsung'];
// Omarchy's package channels. Its own repository carries the kernel and the makers' firmware
// packages, its Arch Linux ARM mirror carries systemd, whose hardware ID list decides which device
// tree boots, and linux-firmware. Their files databases list what each package installs.
const OMARCHY_CHANNELS = [
  { name: 'stable', packages: 'https://pkgs.omarchy.org/stable/aarch64/omarchy.files', core: 'https://arm-stable-mirror.omarchy.org/aarch64/core/core.files' },
  { name: 'rc', packages: 'https://pkgs.omarchy.org/rc/aarch64/omarchy.files', core: 'https://arm-rc-mirror.omarchy.org/aarch64/core/core.files' },
  { name: 'edge', packages: 'https://pkgs.omarchy.org/edge/aarch64/omarchy.files', core: 'https://arm-mirror.omarchy.org/aarch64/core/core.files' },
];
const OMARCHY_PKGS = 'omacom/omarchy-pkgs';
const OMARCHY_KERNEL_RECIPE = 'pkgbuilds/linux-omarchy-aarch64';
// The device trees and hardware ID files move out of the kernel package into this one
const OMARCHY_TREES_RECIPE = 'pkgbuilds/linux-omarchy-dtbs';
const OMARCHY_TREES_PACKAGE = 'linux-omarchy-dtbs';

function githubToken(): string | undefined {
  if (process.env.GITHUB_TOKEN) return process.env.GITHUB_TOKEN;
  try {
    return execFileSync('gh', ['auth', 'token'], { encoding: 'utf8' }).trim();
  } catch {
    return undefined;
  }
}

const token = githubToken();

const PARTS = ['kernel', 'ids', 'firmware', 'acpi', 'omarchy'] as const;
const args = process.argv.slice(2);
const fresh = args.includes('--fresh');
const unknown = args.filter((arg) => arg !== '--fresh' && !(PARTS as readonly string[]).includes(arg));
if (unknown.length) throw new Error(`unknown part ${unknown.join(', ')}; the parts are ${PARTS.join(', ')}`);
const parts = new Set(args.filter((arg) => (PARTS as readonly string[]).includes(arg)).length ? args : PARTS);

// A release tag, a linux-firmware release, a commit or a blob SHA never changes what it points to
const CACHE_DIR = '.cache/collect';
const FIXED_REF = /^([0-9a-f]{40}|v\d+(\.\d+)?|\d{8})$/;

async function cached<T>(key: string, fetcher: () => Promise<T>): Promise<T> {
  const file = join(CACHE_DIR, `${createHash('sha256').update(key).digest('hex').slice(0, 32)}.json`);
  if (!fresh && existsSync(file)) return JSON.parse(readFileSync(file, 'utf8'));
  const value = await fetcher();
  if (value !== undefined) {
    mkdirSync(CACHE_DIR, { recursive: true });
    writeFileSync(file, JSON.stringify(value));
  }
  return value;
}

// GitHub's API and raw hosts answer the odd request with a 5xx, so a server
// error or a dropped connection is tried again before the run gives up. GitHub
// and GitLab also answer a burst of requests with 429. That waits as long as
// Retry-After says, or a minute, and is tried more often.
async function fetchRetrying(url: string, init: RequestInit = {}): Promise<Response> {
  for (let attempt = 1; ; attempt++) {
    let wait = 2000 * attempt;
    try {
      const response = await fetch(url, init);
      if (response.status === 429 && attempt < 6) {
        wait = Math.max(Number(response.headers.get('retry-after')) || 60, 1) * 1000;
      } else if (response.status < 500 || attempt >= 3) {
        return response;
      }
    } catch (error) {
      if (attempt >= 3) throw error;
    }
    await new Promise((resolve) => setTimeout(resolve, wait));
  }
}

async function getJson(url: string, headers: Record<string, string> = {}): Promise<any> {
  // GitLab rate-limits unauthenticated API calls per address, and a token lifts that
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
  if (FIXED_REF.test(ref)) return cached(`raw ${repo} ${ref} ${path}`, () => rawFetch(repo, ref, path));
  return rawFetch(repo, ref, path);
}

async function rawFetch(repo: string, ref: string, path: string): Promise<string> {
  const response = await fetchRetrying(`https://raw.githubusercontent.com/${repo}/${ref}/${path}`);
  if (!response.ok) throw new Error(`${response.status} ${repo}@${ref}:${path}`);
  return response.text();
}

async function listDir(repo: string, ref: string, path: string): Promise<string[] | undefined> {
  return (await listEntries(repo, ref, path))?.map((entry) => entry.name);
}

// A folder's files with their blob SHAs, so a file at a moving ref like main is fetched only when it changed
async function listEntries(repo: string, ref: string, path: string): Promise<{ name: string; sha: string }[] | undefined> {
  const list = () => github(`repos/${repo}/contents/${path}?ref=${ref}`).then((entries) => entries?.map(({ name, sha }: { name: string; sha: string }) => ({ name, sha })));
  return FIXED_REF.test(ref) ? cached(`ls ${repo} ${ref} ${path}`, list) : list();
}

function blob(repo: string, ref: string, path: string, sha: string): Promise<string> {
  return cached(`blob ${sha}`, () => rawFetch(repo, ref, path));
}

// A pull request's changed files, which its head commit fixes
async function pullFiles(repo: string, pull: { number: number; head: { sha: string } }): Promise<{ filename: string; status: string }[]> {
  return cached(`pull files ${repo} ${pull.number} ${pull.head.sha}`, () => pages(`repos/${repo}/pulls/${pull.number}/files`));
}

// The root node's model and compatible strings of a device tree source, which are the first ones after "/ {", however
// they are indented, since some trees indent with spaces
function parseDts(source: string): { model?: string; compatible: string[] } {
  const root = source.slice(Math.max(0, source.search(/^\/ \{/m)));
  const model = root.match(/^[ \t]+model\s*=\s*"([^"]+)"/m)?.[1];
  const compatibleList = root.match(/^[ \t]+compatible\s*=\s*([^;]+);/m)?.[1] ?? '';
  return { model, compatible: [...compatibleList.matchAll(/"([^"]+)"/g)].map((m) => m[1]) };
}

// The vendor-specific firmware a device tree loads, from its firmware-name lines and those of
// the files it includes from the same folder, a .dtsi or another board's .dts
// A tree an Omarchy patch adds can include one from Linux, so the folders are searched in order.
function dtFirmware(dir: string | string[], file: string, seen = new Set<string>()): string[] {
  const dirs = Array.isArray(dir) ? dir : [dir];
  const found = dirs.find((d) => existsSync(join(d, file)));
  if (seen.has(file) || !found) return [];
  seen.add(file);
  const source = readFileSync(join(found, file), 'utf8');
  const names = [...source.matchAll(/firmware-name\s*=\s*([^;]+);/g)].flatMap((m) => [...m[1].matchAll(/"([^"]+)"/g)].map((n) => n[1]));
  const included = [...source.matchAll(/^#include "([^"]+\.dtsi?)"/gm)].flatMap((m) => dtFirmware(dirs, m[1], seen));
  return [...new Set([...names, ...included])].filter((name) => name.startsWith('qcom/') && name.split('/').length >= 4);
}

// linux-next has no GitHub mirror, so take its device tree folder from a sparse clone. kernel.org sometimes
// stalls a fetch for half an hour, so a fetch that takes over two minutes keeps the copy from the last run
// kernel.org is sometimes unreachable for hours, so Google's mirror of the same tree is tried after it
const LINUX_NEXT = [
  'https://git.kernel.org/pub/scm/linux/kernel/git/next/linux-next.git',
  'https://kernel.googlesource.com/pub/scm/linux/kernel/git/next/linux-next',
];

function linuxNextCheckout(): string {
  const dir = join(tmpdir(), 'arewearmyet-linux-next');
  if (existsSync(join(dir, '.git'))) {
    for (const url of LINUX_NEXT) {
      try {
        execFileSync('git', ['-C', dir, 'fetch', '-q', '--depth', '1', url, 'master'], { stdio: 'inherit', timeout: 120_000 });
        execFileSync('git', ['-C', dir, 'reset', '-q', '--hard', 'FETCH_HEAD'], { stdio: 'inherit' });
        return dir;
      } catch {
        console.warn(`linux-next didn't fetch from ${new URL(url).hostname}`);
      }
    }
    console.warn(`keeping ${execFileSync('git', ['-C', dir, 'log', '-1', '--format=%s'], { encoding: 'utf8' }).trim()}`);
    return dir;
  }
  for (const url of LINUX_NEXT) {
    try {
      execFileSync('git', ['clone', '-q', '--depth', '1', '--filter=blob:none', '--sparse', url, dir], { stdio: 'inherit', timeout: 300_000 });
      execFileSync('git', ['-C', dir, 'sparse-checkout', 'set', DT_DIR], { stdio: 'inherit' });
      return dir;
    } catch {
      console.warn(`linux-next didn't clone from ${new URL(url).hostname}`);
      rmSync(dir, { recursive: true, force: true });
    }
  }
  throw new Error('linux-next is unreachable on kernel.org and its mirror');
}

async function collectDeviceTrees(next: string) {
  const firstRelease = new Map<string, string>();
  for (const release of KERNEL_RELEASES) {
    const names = await listDir('torvalds/linux', release, DT_DIR);
    for (const name of names ?? []) {
      if (DT_PATTERN.test(name) && !firstRelease.has(name)) firstRelease.set(name, release);
    }
  }
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
  const mainEntries = (await listEntries('systemd/systemd', 'main', 'src/boot/hwids/aa64')) ?? [];
  const mainNames = mainEntries.map((entry) => entry.name);
  const systemd = [];
  for (const file of [...new Set([...firstRelease.keys(), ...mainNames])].sort()) {
    const path = `src/boot/hwids/aa64/${file}`;
    const onMain = mainEntries.find((entry) => entry.name === file);
    const entry = JSON.parse(onMain ? await blob('systemd/systemd', 'main', path, onMain.sha) : await raw('systemd/systemd', firstRelease.get(file)!, path));
    systemd.push({ file, firstRelease: firstRelease.get(file) ?? null, inMain: mainNames.includes(file), name: entry.name, compatible: entry.compatible, hwids: entry.hwids });
  }

  // systemd copies its list in batches from Ubuntu's stubble, so an entry merged there or open as a pull
  // request there is on its way
  const stubble = [];
  for (const { name: file, sha } of (await listEntries('ubuntu/stubble', 'main', 'hwids/json')) ?? []) {
    const entry = JSON.parse(await blob('ubuntu/stubble', 'main', `hwids/json/${file}`, sha));
    stubble.push({ file, state: 'merged', url: `https://github.com/ubuntu/stubble/blob/main/hwids/json/${file}`, name: entry.name, compatible: entry.compatible, hwids: entry.hwids });
  }
  for (const pull of (await github('repos/ubuntu/stubble/pulls?state=open&per_page=100')) ?? []) {
    for (const change of await pullFiles('ubuntu/stubble', pull)) {
      if (!/^hwids\/json\/[^/]+\.json$/.test(change.filename) || change.status === 'removed') continue;
      const entry = JSON.parse(await raw(pull.head.repo?.full_name ?? 'ubuntu/stubble', pull.head.sha, change.filename));
      stubble.push({ file: change.filename.split('/').pop(), state: 'open', url: pull.html_url, name: entry.name, compatible: entry.compatible, hwids: entry.hwids });
    }
  }
  return { systemd, stubble };
}

const LINUX_FIRMWARE = 'https://gitlab.com/api/v4/projects/kernel-firmware%2Flinux-firmware';

// Every Qualcomm laptop file at one linux-firmware ref, counting the symlinks WHENCE creates
function firmwareFiles(ref: string): Promise<string[]> {
  return cached(`linux-firmware ${ref} ${FIRMWARE_SOCS.join(' ')}`, () => firmwareFilesFetch(ref));
}

async function firmwareFilesFetch(ref: string): Promise<string[]> {
  const files: string[] = [];
  for (const soc of FIRMWARE_SOCS) {
    for (let page = 1; ; page++) {
      const batch: { type: string; path: string }[] = await getJson(`${LINUX_FIRMWARE}/repository/tree?path=qcom/${soc}&recursive=true&per_page=100&page=${page}&ref=${ref}`);
      if (!batch?.length) break;
      files.push(...batch.filter((entry) => entry.type === 'blob').map((entry) => entry.path));
    }
  }
  // Through the API, which takes the token, since GitLab rate-limits its raw file URLs per address
  const headers: Record<string, string> = process.env.GITLAB_TOKEN ? { 'PRIVATE-TOKEN': process.env.GITLAB_TOKEN } : {};
  const whenceResponse = await fetchRetrying(`${LINUX_FIRMWARE}/repository/files/WHENCE/raw?ref=${ref}`, { headers });
  if (!whenceResponse.ok) throw new Error(`${whenceResponse.status} WHENCE at ${ref}`);
  const whence = await whenceResponse.text();
  const links = [...whence.matchAll(/^Link: (qcom\/\S+) ->/gm)].map((m) => m[1]).filter((path) => FIRMWARE_SOCS.some((soc) => path.startsWith(`qcom/${soc}/`)));
  return [...new Set([...files, ...links])].sort();
}

async function collectFirmware() {
  // Distributions ship linux-firmware releases, which are dated tags
  const tags: { name: string }[] = await getJson(`${LINUX_FIRMWARE}/repository/tags?order_by=name&sort=desc&per_page=20`);
  const release = tags.map((tag) => tag.name).find((name) => /^\d{8}$/.test(name))!;
  // main by its commit, so an unchanged main is read from the cache
  const head: string = (await getJson(`${LINUX_FIRMWARE}/repository/commits/main`)).id;
  const [main, released] = await Promise.all([firmwareFiles(head), firmwareFiles(release)]);
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

type Package = { name: string; version: string; replaces: string[]; files: string[] };

function packages(dir: string | undefined): Package[] {
  if (!dir) return [];
  return readdirSync(dir).map((folder) => {
    const field = (text: string, key: string) => text.match(new RegExp(`^%${key}%\n([^\n]+)`, 'm'))?.[1] ?? '';
    const desc = readFileSync(join(dir, folder, 'desc'), 'utf8');
    const files = existsSync(join(dir, folder, 'files')) ? readFileSync(join(dir, folder, 'files'), 'utf8').split('\n').filter((line) => line && !line.startsWith('%') && !line.endsWith('/')) : [];
    const list = (key: string) => desc.match(new RegExp(`^%${key}%\n((?:[^\n]+\n?)+)`, 'm'))?.[1].split('\n').filter(Boolean) ?? [];
    return { name: field(desc, 'NAME'), version: field(desc, 'VERSION'), replaces: list('REPLACES'), files };
  });
}

// What a channel ships for Snapdragon PCs, which is the device trees in its kernel, the hardware ID files
// in systemd and the ones the kernel package adds, and every Qualcomm laptop firmware file by the
// package it comes in, under the name the kernel asks for
function channelContents(repo: Package[], core: Package[]) {
  const kernel = repo.find((pkg) => pkg.name === 'linux-omarchy');
  const trees = [...(kernel?.files ?? []), ...(repo.find((pkg) => pkg.name === OMARCHY_TREES_PACKAGE)?.files ?? [])];
  const systemd = core.find((pkg) => pkg.name === 'systemd');
  const names = (files: string[] | undefined, pattern: RegExp) => (files ?? []).map((path) => path.match(pattern)?.[1]).filter((name): name is string => Boolean(name)).sort();
  const firmware: Record<string, string[]> = {};
  // Omarchy's repository comes first in pacman.conf, so a package it builds, like its
  // linux-firmware-qcom with backported firmware, replaces Arch Linux ARM's of the same name
  const own = new Set(repo.map((pkg) => pkg.name));
  // A renamed package stays published under its old name until it is unpublished, and pacman -Syu
  // swaps it for the package that replaces it, so only the new name counts
  const replaced = new Set(repo.flatMap((pkg) => pkg.replaces));
  for (const pkg of [...repo.filter((pkg) => !replaced.has(pkg.name)), ...core.filter((pkg) => pkg.name.startsWith('linux-firmware') && !own.has(pkg.name))]) {
    const files = names(pkg.files, new RegExp(`^usr/lib/firmware/(qcom/(?:${FIRMWARE_SOCS.join('|')})/.+?)(?:\\.zst|\\.xz)?$`));
    if (files.length) firmware[pkg.name] = files;
  }
  return {
    kernel: kernel?.version ?? null,
    systemd: systemd?.version ?? null,
    deviceTrees: names(trees, /^boot\/dtbs\/linux-omarchy\/qcom\/([^/]+)\.dtb$/).map((name) => `${name}.dts`).filter((file) => DT_PATTERN.test(file)),
    hwids: names(systemd?.files, /^usr\/lib\/systemd\/boot\/hwids\/aa64\/([^/]+\.json)$/),
    carriedIds: names(trees, /^usr\/lib\/systemd\/boot\/hwids\/aa64\/linux-omarchy\/([^/]+\.json)$/),
    firmware,
  };
}

// The device trees a kernel patch adds. A new file shows up as a section from /dev/null to b/<path>
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

// Device trees the linux-omarchy and linux-omarchy-dtbs recipes add on top of the Linux release they
// build from, and the hardware ID files they install next to systemd's. New trees go in the dtbs
// recipe since linux-omarchy 7.3-47
function omarchyKernelRecipe() {
  const dir = join(tmpdir(), 'arewearmyet-omarchy-pkgs');
  if (existsSync(join(dir, '.git'))) {
    execFileSync('git', ['-C', dir, 'fetch', '-q', '--depth', '1', 'origin', 'master'], { stdio: 'inherit' });
    execFileSync('git', ['-C', dir, 'reset', '-q', '--hard', 'FETCH_HEAD'], { stdio: 'inherit' });
  } else {
    execFileSync('git', ['clone', '-q', '--depth', '1', '--filter=blob:none', '--sparse', 'https://github.com/omacom/omarchy-pkgs.git', dir], { stdio: 'inherit' });
  }
  execFileSync('git', ['-C', dir, 'sparse-checkout', 'set', OMARCHY_KERNEL_RECIPE, OMARCHY_TREES_RECIPE], { stdio: 'inherit' });
  const added = [];
  // The tree files the patches add or change, replayed in the order the recipes apply them, so a tree's
  // firmware takes in a later patch's change to it and a .dtsi that another patch adds. A Linux tree a
  // patch changes starts from its linux-next copy
  const replay = mkdtempSync(join(tmpdir(), 'arewearmyet-replay-'));
  const linuxTrees = join(tmpdir(), 'arewearmyet-linux-next', DT_DIR);
  for (const folder of [OMARCHY_KERNEL_RECIPE, OMARCHY_TREES_RECIPE].filter((folder) => existsSync(join(dir, folder)))) {
    for (const patch of readdirSync(join(dir, folder)).filter((name) => name.endsWith('.patch')).sort()) {
      const text = readFileSync(join(dir, folder, patch), 'utf8');
      added.push(...patchDeviceTrees(text).map((dt) => ({ ...dt, patch: `${folder.split('/').pop()}/${patch}` })));
      for (const section of text.split(/^diff --git /m).slice(1)) {
        const path = section.match(/^\+\+\+ b\/(arch\/arm64\/boot\/dts\/qcom\/\S+\.dtsi?)$/m)?.[1];
        if (!path) continue;
        const linuxCopy = join(linuxTrees, path.split('/').pop()!);
        if (!/^--- \/dev\/null$/m.test(section) && !existsSync(join(replay, path))) {
          if (!existsSync(linuxCopy)) continue;
          mkdirSync(dirname(join(replay, path)), { recursive: true });
          copyFileSync(linuxCopy, join(replay, path));
        }
        try {
          execFileSync('patch', ['-p1', '-s', '-f', '-d', replay], { input: 'diff --git ' + section, stdio: ['pipe', 'ignore', 'pipe'] });
        } catch {
          console.warn(`${patch}: its change to ${path} doesn't apply, keeping the version before it`);
        }
      }
    }
  }
  const replayTrees = join(replay, 'arch/arm64/boot/dts/qcom');
  for (const dt of added) dt.firmware = dtFirmware([replayTrees, linuxTrees], dt.file);
  // Linux trees whose firmware the patches change, like the HP Envy x2's, which Linux leaves without any
  const changedFirmware: Record<string, string[]> = {};
  for (const file of readdirSync(linuxTrees).filter((name) => DT_PATTERN.test(name))) {
    const ours = dtFirmware([replayTrees, linuxTrees], file);
    if (ours.join('\n') !== dtFirmware(linuxTrees, file).join('\n')) changedFirmware[file] = ours;
  }
  rmSync(replay, { recursive: true, force: true });
  const ids = [OMARCHY_KERNEL_RECIPE, OMARCHY_TREES_RECIPE].filter((folder) => existsSync(join(dir, folder))).flatMap((folder) =>
    readdirSync(join(dir, folder)).filter((name) => name.endsWith('.json')).map((file) => {
      const entry = JSON.parse(readFileSync(join(dir, folder, file), 'utf8'));
      return { file, recipe: folder, name: entry.name, compatible: entry.compatible, hwids: entry.hwids };
    }),
  ).sort((a, b) => a.file.localeCompare(b.file));
  return { added, changedFirmware, ids };
}

// Open pull requests on omarchy-pkgs that add what a model needs, be it a kernel or device tree package patch
// with its device tree, a hardware ID file for either package, or a maker's firmware package, by the files
// it installs
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
    const changes = await pullFiles(OMARCHY_PKGS, pull);
    const deviceTrees = [];
    const ids = [];
    const firmware: string[] = [];
    for (const { filename, status } of changes) {
      if (status === 'removed') continue;
      if ([OMARCHY_KERNEL_RECIPE, OMARCHY_TREES_RECIPE].some((folder) => filename.startsWith(`${folder}/`)) && /(dts|arm64-qcom).*\.patch$/i.test(filename)) {
        deviceTrees.push(...patchDeviceTrees(await raw(repo, pull.head.sha, filename)));
      } else if ([OMARCHY_KERNEL_RECIPE, OMARCHY_TREES_RECIPE].some((folder) => filename.startsWith(`${folder}/`)) && /^[^/]+\/[^/]+\/[^/]+\.json$/.test(filename)) {
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
  // The edge nightly image, which testers install. It picks up every tree and ID the edge kernel has
  const latest = await getJson('https://nightly.omarchy.org/edge-aarch64/latest.json').catch(() => undefined);
  const image = latest?.url ? { version: latest.version, url: latest.url, kernel: latest.kernel ?? null } : null;
  return { channels, kernelAddedDeviceTrees: recipe.added, kernelChangedFirmware: recipe.changedFirmware, carriedIds: recipe.ids, image, pulls: await collectOmarchyPulls() };
}

// ACPI dumps, each with where scripts/archive.ts fetches its tables (repo, ref, dir):
// - aarch64-laptops' misc/<folder>, merged, or proposed in a pull request that wasn't merged, open or closed, for
//   a folder master doesn't have
// - linux-surface's dumps of the Arm Surfaces, as linux-surface/<folder>
// - the ARM installer's hardware reports, merged or in an open pull request, by reports/<vendor>/<model> folder as
//   hardware-reports/<vendor>/<model>, since one model can get several reports. A report in a folder several
//   models share (PER_REPORT) goes by its own folder
// - this repository's acpi/<folder> as arewearmyet/<folder>, for dumps published here first
// - this repository's acpi/firmware/<folder> as firmware/<folder>, for tables taken out of a maker's BIOS or UEFI image
const LAPTOPS = 'aarch64-laptops/build';
const SURFACE = 'linux-surface/acpidumps';
const REPORTS = 'turbineBMW/omarchy-hardware-reports';
const SURFACE_ARM = /_qcom$|_5g$|^windows_dev_kit|^surface_pro_x/;
// Report folders that hold more than one model, so each report in them goes by its own folder. They are the ones the installer
// couldn't name, and the Galaxy Book4 Edge, whose 14, 15 and 16 inch models all give the same product name
const PER_REPORT = new Set(['unknown/unknown', 'samsung-electronics-co-ltd/galaxy-book4-edge']);

type AcpiDump = { folder: string; url: string; repo?: string; ref?: string; dir?: string; pull?: { number: number; state: string; by: string } };

async function pages(path: string): Promise<any[]> {
  const all: any[] = [];
  for (let page = 1; ; page++) {
    const batch = (await github(`${path}${path.includes('?') ? '&' : '?'}per_page=100&page=${page}`)) ?? [];
    all.push(...batch);
    if (batch.length < 100) return all;
  }
}

async function collectAcpiDumps(): Promise<AcpiDump[]> {
  const head = (repo: string) => github(`repos/${repo}/commits/HEAD`).then((commit) => commit.sha as string);
  const blobs = (repo: string, ref: string): Promise<string[]> => cached(`tree ${repo} ${ref}`, () => github(`repos/${repo}/git/trees/${ref}?recursive=1`).then((tree) => tree.tree.filter((entry: { type: string }) => entry.type === 'blob').map((entry: { path: string }) => entry.path)));
  const [laptopsRef, reportsRef, surfaceRef] = await Promise.all([head(LAPTOPS), head(REPORTS), head(SURFACE)]);
  const [laptops, reports, surface, laptopPulls, reportPulls] = await Promise.all([
    blobs(LAPTOPS, laptopsRef), blobs(REPORTS, reportsRef), blobs(SURFACE, surfaceRef), pages(`repos/${LAPTOPS}/pulls?state=all`), pages(`repos/${REPORTS}/pulls?state=open`),
  ]);

  const miscDump = (path: string) => path.match(/^misc\/([^/]+)\/.*dsdt\.(dat|bin|aml|dsl)$/i)?.[1];
  const misc = new Set(laptops.map(miscDump).filter(Boolean) as string[]);
  const proposed = new Map<string, AcpiDump>();
  // Newest first, so a folder proposed twice comes from the newer pull request
  for (const pull of laptopPulls.filter((pull) => !pull.merged_at).sort((a, b) => b.number - a.number)) {
    const files = await pullFiles(LAPTOPS, pull);
    for (const folder of new Set(files.filter((file) => file.status !== 'removed').map((file) => miscDump(file.filename)).filter(Boolean) as string[])) {
      if (misc.has(folder) || proposed.has(folder)) continue;
      proposed.set(folder, { folder, url: pull.html_url, repo: LAPTOPS, ref: pull.head.sha, dir: `misc/${folder}`, pull: { number: pull.number, state: pull.state, by: pull.user.login } });
    }
  }

  const surfaceFolders = new Set(surface.map((path) => path.split('/')).filter((parts) => parts.length > 1 && SURFACE_ARM.test(parts[0])).map((parts) => parts[0]));

  const reportDump = (path: string) => path.match(/^reports\/([^/]+\/[^/]+)\/([^/]+)\/acpi\/dsdt\.(dat|bin|aml)$/i);
  const reportKey = (model: string, id: string) => (PER_REPORT.has(model) ? `${model}/${id}` : model);
  const reported = new Map<string, AcpiDump>();
  for (const [, model, id] of reports.map(reportDump).filter(Boolean) as RegExpMatchArray[]) {
    const key = reportKey(model, id);
    reported.set(key, { folder: `hardware-reports/${key}`, url: `https://github.com/${REPORTS}/tree/main/reports/${key}` });
  }
  // Merged reports come first, so a model with both links the merged ones
  const pending: AcpiDump[] = [];
  for (const pull of reportPulls) {
    for (const [, model, id] of (await blobs(REPORTS, pull.head.sha)).map(reportDump).filter(Boolean) as RegExpMatchArray[]) {
      if (reports.some((path) => path.startsWith(`reports/${model}/${id}/`))) continue;
      const key = reportKey(model, id);
      pending.push({ folder: `hardware-reports/${key}`, url: pull.html_url, repo: REPORTS, ref: pull.head.sha, dir: `reports/${model}/${id}`, pull: { number: pull.number, state: pull.state, by: pull.user.login } });
    }
  }

  return [
    ...[...misc].sort().map((folder) => ({ folder, url: `https://github.com/${LAPTOPS}/tree/HEAD/misc/${folder}`, repo: LAPTOPS, ref: laptopsRef, dir: `misc/${folder}` })),
    ...[...proposed.values()].sort((a, b) => a.folder.localeCompare(b.folder)),
    ...[...surfaceFolders].sort().map((folder) => ({ folder: `linux-surface/${folder}`, url: `https://github.com/${SURFACE}/tree/HEAD/${folder}`, repo: SURFACE, ref: surfaceRef, dir: folder })),
    ...[...reported.values()].sort((a, b) => a.folder.localeCompare(b.folder)),
    ...pending,
    ...(existsSync('acpi') ? readdirSync('acpi').filter((folder) => existsSync(join('acpi', folder, 'dsdt.dat'))).sort() : [])
      .map((folder) => ({ folder: `arewearmyet/${folder}`, url: `https://github.com/birkskyum/arewearmyet/tree/main/acpi/${folder}` })),
    ...(existsSync('acpi/firmware') ? readdirSync('acpi/firmware').filter((folder) => readdirSync(join('acpi/firmware', folder)).some((file) => /^dsdt.*\.dat$/i.test(file))).sort() : [])
      .map((folder) => ({ folder: `firmware/${folder}`, url: `https://github.com/birkskyum/arewearmyet/tree/main/acpi/firmware/${folder}` })),
  ];
}

// One source being down mustn't stop the rest. A part that fails keeps what the last run found, and is
// listed under stale with when it last worked, which the site shows. Only a run where every part fails stops.
const previous = existsSync('data/sources.json') ? JSON.parse(readFileSync('data/sources.json', 'utf8')) : undefined;
if (parts.size < PARTS.length && !previous) throw new Error('a partial collect needs data/sources.json from a full one');
const failed: Partial<Record<(typeof PARTS)[number], string>> = {};
async function attempt<T>(part: (typeof PARTS)[number], collect: () => Promise<T>): Promise<T | undefined> {
  if (!parts.has(part) || failed[part]) return undefined;
  try {
    return await collect();
  } catch (error) {
    failed[part] = error instanceof Error ? error.message.split('\n')[0].slice(0, 300) : String(error);
    return undefined;
  }
}

// The Omarchy recipe's replay reads linux-next too, so it is fetched once before either part starts
let next = '';
if (parts.has('kernel') || parts.has('omarchy')) {
  try {
    next = linuxNextCheckout();
  } catch (error) {
    for (const part of ['kernel', 'omarchy'] as const) if (parts.has(part)) failed[part] = (error as Error).message;
  }
}
const [kernel, hardwareIds, firmware, acpiDumps, omarchy] = await Promise.all([
  attempt('kernel', () => collectDeviceTrees(next)),
  attempt('ids', collectHardwareIds),
  attempt('firmware', collectFirmware),
  attempt('acpi', collectAcpiDumps),
  attempt('omarchy', collectOmarchy),
]);

const failures = Object.entries(failed) as [(typeof PARTS)[number], string][];
for (const [part, message] of failures) console.log(`::warning::${part} wasn't collected, keeping the last run's: ${message}`);
if (failures.length && !previous) throw new Error(`the first collect needs every part: ${failures.map(([part]) => part).join(', ')} failed`);
if (failures.length === parts.size) throw new Error(`nothing could be collected: ${failures.map(([part, message]) => `${part} (${message})`).join('; ')}`);
// A part stays stale from the last run it worked in until it works again
const stale: Record<string, { since: string; error: string }> = { ...previous?.stale };
for (const part of parts) delete stale[part];
for (const [part, message] of failures) stale[part] = { since: previous.stale?.[part]?.since ?? previous.collected, error: message };

const sources = {
  ...previous,
  collected: new Date().toISOString(),
  stale: Object.keys(stale).length ? stale : undefined,
  kernelReleases: KERNEL_RELEASES,
  systemdReleases: SYSTEMD_RELEASES,
  ...kernel,
  ...hardwareIds,
  ...(firmware && { firmwareRelease: firmware.release, firmware: firmware.folders, firmwareMain: firmware.firmwareMain, firmwareReleased: firmware.firmwareReleased }),
  ...(acpiDumps && { acpiDumps }),
  ...(omarchy && { omarchy }),
};
mkdirSync('data', { recursive: true });
writeFileSync('data/sources.json', JSON.stringify(sources, null, 2) + '\n');
const o = sources.omarchy;
console.log(`${[...parts].filter((part) => !failed[part]).join(', ')} collected. Omarchy ${o.channels.map((c: any) => `${c.name} ${c.kernel}/${c.systemd} (${c.deviceTrees.length} trees, ${c.hwids.length}+${c.carriedIds.length} IDs, ${Object.values(c.firmware).flat().length} firmware files)`).join(', ')}, recipe device trees ${o.kernelAddedDeviceTrees.length}, open PRs ${o.pulls.map((pull: any) => `#${pull.number}`).join(' ')}; device trees ${sources.deviceTrees.length}, systemd IDs ${sources.systemd.length}, stubble ${sources.stubble.length}, firmware folders ${sources.firmware.length} (release ${sources.firmwareRelease}), ACPI dumps ${sources.acpiDumps.length}`);
