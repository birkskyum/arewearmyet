// Joins the model list (data/models.yaml) with what the sources say
// (data/sources.json) and writes src/data/status.json for the site.
// Run with `pnpm status`.

import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { parse } from 'yaml';

// The channel whose image people install today
const OMARCHY_CHANNEL = 'rc';

type Variant = { model?: string; chip?: string; display?: string; compatible?: string; ids?: string[] };
type Model = {
  id: string;
  vendor: string;
  name: string;
  generation: 'x1' | 'x2' | '8cx';
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
const kernelRank = (release: string | null) => (release ? sources.kernelReleases.indexOf(release) : Infinity);
const systemdRank = (release: string | null) => (release ? sources.systemdReleases.indexOf(release) : Infinity);

type DeviceTree = { file: string; firstRelease: string | null; inNext: boolean; model?: string; compatible: string[]; firmware?: string[] };
const deviceTrees: DeviceTree[] = sources.deviceTrees;
const omarchyAdded: DeviceTree[] = sources.omarchy.kernelAddedDeviceTrees.map((dt: DeviceTree) => ({ ...dt, firstRelease: null, inNext: false }));

function findDeviceTree(compatible: string | undefined) {
  if (!compatible) return undefined;
  const upstream = deviceTrees.find((dt) => dt.compatible[0] === compatible) ?? deviceTrees.find((dt) => dt.compatible.includes(compatible));
  const added = omarchyAdded.find((dt) => dt.compatible.includes(compatible));
  if (!upstream && !added) return undefined;
  const dt = upstream ?? added!;
  const inOmarchy = Boolean(added) || kernelRank(dt.firstRelease) <= kernelRank(kernelBase);
  return { file: dt.file, compatible: dt.compatible, firstRelease: upstream?.firstRelease ?? null, inNext: upstream?.inNext ?? false, upstream: Boolean(upstream), inOmarchy, firmware: dt.firmware ?? [] };
}

type Ids = { file: string; firstRelease: string | null; inMain: boolean; compatible: string; name: string };
const systemdIds: Ids[] = sources.systemd;

const used = { deviceTrees: new Set<string>(), ids: new Set<string>(), firmware: new Set<string>(), acpi: new Set<string>() };

function variantStatus(variant: Variant) {
  const dt = findDeviceTree(variant.compatible);
  if (dt) used.deviceTrees.add(dt.file);
  const compatibles = dt?.compatible ?? (variant.compatible ? [variant.compatible] : []);
  const named = (variant.ids ?? []).map((file) => systemdIds.find((ids) => ids.file === file)).filter((ids): ids is Ids => Boolean(ids));
  const byCompatible = systemdIds.filter((ids) => compatibles.includes(ids.compatible));
  const ids = [...new Set([...named, ...byCompatible])];
  ids.forEach((entry) => used.ids.add(entry.file));
  const matching = ids.filter((entry) => compatibles.includes(entry.compatible));
  const idsInOmarchy = matching.some((entry) => systemdRank(entry.firstRelease) <= systemdRank(systemdRelease));

  // Omarchy should boot a model when its kernel has the device tree and systemd's list picks it
  const computed = dt?.inOmarchy ? (idsInOmarchy ? 'should' : 'manual') : dt ? 'newer' : 'none';
  return {
    ...variant,
    computed,
    deviceTree: dt ? { file: dt.file, firstRelease: dt.firstRelease, inNext: dt.inNext, upstream: dt.upstream, inOmarchy: dt.inOmarchy } : null,
    firmwareNeeded: dt?.firmware ?? [],
    ids: ids.map((entry) => ({ file: entry.file, firstRelease: entry.firstRelease, inMain: entry.inMain, matches: compatibles.includes(entry.compatible), compatible: entry.compatible })),
  };
}

// What an owner saw outranks what the pieces predict; a model shows its best variant
const OUTCOMES = ['works', 'should', 'problems', 'manual', 'newer', 'none'] as const;
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
const acpiDumps: { folder: string; url: string }[] = sources.acpiDumps;

const rows = models.map((model) => {
  const reports = model.reports ?? [];
  const variants = (model.variants?.length ? model.variants : [{}]).map((variant: Variant) => {
    const status = variantStatus(variant);
    const own = reports.filter((r) => !r.variant || r.variant === variant.model);
    return { ...status, outcome: outcome(status.computed, own), reports: own };
  });
  const firmware = (model.firmware ?? []).map((path) => firmwareFolders.find((folder) => folder.path === path)).filter(Boolean) as typeof firmwareFolders;
  const fw = firmwareState([...new Set(variants.flatMap((v) => v.firmwareNeeded))], firmware, model.firmware_download, model.firmware_posted);
  firmware.forEach((folder) => used.firmware.add(folder.path));
  const acpi = (model.acpi ?? []).map((folder) => acpiDumps.find((dump) => dump.folder === folder)).filter(Boolean) as typeof acpiDumps;
  acpi.forEach((dump) => used.acpi.add(dump.folder));

  const dts = variants.map((v) => v.deviceTree).filter(Boolean) as NonNullable<(typeof variants)[number]['deviceTree']>[];
  // Closest to Linux first: released, queued, posted, then carried by Omarchy or by another kernel
  const upstreamDts = dts.filter((dt) => dt.upstream);
  const deviceTree = upstreamDts.length ? (upstreamDts.some((dt) => dt.firstRelease) ? 'released' : 'next')
    : model.devicetree_posted ? 'posted'
    : dts.length ? 'omarchy'
    : model.devicetree_out_of_tree ? 'out-of-tree'
    : 'missing';
  const firstRelease = dts.map((dt) => dt.firstRelease).filter(Boolean).sort((a, b) => kernelRank(a) - kernelRank(b))[0] ?? null;
  // systemd's list first; otherwise IDs sent in through the board count as collected
  const idsState = variants.some((v) => v.ids.some((entry) => entry.matches)) ? 'listed' : variants.some((v) => v.ids.length) ? 'mismatch' : model.ids_collected ? 'collected' : 'missing';

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
    reports,
    upstream: {
      deviceTree,
      firstRelease,
      devicetreePosted: model.devicetree_posted ?? null,
      devicetreeOutOfTree: model.devicetree_out_of_tree ?? null,
      devicetreeArchive: archived[model.id] ? ARCHIVE + model.id : null,
      ids: idsState,
      idsCollected: model.ids_collected ?? null,
      // What counts is getting the firmware without the laptop's own Windows
      firmware: fw.state,
      firmwareMissing: fw.missing,
      firmwareAudioOnly: fw.state === 'windows' && firmware.length > 0,
      firmwareDownload: model.firmware_download ?? null,
      firmwarePosted: model.firmware_posted ?? null,
      firmwareFolders: firmware.map((folder) => folder.path),
      acpi: acpi.length ? 'collected' : 'missing',
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
const current = rows.filter((row) => row.generation !== '8cx' && row.boot !== 'android');
const summary = {
  models: current.length,
  nvidia: nvidia.length,
  nvidiaWorks: count(nvidia, (m) => m.omarchy === 'works'),
  nvidiaExpected: count(nvidia, (m) => m.omarchy === 'expected'),
  works: count(current, (r) => r.outcome === 'works'),
  should: count(current, (r) => r.outcome === 'should'),
  problems: count(current, (r) => r.outcome === 'problems'),
  notYet: count(current, (r) => ['manual', 'newer', 'none'].includes(r.outcome)),
  deviceTree: count(current, (r) => r.upstream.deviceTree === 'released' || r.upstream.deviceTree === 'next'),
  ids: count(current, (r) => r.upstream.ids === 'listed'),
  firmware: count(current, (r) => r.upstream.firmware === 'linux-firmware' || r.upstream.firmware === 'download'),
  // On its way: posted or carried elsewhere, sent in, or merged for the next release
  deviceTreeComing: count(current, (r) => ['posted', 'omarchy', 'out-of-tree'].includes(r.upstream.deviceTree)),
  idsComing: count(current, (r) => r.upstream.ids === 'collected' || r.upstream.ids === 'mismatch'),
  firmwareComing: count(current, (r) => r.upstream.firmware === 'next-release' || r.upstream.firmware === 'posted'),
  // Hardware IDs and Windows firmware only apply to models that start through UEFI
  googlebooks: count(rows, (r) => r.boot === 'android'),
  // A dump only matters while no device tree exists for a model anywhere, posted or out of tree included
  acpiNeeded: count(current, (r) => r.upstream.deviceTree === 'missing'),
  acpi: count(current, (r) => r.upstream.deviceTree === 'missing' && r.upstream.acpi === 'collected'),
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
  JSON.stringify({ collected: sources.collected, linuxNext: sources.linuxNext, firmwareRelease: sources.firmwareRelease, omarchy: { channel: OMARCHY_CHANNEL, kernel: channel.kernel, systemd: channel.systemd, kernelBase, systemdRelease, channels: sources.omarchy.channels }, summary, models: rows, nvidia, unmatched }, null, 2) + '\n',
);
console.log(summary);
console.log(`unmatched: ${unmatched.deviceTrees.length} device trees, ${unmatched.ids.length} ID entries, ${unmatched.firmware.length} firmware folders, ${unmatched.acpi.length} ACPI dumps`);
