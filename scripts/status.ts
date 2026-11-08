// Joins the model list (data/models.yaml) with what the sources say
// (data/sources.json) and writes src/data/status.json for the site.
// Run with `pnpm status`.

import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { parse } from 'yaml';

// The channel testers install, from the edge nightly image
const OMARCHY_CHANNEL = 'edge' as const;

type Variant = { model?: string; chip?: string; display?: string; compatible?: string; ids?: string[]; borrowed?: boolean };
type Model = {
  id: string;
  vendor: string;
  name: string;
  generation: 'x1' | 'x2' | '8cx' | 'c';
  form?: string;
  released?: string | number;
  model_numbers?: string[];
  chips?: string[];
  status?: string;
  sources?: string[];
  variants?: Variant[];
  firmware?: string[];
  firmware_download?: string;
  firmware_posted?: string;
  acpi?: string[];
  devicetree_posted?: string;
  devicetree_out_of_tree?: string;
  ids_collected?: string;
  boot?: 'android';
  bootloader?: 'unlockable' | 'locked';
  linux_image?: 'boots' | 'none';
  discussion?: string;
  team_has_one?: boolean;
  notes?: string;
  reports?: Report[];
};
type Report = { state: 'works' | 'problems'; variant?: string; note: string; checked?: string };

const sources = JSON.parse(readFileSync('data/sources.json', 'utf8'));
const list = parse(readFileSync('data/models.yaml', 'utf8'));
// Copies of trees not in Linux yet, made by scripts/trees.ts
const archived: Record<string, unknown> = (() => { try { return JSON.parse(readFileSync('device-trees/index.json', 'utf8')); } catch { return {}; } })();
const ARCHIVE = 'https://github.com/birkskyum/arewearmyet/tree/main/device-trees/';
const models: Model[] = list.models;
const ignore: { ids?: string[]; firmware?: string[]; acpi?: string[] } = list.ignore ?? {};

// NVIDIA's ARM PCs boot through ACPI; their status is what testers report
type NvidiaModel = { id: string; vendor: string; name: string; generation: 'n1x' | 'gb10'; form?: string; released?: string | number; omarchy: 'works' | 'expected' | 'untested' | 'problems' | 'missing'; linux: 'omarchy' | 'mainline'; status?: string; notes?: string; sources?: string[] };
const nvidia: NvidiaModel[] = (list.nvidia ?? []).map((model: NvidiaModel) => ({ form: 'laptop', released: null, status: null, notes: null, sources: [], ...model }));

// An unquoted vendor,model inside { } is split at the comma by YAML, so catch it here
for (const model of models) {
  for (const variant of model.variants ?? []) {
    if (variant.compatible !== undefined && !String(variant.compatible).includes(',')) {
      throw new Error(`${model.id}: compatible "${variant.compatible}" has no vendor prefix; quote it in data/models.yaml`);
    }
  }
}

const channel = sources.omarchy.channels.find((c: { name: string }) => c.name === OMARCHY_CHANNEL);
// A package version is [epoch:]version-pkgrel, and 7.3-3 has no third number to cut off
const upstreamVersion = (version: string) => version.replace(/^\d+:/, '').replace(/-[^-]*$/, '');
const kernelBase = `v${upstreamVersion(channel.kernel).split('.').slice(0, 2).join('.')}`;
const systemdRelease = `v${upstreamVersion(channel.systemd).split('.')[0]}`;
if (!sources.kernelReleases.includes(kernelBase)) throw new Error(`Omarchy's ${OMARCHY_CHANNEL} kernel ${channel.kernel} is ${kernelBase}, which KERNEL_RELEASES in scripts/collect.ts doesn't list`);
if (!sources.systemdReleases.includes(systemdRelease)) throw new Error(`Omarchy's ${OMARCHY_CHANNEL} systemd ${channel.systemd} is ${systemdRelease}, which SYSTEMD_RELEASES in scripts/collect.ts doesn't list`);

// Release order, so "first in v6.15" can be compared with "Omarchy builds v7.2"
// Where Omarchy's kernel keeps the device trees it adds on top of a Linux release
const OMARCHY_PATCHES = 'https://github.com/omacom/omarchy-pkgs/blob/master/pkgbuilds/linux-omarchy-aarch64/';
const kernelRank = (release: string | null) => (release ? sources.kernelReleases.indexOf(release) : Infinity);

type DeviceTree = { file: string; firstRelease: string | null; inNext: boolean; model?: string; compatible: string[]; firmware?: string[]; patch?: string };
const deviceTrees: DeviceTree[] = sources.deviceTrees;
const omarchyAdded: DeviceTree[] = sources.omarchy.kernelAddedDeviceTrees.map((dt: DeviceTree) => ({ ...dt, firstRelease: null, inNext: false }));

// Glymur's boot firmware starts the SoCCP and Linux only attaches to it, so no package ships the
// soccp images a tree names. The Surface Laptop 8's USB4 router image exists only inside Qualcomm's
// Windows USB4 driver, which microsoft-surface-laptop-8-support copies it out of, so no firmware package
// carries it either
const loadedByLinux = (name: string) => !/\/soccp[^/]*$/.test(name) && !/\/usb4-router[^/]*$/.test(name);

function findDeviceTree(compatible: string | undefined) {
  if (!compatible) return undefined;
  const upstream = deviceTrees.find((dt) => dt.compatible[0] === compatible) ?? deviceTrees.find((dt) => dt.compatible.includes(compatible));
  const added = omarchyAdded.find((dt) => dt.compatible.includes(compatible));
  if (!upstream && !added) return undefined;
  const dt = upstream ?? added!;
  // Omarchy's kernel may carry a tree under its own file name before Linux takes it
  const files = [...new Set([upstream?.file, added?.file].filter(Boolean) as string[])];
  return { file: dt.file, files, compatible: dt.compatible, firstRelease: upstream?.firstRelease ?? null, inNext: upstream?.inNext ?? false, upstream: Boolean(upstream), firmware: (dt.firmware ?? []).filter(loadedByLinux), patch: upstream ? null : added?.patch ?? null };
}

type Ids = { file: string; firstRelease: string | null; inMain: boolean; compatible: string; name: string };
const systemdIds: Ids[] = sources.systemd;
type Stubble = { file: string; state: 'merged' | 'open'; url: string; name: string; compatible: string };
const stubbleIds: Stubble[] = sources.stubble;

const used = { deviceTrees: new Set<string>(), ids: new Set<string>(), firmware: new Set<string>(), acpi: new Set<string>() };

// What each Omarchy channel ships, read from its packages' file lists, most stable channel first: the
// device trees in its kernel, the compatibles that systemd's hardware IDs and the kernel's own pick,
// and every firmware file with the package it comes in
const CHANNELS = ['stable', 'rc', 'edge'] as const;
type Channel = (typeof CHANNELS)[number];
type ChannelSource = { name: string; kernel: string | null; systemd: string | null; deviceTrees: string[]; hwids: string[]; carriedIds: string[]; firmware: Record<string, string[]> };
const carriedIds: { file: string; name: string; compatible: string }[] = sources.omarchy.carriedIds ?? [];
const shipped = Object.fromEntries(
  CHANNELS.map((name) => {
    const source: ChannelSource | undefined = sources.omarchy.channels.find((c: ChannelSource) => c.name === name);
    const compatibles = [
      ...(source?.hwids ?? []).map((file) => systemdIds.find((ids) => ids.file === file)?.compatible),
      ...(source?.carriedIds ?? []).map((file) => carriedIds.find((ids) => ids.file === file)?.compatible),
    ];
    return [name, {
      trees: new Set(source?.deviceTrees ?? []),
      ids: new Set(compatibles.filter(Boolean) as string[]),
      idFiles: new Set([...(source?.hwids ?? []), ...(source?.carriedIds ?? [])]),
      firmware: new Map(Object.entries(source?.firmware ?? {}).flatMap(([pkg, files]) => files.map((file) => [file, pkg] as const))),
    }];
  }),
) as Record<Channel, { trees: Set<string>; ids: Set<string>; idFiles: Set<string>; firmware: Map<string, string> }>;
const channelRank = (name: Channel | null) => (name ? CHANNELS.indexOf(name) : Infinity);
const mostStable = (names: (Channel | null)[]) => [...names].filter(Boolean).sort((a, b) => channelRank(a) - channelRank(b))[0] ?? null;
// Every Omarchy kernel carries the N1x and GB10 patches, so an NVIDIA model's kernel is the most stable channel's
const nvidiaKernel = CHANNELS.find((name) => sources.omarchy.channels.find((c: ChannelSource) => c.name === name)?.kernel) ?? null;

// Open omarchy-pkgs pull requests that would add a device tree, hardware IDs or firmware files
type Pull = { number: number; title: string; url: string; draft: boolean; deviceTrees: { file: string; compatible: string[] }[]; ids: { file: string; compatible: string }[]; firmware: string[] };
const pulls: Pull[] = sources.omarchy.pulls ?? [];
const pullRef = (pull: Pull | undefined) => (pull ? { url: pull.url, number: pull.number, title: pull.title } : null);

function variantStatus(variant: Variant) {
  const dt = findDeviceTree(variant.compatible);
  if (dt) used.deviceTrees.add(dt.file);
  const compatibles = dt?.compatible ?? (variant.compatible ? [variant.compatible] : []);
  // systemd-stub compares a hardware ID entry only with a tree's first compatible
  const picked = compatibles[0];
  const named = (variant.ids ?? []).map((file) => systemdIds.find((ids) => ids.file === file)).filter((ids): ids is Ids => Boolean(ids));
  // A borrowed tree is another model's, whose hardware IDs don't pick it on this one, so only the files the
  // variant names count
  const byCompatible = variant.borrowed ? [] : systemdIds.filter((ids) => compatibles.includes(ids.compatible));
  const ids = [...new Set([...named, ...byCompatible])];
  ids.forEach((entry) => used.ids.add(entry.file));
  const ownIds = (file: string) => (variant.ids ?? []).includes(file);
  // The first channel whose kernel ships the tree, and whose hardware IDs pick it
  const treeChannel = dt ? CHANNELS.find((name) => dt.files.some((file) => shipped[name].trees.has(file))) ?? null : null;
  const idsChannel = variant.borrowed
    ? CHANNELS.find((name) => [...shipped[name].idFiles].some(ownIds)) ?? null
    : picked ? CHANNELS.find((name) => shipped[name].ids.has(picked)) ?? null : null;
  const treeCompatible = variant.compatible ? [variant.compatible] : compatibles;
  const treePull = treeChannel ? undefined : pulls.find((pull) => pull.deviceTrees.some((tree) => tree.compatible.some((compatible) => treeCompatible.includes(compatible))));
  const idsPull = idsChannel ? undefined : pulls.find((pull) => pull.ids.some((entry) => (variant.borrowed ? ownIds(entry.file) : entry.compatible === picked)));

  // Omarchy should boot a model when the kernel of the channel people install has its device tree and
  // the hardware IDs there pick it
  const treeHere = channelRank(treeChannel) <= channelRank(OMARCHY_CHANNEL);
  const idsHere = channelRank(idsChannel) <= channelRank(OMARCHY_CHANNEL);
  const computed = treeHere ? (idsHere ? 'should' : 'manual') : dt ? 'notCarried' : 'none';
  // The most stable channel whose install image boots it: its device tree and the hardware IDs that pick it
  const bootChannel = treeChannel && idsChannel ? CHANNELS[Math.max(channelRank(treeChannel), channelRank(idsChannel))] : null;
  return {
    ...variant,
    computed,
    treeChannel,
    idsChannel,
    bootChannel,
    treePull: pullRef(treePull),
    idsPull: pullRef(idsPull),
    deviceTree: dt ? { file: dt.file, firstRelease: dt.firstRelease, inNext: dt.inNext, upstream: dt.upstream, patch: dt.patch } : null,
    firmwareNeeded: dt?.firmware ?? [],
    ids: ids.map((entry) => ({ file: entry.file, firstRelease: entry.firstRelease, inMain: entry.inMain, matches: entry.compatible === picked, compatible: entry.compatible })),
    stubble: stubbleIds.filter((entry) => (variant.borrowed ? ownIds(entry.file) : entry.compatible === picked)).map(({ file, state, url }) => ({ file, state, url })),
  };
}

// What an owner saw outranks what the pieces predict; a model shows its best variant
const OUTCOMES = ['works', 'should', 'problems', 'manual', 'notCarried', 'none'] as const;
type Outcome = (typeof OUTCOMES)[number];
const best = (values: Outcome[]) => [...values].sort((a, b) => OUTCOMES.indexOf(a) - OUTCOMES.indexOf(b))[0];

function outcome(computed: string, reports: Report[]): Outcome {
  if (reports.some((r) => r.state === 'problems')) return 'problems';
  if (reports.some((r) => r.state === 'works')) return 'works';
  return computed as Outcome;
}

const firmwareFolders: { path: string; files: string[] }[] = sources.firmware;
const firmwareReleased = new Set<string>(sources.firmwareReleased);
const firmwareMain = new Set<string>(sources.firmwareMain);

// data/firmware-sources.yaml lists the makers' downloads holding the firmware linux-firmware doesn't
// ship yet. A model has a vendor download when every file its device tree loads is in linux-firmware
// or in one of them; without a device tree, when every file of its folders there is in one.
type SourceFolder = { path: string; models: string[]; files?: { name: string; from: unknown[] }[]; links?: { name: string; target: string }[] };
const sourceFolders: SourceFolder[] = parse(readFileSync('data/firmware-sources.yaml', 'utf8')).folders;
const sourced = new Set(sourceFolders.flatMap((folder) => (folder.files ?? []).filter((file) => file.from.length).map((file) => `${folder.path}/${file.name}`)));
for (const folder of sourceFolders) {
  for (const link of folder.links ?? []) {
    const target = new URL(link.target, `file:///${folder.path}/${link.name}`).pathname.slice(1);
    if (sourced.has(target)) sourced.add(`${folder.path}/${link.name}`);
  }
}
// Trees outside Linux are only known through their copies in device-trees/, so the files they load
// come from the firmware-name lines there: in the .dts naming one of the model's compatibles (a series
// can add trees for several models) and the files it includes
// A model that shares another's board and tree, like the OmniBook 5 16 the OmniBook 5 14's, has no copy of
// its own and reads that model's
function archivedFirmware(id: string, compatibles: string[]): string[] {
  const from = archived[id] ? id : models.find((other: Model) => archived[other.id] && (other.variants ?? []).some((v: Variant) => v.compatible && compatibles.includes(v.compatible)))?.id;
  const entry = from ? archived[from] as { files?: string[] } : undefined;
  const files = new Map<string, string>();
  for (const file of entry?.files ?? []) {
    if (/\.dtsi?$/.test(file)) files.set(file, readFileSync(`device-trees/${from}/${file}`, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, ''));
  }
  const boards = [...files.keys()].filter((file) => file.endsWith('.dts'));
  const own = boards.filter((file) => compatibles.some((compatible) => files.get(file)!.includes(`"${compatible}"`)));
  const todo = own.length ? own : boards;
  const seen = new Set<string>();
  const names = new Set<string>();
  while (todo.length) {
    const file = todo.pop()!;
    if (seen.has(file) || !files.has(file)) continue;
    seen.add(file);
    const text = files.get(file)!;
    for (const include of text.matchAll(/#include\s+"([^"]+)"/g)) todo.push(include[1]);
    for (const line of text.matchAll(/firmware-name\s*=\s*([^;]*);/g)) {
      for (const name of line[1].matchAll(/"([^"]+)"/g)) names.add(name[1]);
    }
  }
  return [...names].filter(loadedByLinux);
}
const anchor = (path: string) => `/firmware/#${path.replaceAll('/', '-')}`;
function sourcesDownload(id: string, needed: string[]): string | undefined {
  if (needed.length) {
    const fromMaker = needed.filter((file) => sourced.has(file));
    if (!fromMaker.length || !needed.every((file) => firmwareMain.has(file) || sourced.has(file))) return undefined;
    return anchor(fromMaker[0].slice(0, fromMaker[0].lastIndexOf('/')));
  }
  const folders = sourceFolders.filter((folder) => folder.models.includes(id));
  if (!folders.length || folders.some((folder) => (folder.files ?? []).some((file) => !file.from.length))) return undefined;
  return anchor(folders[0].path);
}

// Firmware counts as there when every file the device tree loads is in linux-firmware: in its
// newest release, which distributions ship, or already merged for the next one. Models without a
// device tree fall back to their folder having the signed audio DSP image. A vendor download works
// today, so it comes before a merge request that is still waiting for review.
function firmwareState(needed: string[], folders: { path: string; files: string[] }[], download?: string, posted?: string) {
  const fallback = folders.flatMap((folder) => folder.files.filter((file) => /^qcadsp\d+\.mbn$/.test(file)).map((file) => `${folder.path}/${file}`));
  const files = needed.length ? needed : fallback;
  const missingReleased = files.filter((file) => !firmwareReleased.has(file));
  const missingMain = files.filter((file) => !firmwareMain.has(file));
  const name = (file: string) => file.split('/').pop()!;
  if (files.length && !missingReleased.length) return { state: 'linux-firmware', missing: [] as string[] };
  if (files.length && !missingMain.length) return { state: 'next-release', missing: missingReleased.map(name) };
  if (download) return { state: 'download', missing: [] as string[] };
  if (posted) return { state: 'posted', missing: [] as string[] };
  // Some files there, some not: say which are still missing
  if (needed.length && missingMain.length < needed.length) return { state: 'partial', missing: missingMain.map(name) };
  return { state: 'windows', missing: missingMain.map(name) };
}

// Upstream on its own: whether linux-firmware has every file, in a release, merged for the next one,
// sent as a merge request, partly, or not at all
function upstreamFirmware(files: string[], posted?: string) {
  const name = (file: string) => file.split('/').pop()!;
  if (files.length && files.every((file) => firmwareReleased.has(file))) return { state: 'released', missing: [] as string[] };
  if (files.length && files.every((file) => firmwareMain.has(file))) return { state: 'next-release', missing: files.filter((file) => !firmwareReleased.has(file)).map(name) };
  if (posted) return { state: 'posted', missing: [] as string[] };
  if (files.some((file) => firmwareMain.has(file))) return { state: 'partial', missing: files.filter((file) => !firmwareMain.has(file)).map(name) };
  return { state: 'missing', missing: [] as string[] };
}

// In Omarchy: the first channel whose packages hold every file, from linux-firmware or a maker's package.
// Otherwise an open pull request that adds the rest, some of the files, a maker's download it could
// package, or the laptop's own Windows.
function omarchyFirmware(files: string[], download?: string) {
  for (const name of CHANNELS) {
    if (files.length && files.every((file) => shipped[name].firmware.has(file))) {
      return { state: 'shipped', channel: name as Channel, packages: [...new Set(files.map((file) => shipped[name].firmware.get(file)!))].sort(), pull: null };
    }
  }
  const newest = shipped[CHANNELS[CHANNELS.length - 1]];
  const pull = files.length ? pulls.find((p) => files.every((file) => newest.firmware.has(file) || p.firmware.includes(file))) : undefined;
  if (pull) return { state: 'pr', channel: null, packages: [] as string[], pull: pullRef(pull) };
  const state = files.some((file) => newest.firmware.has(file)) ? 'partial' : download ? 'download' : 'windows';
  return { state, channel: null, packages: [] as string[], pull: null };
}

const acpiDumps: { folder: string; url: string }[] = sources.acpiDumps;

// Whether a fresh install boots it: problems an owner reported, otherwise the most stable channel whose install
// image picks its device tree, whether or not an owner has tried it yet. Without one, what it still needs
// to boot, the first piece nobody has, then pieces Omarchy doesn't carry, then open pull requests.
// Firmware isn't needed to boot, so the firmware column covers it.
type Piece = Channel | 'pr' | 'elsewhere' | 'none';
function bootable(outcome: Outcome, channel: Channel | null, pieces: Record<'deviceTree' | 'ids', Piece>) {
  if (outcome === 'problems') return 'problems';
  if (channel) return channel;
  if (outcome === 'works') return 'works';
  const values = [pieces.deviceTree, pieces.ids];
  if (pieces.deviceTree === 'none') return 'needs-deviceTree';
  if (pieces.ids === 'none') return 'needs-ids';
  if (values.includes('elsewhere')) return 'carry';
  if (values.includes('pr')) return 'pr';
  // Every piece ships, but for different variants
  return 'needs-ids';
}

// Who we know has the machine: the team, someone in Omarchy's community (a report on Omarchy, a hardware
// report from its installer tool, hardware IDs sent in here or an Omarchy discussion), someone in the wider
// Linux community (a tree in Linux, a posted patch, an owner's repository or an aarch64-laptops dump), or
// nobody (no tree, no tables, no report)
function whoHasOne(model: Model, reports: Report[], deviceTree: string, acpi: string[]) {
  if (model.team_has_one) return 'team';
  // A tree borrowed from another model on the same mainboard says nothing about who has this one
  const variants: Variant[] = model.variants ?? [];
  const ownTree = deviceTree !== 'missing' && !(variants.length && variants.every((variant) => variant.borrowed));
  // Tables taken out of a maker's firmware image say nothing about who has one either
  const dumps = acpi.filter((folder) => !folder.startsWith('firmware/'));
  if (!ownTree && !dumps.length && !reports.length) return 'nobody';
  const sentToOmarchy = /^https:\/\/github\.com\/(omacom|turbineBMW\/omarchy-hardware-reports|birkskyum\/arewearmyet)\//.test(model.ids_collected ?? '');
  const omarchyCommunity = reports.length > 0 || dumps.some((folder) => folder.startsWith('hardware-reports/')) || sentToOmarchy || Boolean(model.discussion?.includes('omacom/omarchy'));
  return omarchyCommunity ? 'omarchy' : 'linux';
}

const rows = models.map((model) => {
  const reports = model.reports ?? [];
  const variants = (model.variants?.length ? model.variants : [{}]).map((variant: Variant) => {
    const status = variantStatus(variant);
    const own = reports.filter((r) => !r.variant || r.variant === variant.model);
    return { ...status, outcome: outcome(status.computed, own), reports: own };
  });
  const firmware = (model.firmware ?? []).map((path) => firmwareFolders.find((folder) => folder.path === path)).filter(Boolean) as typeof firmwareFolders;
  const fromTrees = [...new Set(variants.flatMap((v) => v.firmwareNeeded))];
  const needed = fromTrees.length ? fromTrees : archivedFirmware(model.id, (model.variants ?? []).map((v: Variant) => v.compatible).filter(Boolean) as string[]);
  const download = model.firmware_download ?? sourcesDownload(model.id, needed);
  const fw = firmwareState(needed, firmware, download, model.firmware_posted);
  // Models without a device tree fall back to their folder's signed audio DSP image, as firmwareState does
  const fwFiles = needed.length ? needed : firmware.flatMap((folder) => folder.files.filter((file) => /^qcadsp\d+\.mbn$/.test(file)).map((file) => `${folder.path}/${file}`));
  const omarchyFw = omarchyFirmware(fwFiles, download);
  const firmwareUpstream = upstreamFirmware(fwFiles, model.firmware_posted);
  firmware.forEach((folder) => used.firmware.add(folder.path));
  const acpi = (model.acpi ?? []).map((folder) => acpiDumps.find((dump) => dump.folder === folder)).filter(Boolean) as typeof acpiDumps;
  acpi.forEach((dump) => used.acpi.add(dump.folder));

  const dts = variants.map((v) => v.deviceTree).filter(Boolean) as NonNullable<(typeof variants)[number]['deviceTree']>[];
  // Closest to Linux first: released, queued, posted, then out of tree, whether Omarchy's kernel or another
  // kernel or repository carries it
  const upstreamDts = dts.filter((dt) => dt.upstream);
  const deviceTree = upstreamDts.length ? (upstreamDts.some((dt) => dt.firstRelease) ? 'released' : 'next')
    : model.devicetree_posted ? 'posted'
    : dts.length || model.devicetree_out_of_tree ? 'out-of-tree'
    : 'missing';
  const omarchyPatch = dts.find((dt) => dt.patch)?.patch;
  const firstRelease = dts.map((dt) => dt.firstRelease).filter(Boolean).sort((a, b) => kernelRank(a) - kernelRank(b))[0] ?? null;
  // systemd's list first, then merged into stubble or open as a pull request there; otherwise IDs sent in
  // through the board count as collected
  const stubble = variants.flatMap((v) => v.stubble);
  const stubbleEntry = stubble.find((entry) => entry.state === 'merged') ?? stubble[0];
  const idsState = variants.some((v) => v.ids.some((entry) => entry.matches)) ? 'listed'
    : stubbleEntry ? (stubbleEntry.state === 'merged' ? 'ubuntu' : 'posted')
    : variants.some((v) => v.ids.length) ? 'mismatch'
    : model.ids_collected ? 'collected'
    : 'missing';

  // What Omarchy's channels ship for it, each piece by the most stable channel that has it
  const omarchy = {
    deviceTree: mostStable(variants.map((v) => v.treeChannel)),
    deviceTreePull: variants.map((v) => v.treePull).find(Boolean) ?? null,
    ids: mostStable(variants.map((v) => v.idsChannel)),
    idsPull: variants.map((v) => v.idsPull).find(Boolean) ?? null,
    firmware: omarchyFw.state,
    firmwareChannel: omarchyFw.channel,
    firmwarePackages: omarchyFw.packages,
    firmwarePull: omarchyFw.pull,
  };
  // Each piece: the channel that ships it, an open pull request, somewhere else Omarchy could take it from,
  // or nowhere yet
  const pieces = {
    deviceTree: omarchy.deviceTree ?? (omarchy.deviceTreePull ? 'pr' : deviceTree !== 'missing' ? 'elsewhere' : 'none'),
    ids: omarchy.ids ?? (omarchy.idsPull ? 'pr' : idsState !== 'missing' ? 'elsewhere' : 'none'),
    firmware: omarchy.firmwareChannel ?? (omarchy.firmware === 'pr' ? 'pr' : download || ['next-release', 'posted'].includes(firmwareUpstream.state) ? 'elsewhere' : 'none'),
  };

  return {
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    generation: model.generation,
    form: model.form ?? 'laptop',
    released: model.released ?? null,
    modelNumbers: model.model_numbers ?? [],
    chips: model.chips ?? [],
    status: model.status ?? null,
    sources: model.sources ?? [],
    notes: model.notes ?? null,
    outcome: best(variants.map((v) => v.outcome)),
    bootable: bootable(best(variants.map((v) => v.outcome)), mostStable(variants.map((v) => v.bootChannel)), pieces),
    reports,
    machine: whoHasOne(model, reports, deviceTree, model.acpi ?? []),
    omarchy,
    pieces,
    upstream: {
      deviceTree,
      firstRelease,
      devicetreePosted: model.devicetree_posted ?? null,
      devicetreeOutOfTree: model.devicetree_out_of_tree ?? (omarchyPatch ? OMARCHY_PATCHES + omarchyPatch : null),
      devicetreeArchive: archived[model.id] ? ARCHIVE + model.id : null,
      ids: idsState,
      idsCollected: model.ids_collected ?? null,
      idsStubble: idsState === 'ubuntu' || idsState === 'posted' ? stubbleEntry!.url : null,
      // What counts is getting the firmware without the laptop's own Windows
      firmware: fw.state,
      firmwareMissing: fw.missing,
      firmwareAudioOnly: fw.state === 'windows' && firmware.length > 0,
      firmwareDownload: download ?? null,
      firmwarePosted: model.firmware_posted ?? null,
      firmwareUpstream: firmwareUpstream.state,
      firmwareUpstreamMissing: firmwareUpstream.missing,
      firmwareFolders: firmware.map((folder) => folder.path),
      acpi: acpi.some((dump) => !dump.folder.startsWith('firmware/')) ? 'collected' : acpi.length ? 'firmware' : 'missing',
      acpiUrls: acpi.map((dump) => dump.url),
    },
    variants,
    boot: model.boot ?? 'uefi',
    bootloader: model.bootloader ?? null,
    linuxImage: model.linux_image ?? null,
    discussion: model.discussion ?? null,
  };
});

const count = <T>(items: T[], test: (item: T) => boolean) => items.filter(test).length;
type Row = (typeof rows)[number];
const current = rows.filter((row) => row.generation !== '8cx' && row.generation !== 'c' && row.boot !== 'android');
const summary = {
  models: current.length,
  nvidia: nvidia.length,
  nvidiaWorks: count(nvidia, (m) => m.omarchy === 'works'),
  nvidiaExpected: count(nvidia, (m) => m.omarchy === 'expected'),
  works: count(current, (r) => r.outcome === 'works'),
  should: count(current, (r) => r.outcome === 'should'),
  problems: count(current, (r) => r.outcome === 'problems'),
  notYet: count(current, (r) => ['manual', 'notCarried', 'none'].includes(r.outcome)),
  // What we have to run each model, wherever it is: a device tree in Linux or out of tree, hardware IDs
  // listed or sent in, and firmware we can get without the machine's own Windows
  deviceTree: count(current, (r) => r.upstream.deviceTree !== 'missing'),
  ids: count(current, (r) => r.upstream.ids !== 'missing'),
  firmware: count(current, (r) => ['linux-firmware', 'next-release', 'download', 'posted'].includes(r.upstream.firmware)),
  // Hardware IDs and Windows firmware only apply to models that start through UEFI
  googlebooks: count(rows, (r) => r.boot === 'android'),
  // Every dump collected counts, also once its model has a device tree; one is still wanted while a model
  // has no device tree anywhere, posted or out of tree included
  acpi: count(current, (r) => r.upstream.acpi === 'collected'),
  acpiWanted: count(current, (r) => r.upstream.deviceTree === 'missing' && r.upstream.acpi !== 'collected'),
  machine: Object.fromEntries(['team', 'omarchy', 'linux', 'nobody'].map((who) => [who, count(current, (r) => r.machine === who)])),
  // Where each piece stands: in an Omarchy channel, in the releases of Linux, systemd and linux-firmware
  // that any distro can ship, in an open pull request or elsewhere, and all three together
  byPiece: (() => {
    const omarchy = {
      deviceTree: (r: Row) => r.omarchy.deviceTree !== null,
      ids: (r: Row) => r.omarchy.ids !== null,
      firmware: (r: Row) => r.omarchy.firmwareChannel !== null,
    };
    const upstream = {
      deviceTree: (r: Row) => r.upstream.deviceTree === 'released',
      ids: (r: Row) => r.variants.some((v) => v.ids.some((entry) => entry.matches && entry.firstRelease)),
      firmware: (r: Row) => r.upstream.firmwareUpstream === 'released',
    };
    // Exists elsewhere, but Omarchy doesn't carry it yet and no pull request adds it: a tree in Linux, linux-next,
    // a posted patch or out of tree, hardware IDs anywhere, firmware in a maker's download or on its way to linux-firmware
    const available = {
      deviceTree: (r: Row) => !omarchy.deviceTree(r) && !pull.deviceTree(r) && r.upstream.deviceTree !== 'missing',
      ids: (r: Row) => !omarchy.ids(r) && !pull.ids(r) && r.upstream.ids !== 'missing',
      firmware: (r: Row) => !omarchy.firmware(r) && !pull.firmware(r) && (r.upstream.firmwareDownload !== null || ['next-release', 'posted'].includes(r.upstream.firmwareUpstream)),
    };
    const pieces = ['deviceTree', 'ids', 'firmware'] as const;
    // Omarchy's count split into what it has from upstream and what it adds
    const split = (ours: (r: Row) => boolean, theirs: (r: Row) => boolean, open: (r: Row) => boolean, elsewhere: (r: Row) => boolean, channels?: Record<string, number>) => ({
      omarchy: count(current, ours),
      fromUpstream: count(current, (r) => ours(r) && theirs(r)),
      added: count(current, (r) => ours(r) && !theirs(r)),
      pull: count(current, open),
      available: count(current, elsewhere),
      upstream: count(current, theirs),
      ...(channels ? { channels } : {}),
      // Which models make up each part, so the page can draw one cell per model
      models: {
        fromUpstream: ids((r) => ours(r) && theirs(r)),
        added: ids((r) => ours(r) && !theirs(r)),
        pull: ids(open),
        available: ids(elsewhere),
      },
    });
    const ids = (test: (r: Row) => boolean) => current.filter(test).map((r) => r.id);
    const byChannel = (piece: (typeof pieces)[number]) =>
      Object.fromEntries(CHANNELS.map((name) => [name, count(current, (r) => channelRank(piece === 'firmware' ? r.omarchy.firmwareChannel : r.omarchy[piece]) <= channelRank(name))]));
    // Not shipped yet, but an open omarchy-pkgs pull request adds it
    const pull = {
      deviceTree: (r: Row) => !omarchy.deviceTree(r) && r.omarchy.deviceTreePull !== null,
      ids: (r: Row) => !omarchy.ids(r) && r.omarchy.idsPull !== null,
      firmware: (r: Row) => r.omarchy.firmware === 'pr',
    };
    return {
      ...Object.fromEntries(pieces.map((piece) => [piece, split(omarchy[piece], upstream[piece], pull[piece], available[piece], byChannel(piece))])),
      all: split(
        (r) => pieces.every((piece) => omarchy[piece](r)),
        (r) => pieces.every((piece) => upstream[piece](r)),
        (r) => pieces.every((piece) => omarchy[piece](r) || pull[piece](r)) && pieces.some((piece) => pull[piece](r)),
        (r) => pieces.every((piece) => omarchy[piece](r) || pull[piece](r) || available[piece](r)) && pieces.some((piece) => available[piece](r)),
      ),
    };
  })(),
};

// Anything the sources know that no model claims yet, so the list can be completed
const unmatched = {
  deviceTrees: deviceTrees.filter((dt) => !used.deviceTrees.has(dt.file) && !/(crd|iot|evk|qcp|qcb|som)/.test(dt.file)).map((dt) => ({ file: dt.file, model: dt.model, compatible: dt.compatible[0] })),
  ids: systemdIds.filter((ids) => !used.ids.has(ids.file) && !ignore.ids?.includes(ids.file)).map((ids) => ({ file: ids.file, name: ids.name, compatible: ids.compatible })),
  firmware: firmwareFolders.filter((folder) => !used.firmware.has(folder.path) && !ignore.firmware?.includes(folder.path)).map((folder) => folder.path),
  acpi: acpiDumps.filter((dump) => !used.acpi.has(dump.folder) && !ignore.acpi?.includes(dump.folder)).map((dump) => dump.folder),
};

mkdirSync('src/data', { recursive: true });
writeFileSync(
  'src/data/status.json',
  JSON.stringify({ collected: sources.collected, linuxNext: sources.linuxNext, firmwareRelease: sources.firmwareRelease, omarchy: { channel: OMARCHY_CHANNEL, kernel: channel.kernel, systemd: channel.systemd, kernelBase, systemdRelease, image: sources.omarchy.image ?? null, channels: sources.omarchy.channels }, summary, models: rows, nvidia: nvidia.map((model) => ({ ...model, kernel: nvidiaKernel })), unmatched }, null, 2) + '\n',
);
console.log(summary);
// A file both linux-firmware-qcom and one of Omarchy's firmware packages ship on edge. The package's copy is
// xz-compressed, so the kernel loads linux-firmware's, but the recipe should drop it: regenerate it with
// scripts/snapdragon-firmware-recipes.py, which also stops when linux-firmware has only half a DSP's set.
const edgeFirmware: Record<string, string[]> = sources.omarchy.channels.find((c: { name: string }) => c.name === 'edge')?.firmware ?? {};
const inLinuxFirmware = new Set(edgeFirmware['linux-firmware-qcom'] ?? []);
for (const [pkg, files] of Object.entries(edgeFirmware)) {
  if (pkg === 'linux-firmware-qcom') continue;
  for (const file of files.filter((f) => inLinuxFirmware.has(f))) console.log(`::warning::linux-firmware-qcom now ships ${file}, which ${pkg} also installs; regenerate its recipe`);
}
console.log(`unmatched: ${unmatched.deviceTrees.length} device trees, ${unmatched.ids.length} ID entries, ${unmatched.firmware.length} firmware folders, ${unmatched.acpi.length} ACPI dumps`);
