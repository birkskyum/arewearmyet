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
  acpi?: string[];
  devicetree_posted?: string;
  notes?: string;
};

const sources = JSON.parse(readFileSync('data/sources.json', 'utf8'));
const list = parse(readFileSync('data/models.yaml', 'utf8'));
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
const kernelBase = `v${channel.kernel.split('.').slice(0, 2).join('.')}`;
const systemdRelease = `v${channel.systemd.split('.')[0]}`;

// Release order, so "first in v6.15" can be compared with "Omarchy builds v7.2"
const kernelRank = (release: string | null) => (release ? sources.kernelReleases.indexOf(release) : Infinity);
const systemdRank = (release: string | null) => (release ? sources.systemdReleases.indexOf(release) : Infinity);

type DeviceTree = { file: string; firstRelease: string | null; inNext: boolean; model?: string; compatible: string[] };
const deviceTrees: DeviceTree[] = sources.deviceTrees;
const omarchyAdded: DeviceTree[] = sources.omarchy.kernelAddedDeviceTrees.map((dt: DeviceTree) => ({ ...dt, firstRelease: null, inNext: false }));

function findDeviceTree(compatible: string | undefined) {
  if (!compatible) return undefined;
  const upstream = deviceTrees.find((dt) => dt.compatible[0] === compatible) ?? deviceTrees.find((dt) => dt.compatible.includes(compatible));
  const added = omarchyAdded.find((dt) => dt.compatible.includes(compatible));
  if (!upstream && !added) return undefined;
  const dt = upstream ?? added!;
  const inOmarchy = Boolean(added) || kernelRank(dt.firstRelease) <= kernelRank(kernelBase);
  return { file: dt.file, compatible: dt.compatible, firstRelease: upstream?.firstRelease ?? null, inNext: upstream?.inNext ?? false, inOmarchy };
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

  // Omarchy boots a model when its kernel has the device tree and systemd's list picks it
  const omarchy = !dt?.inOmarchy ? 'missing' : idsInOmarchy ? 'works' : 'manual';
  return {
    ...variant,
    omarchy,
    deviceTree: dt ? { file: dt.file, firstRelease: dt.firstRelease, inNext: dt.inNext, inOmarchy: dt.inOmarchy } : null,
    ids: ids.map((entry) => ({ file: entry.file, firstRelease: entry.firstRelease, inMain: entry.inMain, matches: compatibles.includes(entry.compatible), compatible: entry.compatible })),
  };
}

const omarchyOrder = ['works', 'manual', 'missing'];
const best = <T extends string>(values: T[], order: T[]) => values.sort((a, b) => order.indexOf(a) - order.indexOf(b))[0];

const firmwareFolders: { path: string; files: string[] }[] = sources.firmware;
const acpiDumps: { folder: string; url: string }[] = sources.acpiDumps;

const rows = models.map((model) => {
  const variants = (model.variants?.length ? model.variants : [{}]).map(variantStatus);
  const firmware = (model.firmware ?? []).map((path) => firmwareFolders.find((folder) => folder.path === path)).filter(Boolean) as typeof firmwareFolders;
  // The signed DSP images are what a laptop can't do without; many folders only carry the open audio topology
  const signed = firmware.some((folder) => folder.files.some((file) => /^qcadsp\d+\.mbn$/.test(file)));
  firmware.forEach((folder) => used.firmware.add(folder.path));
  const acpi = (model.acpi ?? []).map((folder) => acpiDumps.find((dump) => dump.folder === folder)).filter(Boolean) as typeof acpiDumps;
  acpi.forEach((dump) => used.acpi.add(dump.folder));

  const dts = variants.map((v) => v.deviceTree).filter(Boolean) as NonNullable<(typeof variants)[number]['deviceTree']>[];
  const deviceTree = dts.length === 0 ? (model.devicetree_posted ? 'posted' : 'missing') : dts.some((dt) => dt.firstRelease) ? 'released' : 'next';
  const firstRelease = dts.map((dt) => dt.firstRelease).filter(Boolean).sort((a, b) => kernelRank(a) - kernelRank(b))[0] ?? null;
  const idsState = variants.some((v) => v.ids.some((entry) => entry.matches)) ? 'listed' : variants.some((v) => v.ids.length) ? 'mismatch' : 'missing';

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
    omarchy: best(variants.map((v) => v.omarchy), omarchyOrder),
    upstream: {
      deviceTree,
      firstRelease,
      devicetreePosted: model.devicetree_posted ?? null,
      ids: idsState,
      // What counts is getting the firmware without the laptop's own Windows
      firmware: signed ? 'linux-firmware' : model.firmware_download ? 'download' : 'windows',
      firmwareAudioOnly: !signed && firmware.length > 0,
      firmwareDownload: model.firmware_download ?? null,
      firmwareFolders: firmware.map((folder) => folder.path),
      acpi: acpi.length ? 'collected' : 'missing',
      acpiUrls: acpi.map((dump) => dump.url),
    },
    variants,
  };
});

const count = <T>(items: T[], test: (item: T) => boolean) => items.filter(test).length;
const current = rows.filter((row) => row.generation !== '8cx');
const summary = {
  models: current.length,
  nvidia: nvidia.length,
  nvidiaWorks: count(nvidia, (m) => m.omarchy === 'works'),
  nvidiaExpected: count(nvidia, (m) => m.omarchy === 'expected'),
  omarchyWorks: count(current, (r) => r.omarchy === 'works'),
  omarchyManual: count(current, (r) => r.omarchy === 'manual'),
  deviceTree: count(current, (r) => r.upstream.deviceTree === 'released' || r.upstream.deviceTree === 'next'),
  ids: count(current, (r) => r.upstream.ids === 'listed'),
  firmware: count(current, (r) => r.upstream.firmware === 'linux-firmware' || r.upstream.firmware === 'download'),
  acpi: count(current, (r) => r.upstream.acpi === 'collected'),
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
  JSON.stringify({ collected: sources.collected, linuxNext: sources.linuxNext, omarchy: { channel: OMARCHY_CHANNEL, kernel: channel.kernel, systemd: channel.systemd, kernelBase, systemdRelease, channels: sources.omarchy.channels }, summary, models: rows, nvidia, unmatched }, null, 2) + '\n',
);
console.log(summary);
console.log(`unmatched: ${unmatched.deviceTrees.length} device trees, ${unmatched.ids.length} ID entries, ${unmatched.firmware.length} firmware folders, ${unmatched.acpi.length} ACPI dumps`);
