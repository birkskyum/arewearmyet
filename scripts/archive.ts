// Keeps a copy of what the board links that no durable archive holds yet, next to device-trees/:
// - hwids/<model id>/: every hardware ID file that systemd's list doesn't have, from Omarchy's kernel
//   package or from Ubuntu's stubble (merged or in a pull request), in stubble's format, with a SOURCE.md.
// - reports/<maker>/<model>/<report>/: each hardware report in turbineBMW/omarchy-hardware-reports, merged or in an
//   open pull request, minus its device list, driver files and registry export, which stay there. The SoC serial
//   number (SOSN) is zeroed in its ACPI tables.
// - acpi/aarch64-laptops/<folder>/ and acpi/linux-surface/<folder>/: the tables of every ACPI dump the board reads
//   from aarch64-laptops, merged or proposed, and from linux-surface, with the SOSN zeroed and the tables that hold
//   a Windows product key left out, and acpi/README.md, which lists every model's dumps.
// It reads data/sources.json, so it runs after scripts/collect.ts.
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { parse } from 'yaml';

function githubToken(): string | undefined {
  if (process.env.GITHUB_TOKEN) return process.env.GITHUB_TOKEN;
  try {
    return execFileSync('gh', ['auth', 'token'], { encoding: 'utf8' }).trim();
  } catch {
    return undefined;
  }
}
const token = githubToken();

// GitHub answers the odd request with a 5xx, so a server error or a dropped connection is tried again
async function get(url: string, headers: Record<string, string> = {}): Promise<Response> {
  for (let attempt = 1; ; attempt++) {
    let response: Response | undefined;
    try {
      response = await fetch(url, { headers });
    } catch (error) {
      if (attempt === 3) throw error;
    }
    if (response && (response.ok || (response.status < 500 && response.status !== 429) || attempt === 3)) {
      if (!response.ok) throw new Error(`${response.status} ${url}`);
      return response;
    }
    await new Promise((resolve) => setTimeout(resolve, 2000 * attempt));
  }
}
const githubApi = async (path: string) => (await get(`https://api.github.com/${path}`, { Accept: 'application/vnd.github+json', ...(token ? { Authorization: `Bearer ${token}` } : {}) })).json();
const rawFile = async (repo: string, ref: string, path: string) => Buffer.from(await (await get(`https://raw.githubusercontent.com/${repo}/${ref}/${path.split('/').map(encodeURIComponent).join('/')}`)).arrayBuffer());

// Each repository's files at a commit, by path, with their sizes, listed once
const trees = new Map<string, Promise<{ path: string; size: number }[]>>();
function filesAt(repo: string, ref: string) {
  const key = `${repo}@${ref}`;
  if (!trees.has(key)) trees.set(key, githubApi(`repos/${repo}/git/trees/${ref}?recursive=1`).then((tree) => tree.tree.filter((entry: { type: string }) => entry.type === 'blob')));
  return trees.get(key)!;
}

// Runs the jobs a few at a time
async function inBatches<T, R>(items: T[], size: number, job: (item: T) => Promise<R>): Promise<R[]> {
  const results: R[] = [];
  for (let at = 0; at < items.length; at += size) results.push(...(await Promise.all(items.slice(at, at + size).map(job))));
  return results;
}

type Model = { id: string; vendor: string; name: string; variants?: { compatible?: string; ids?: string[] }[] };
type Ids = { file: string; name: string; compatible: string; hwids?: string[] };
type AcpiDump = { folder: string; url: string; repo?: string; ref?: string; dir?: string; pull?: { number: number; state: string; by: string } };

const models: Model[] = parse(readFileSync('data/models.yaml', 'utf8')).models;
const sources = JSON.parse(readFileSync('data/sources.json', 'utf8'));

// Hardware IDs
// A model's own list of ID files comes first, since some entries carry a placeholder compatible, then the
// compatible. Entries for boards the board doesn't track are left out.
const modelFor = new Map<string, Model>();
const modelForFile = new Map<string, Model>();
for (const model of models) {
  for (const variant of model.variants ?? []) {
    if (variant.compatible && !modelFor.has(variant.compatible)) modelFor.set(variant.compatible, model);
    for (const file of variant.ids ?? []) if (!modelForFile.has(file)) modelForFile.set(file, model);
  }
}

// An entry systemd already lists with the same compatible and the same IDs needs no copy
const same = (a: Ids, b: Ids) => a.compatible === b.compatible && [...(a.hwids ?? [])].sort().join() === [...(b.hwids ?? [])].sort().join();
const inSystemd = (entry: Ids) => sources.systemd.some((listed: Ids) => same(listed, entry));

const PKGS_RECIPE = 'https://github.com/omacom/omarchy-pkgs/blob/master/pkgbuilds/linux-omarchy-aarch64';
const candidates: (Ids & { source: string })[] = [
  ...(sources.omarchy.carriedIds ?? []).map((entry: Ids) => ({ ...entry, source: `Omarchy's kernel package, ${PKGS_RECIPE}/${entry.file}` })),
  ...sources.stubble.map((entry: Ids & { state: string; url: string }) => ({ ...entry, source: `Ubuntu's stubble, ${entry.state === 'open' ? 'pull request' : 'merged'}: ${entry.url}` })),
];

type Archived = { model: Model; file: string; entry: Ids; sources: string[] };
const archived = new Map<string, Archived>();
for (const candidate of candidates) {
  if (!candidate.hwids?.length || inSystemd(candidate)) continue;
  const model = modelForFile.get(candidate.file) ?? modelFor.get(candidate.compatible);
  if (!model) continue;
  const folder = model.id;
  // The same file from two places is kept once when it holds the same IDs, and twice when it doesn't
  const existing = archived.get(`${folder}/${candidate.file}`);
  if (existing && same(existing.entry, candidate)) {
    existing.sources.push(candidate.source);
    continue;
  }
  const file = existing ? `stubble-${candidate.file}` : candidate.file;
  archived.set(`${folder}/${file}`, { model, file, entry: candidate, sources: [candidate.source] });
}

rmSync('hwids', { recursive: true, force: true });
const byFolder = new Map<string, Archived[]>();
for (const [key, item] of archived) {
  const folder = key.split('/')[0];
  byFolder.set(folder, [...(byFolder.get(folder) ?? []), item]);
}
for (const [folder, items] of byFolder) {
  mkdirSync(`hwids/${folder}`, { recursive: true });
  for (const { file, entry } of items) {
    writeFileSync(`hwids/${folder}/${file}`, JSON.stringify({ type: 'devicetree', name: entry.name, compatible: entry.compatible, hwids: entry.hwids }, null, 4) + '\n');
  }
  const model = items[0].model;
  writeFileSync(`hwids/${folder}/SOURCE.md`, [
    `# ${model.vendor} ${model.name}`, '',
    'Hardware IDs systemd\'s list doesn\'t have yet, in stubble\'s format.', '',
    ...items.map(({ file, entry, sources: from }) => `- ${file} (${entry.compatible}, ${entry.hwids!.length} IDs): from ${from.join('; ')}`), '',
    'Copied by scripts/archive.ts.', '',
  ].join('\n'));
  console.log(`hwids/${folder}: ${items.map((item) => item.file).join(', ')}`);
}
writeFileSync('hwids/README.md', [
  '# Hardware IDs not in systemd yet', '',
  'A copy of every hardware ID file the board counts that systemd\'s list doesn\'t have: the ones Omarchy\'s kernel',
  'package carries and the ones merged or proposed in Ubuntu\'s stubble, which systemd copies from. scripts/archive.ts',
  'rewrites this folder every day. Each folder\'s SOURCE.md says where its files came from.', '',
].join('\n'));

// Hardware reports
const REPORTS = 'turbineBMW/omarchy-hardware-reports';
const checkout = join(tmpdir(), 'arewearmyet-hardware-reports');
if (existsSync(join(checkout, '.git'))) {
  execFileSync('git', ['-C', checkout, 'fetch', '-q', '--depth', '1', 'origin', 'HEAD'], { stdio: 'inherit' });
  execFileSync('git', ['-C', checkout, 'reset', '-q', '--hard', 'FETCH_HEAD'], { stdio: 'inherit' });
} else {
  execFileSync('git', ['clone', '-q', '--depth', '1', '--filter=blob:none', '--sparse', `https://github.com/${REPORTS}.git`, checkout], { stdio: 'inherit' });
  execFileSync('git', ['-C', checkout, 'sparse-checkout', 'set', '--no-cone', '/reports/', '!/reports/**/drivers/', '!/reports/**/registry/', '!/reports/**/devices.json', '!/reports/**/windows.json'], { stdio: 'inherit' });
}
const commit = execFileSync('git', ['-C', checkout, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();

// Qualcomm's DSDT declares Name (SOSN, ...), which the firmware fills with the chip's serial number. Its value
// is zeroed where it sits and the table's checksum put right, as the reports' own scrub.py does.
const WIDTH: Record<number, number> = { 0x0a: 1, 0x0b: 2, 0x0c: 4, 0x0e: 8 };
function zeroSosn(table: Buffer) {
  const data = Buffer.from(table);
  let changed = false;
  for (let at = data.indexOf('SOSN', 1); at > 0; at = data.indexOf('SOSN', at + 1)) {
    const value = at + 4;
    if (data[at - 1] !== 0x08 || value >= data.length) continue;
    const width = WIDTH[data[value]];
    if (width && value + 1 + width <= data.length) {
      if (data.subarray(value + 1, value + 1 + width).some((byte) => byte !== 0)) changed = true;
      data.fill(0, value + 1, value + 1 + width);
    } else if (data[value] === 0x0d) {
      const end = data.indexOf(0, value + 1);
      if (end > value + 1) {
        changed = true;
        data.fill(0, value + 1, end);
      }
    }
  }
  if (changed && data.length > 9) {
    data[9] = 0;
    data[9] = (256 - (data.reduce((sum, byte) => sum + byte, 0) % 256)) % 256;
  }
  return { data, changed };
}

const KEEP = /^(acpi\/[^/]+\.dat|smbios\/[^/]+|edid\/[^/]+|firmware\.json|hwids\.json|report\.json|summary\.md)$/;
const walk = (dir: string, base = ''): string[] => readdirSync(join(dir, base), { withFileTypes: true }).flatMap((entry) =>
  entry.isDirectory() ? walk(dir, join(base, entry.name)) : [join(base, entry.name)]);

rmSync('reports', { recursive: true, force: true });

// Writes one report's files with the SOSN zeroed in its ACPI tables, and its SOURCE.md
function copyReport(report: string, files: { file: string; data: Buffer }[], from: string) {
  const to = join('reports', report);
  let zeroed = 0;
  for (const { file, data } of files) {
    mkdirSync(dirname(join(to, file)), { recursive: true });
    if (file.startsWith('acpi/')) {
      const scrubbed = zeroSosn(data);
      if (scrubbed.changed) zeroed++;
      writeFileSync(join(to, file), scrubbed.data);
    } else {
      writeFileSync(join(to, file), data);
    }
  }
  writeFileSync(join(to, 'SOURCE.md'), [
    `# Hardware report ${report}`, '',
    `From ${from}, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC`,
    `addresses. Here the SoC serial number (SOSN) is also zeroed in the ACPI tables and each table's checksum corrected${zeroed ? ` (${zeroed} table${zeroed === 1 ? '' : 's'} still had it)` : ''}.`,
    'The device list, driver files and registry export stay in that repository.', '',
    '- `acpi/`: the ACPI tables. Disassemble with `iasl -d`.',
    '- `smbios/dmi.bin`: the SMBIOS table. Read it with `dmidecode --from-dump`.',
    '- `edid/`: the panel\'s EDID.',
    '- `firmware.json`: the firmware files of the driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.',
    '- `hwids.json`, `report.json`, `summary.md`: the report\'s hardware IDs and its own record.', '',
    'Copied by scripts/archive.ts.', '',
  ].join('\n'));
  console.log(`reports/${report}: ${files.length} files${zeroed ? `, SOSN zeroed in ${zeroed}` : ''}`);
}

const reportDirs = walk(join(checkout, 'reports')).filter((file) => file.endsWith('/report.json')).map((file) => dirname(file)).sort();
for (const report of reportDirs) {
  const from = join(checkout, 'reports', report);
  const files = walk(from).filter((file) => KEEP.test(file)).sort().map((file) => ({ file, data: readFileSync(join(from, file)) }));
  copyReport(report, files, `https://github.com/${REPORTS}/tree/${commit}/reports/${report}`);
}

// Reports still in an open pull request, fetched from its head
const acpiDumps: AcpiDump[] = sources.acpiDumps;
for (const dump of acpiDumps.filter((dump) => dump.repo === REPORTS && dump.pull && dump.ref && dump.dir)) {
  const report = dump.dir!.replace(/^reports\//, '');
  const listed = (await filesAt(REPORTS, dump.ref!)).filter((entry) => entry.path.startsWith(`${dump.dir}/`) && KEEP.test(entry.path.slice(dump.dir!.length + 1)));
  const files = await inBatches(listed, 8, async (entry) => ({ file: entry.path.slice(dump.dir!.length + 1), data: await rawFile(REPORTS, dump.ref!, entry.path) }));
  copyReport(report, files.sort((a, b) => a.file.localeCompare(b.file)), `pull request #${dump.pull!.number} (${dump.url}, not merged yet) at commit ${dump.ref}`);
}
writeFileSync('reports/README.md', [
  '# Hardware reports', '',
  `A copy of every hardware report in ${REPORTS}, merged or in an open pull request: ACPI tables with the SoC serial`,
  'number zeroed, SMBIOS table, panel EDID, firmware list, hardware IDs and the report\'s own record. The device list,',
  'driver files and registry export stay in that repository. scripts/archive.ts rewrites this folder every day.', '',
].join('\n'));

// ACPI dumps
// Only the tables, one file each: the binary files that hold a table, under their own names, else acpidump's text
// output, named after each table's signature (ssdt1.dat, ssdt2.dat and so on when there are several), and the
// disassembly of any table with no binary. Left out: MSDM and SLIC, which hold the Windows product key, WPBT, as
// the installer leaves it out, and RGRT, the regulatory logo, an image of several MB. Dumps the board ignores
// aren't copied.
const ARCHIVED_ACPI = { 'aarch64-laptops/build': 'aarch64-laptops', 'linux-surface/acpidumps': 'linux-surface' } as Record<string, string>;
const LEFT_OUT: Record<string, string> = { MSDM: 'the Windows product key', SLIC: 'the Windows licence', WPBT: 'a Windows program to run at boot', RGRT: 'the regulatory logo, an image' };
const ignored = new Set<string>(parse(readFileSync('data/models.yaml', 'utf8')).ignore?.acpi ?? []);
const NOT_TABLES = /\.(dsl|log|md|txt|out|xml|dts|dtb|json|sh|deb|mbn|efi|mbox|fw|mdt|pre|zip)$/i;

// A table from a binary file keeps that file's name, one from a text dump gets none
type Table = { signature: string; data: Buffer; from: string; name?: string };
const signatureOf = (data: Buffer) => {
  const signature = data.subarray(0, 4).toString('latin1');
  return data.length >= 36 && /^[A-Z0-9_]{4}$/i.test(signature) && data.readUInt32LE(4) === data.length ? signature.toUpperCase() : undefined;
};
const stemOf = (path: string) => path.split('/').pop()!.replace(/\.[^.]*$/, '').toLowerCase();

// The tables in acpidump's text output: a "DSDT @ 0x..." line, then lines of offset, hex bytes and text
function parseAcpidump(text: string, from: string): Table[] {
  const tables: Table[] = [];
  let bytes: number[] | undefined;
  const flush = () => {
    if (!bytes?.length) return;
    const data = Buffer.from(bytes);
    const length = data.length >= 8 ? data.readUInt32LE(4) : 0;
    const table = length && length <= data.length ? data.subarray(0, length) : data;
    const signature = signatureOf(table);
    if (signature) tables.push({ signature, data: table, from });
  };
  for (const line of text.split(/\r?\n/)) {
    if (/^\s*[A-Za-z0-9_]{4} @ 0x[0-9A-Fa-f]+/.test(line)) {
      flush();
      bytes = [];
      continue;
    }
    const hex = line.match(/^\s*[0-9A-Fa-f]{4,8}: ((?:[0-9A-Fa-f]{2} ){1,16})/);
    if (hex && bytes) bytes.push(...hex[1].trim().split(' ').map((byte) => parseInt(byte, 16)));
  }
  flush();
  return tables;
}

rmSync('acpi/aarch64-laptops', { recursive: true, force: true });
rmSync('acpi/linux-surface', { recursive: true, force: true });
const copiedAcpi = new Map<string, string>();
for (const dump of acpiDumps.filter((dump) => dump.repo && ARCHIVED_ACPI[dump.repo] && dump.ref && dump.dir && !ignored.has(dump.folder))) {
  const source = ARCHIVED_ACPI[dump.repo!];
  const name = dump.folder.replace(/^linux-surface\//, '');
  const to = join('acpi', source, name);
  const listed = (await filesAt(dump.repo!, dump.ref!)).filter((entry) => entry.path.startsWith(`${dump.dir}/`) && entry.size <= 16e6).sort((a, b) => a.path.localeCompare(b.path, 'en', { numeric: true }));
  const read = (path: string) => rawFile(dump.repo!, dump.ref!, path);
  const relative = (path: string) => path.slice(dump.dir!.length + 1);

  // Binary tables first; files that aren't one, like firmware or a damaged table, are skipped
  let tables: Table[] = (await inBatches(listed.filter((entry) => !NOT_TABLES.test(entry.path)), 8, async (entry) => ({ entry, data: await read(entry.path) })))
    .flatMap(({ entry, data }) => {
      const signature = signatureOf(data);
      return signature ? [{ signature, data, from: relative(entry.path), name: stemOf(entry.path) }] : [];
    });
  if (!tables.length) {
    for (const entry of listed.filter((entry) => /dump[^/]*\.(txt|out)$/i.test(entry.path))) tables.push(...parseAcpidump((await read(entry.path)).toString('latin1'), relative(entry.path)));
  }
  const fromDump = tables.some((table) => !table.name);
  const binaryNames = new Set(tables.map((table) => table.name));
  // The same table can come twice, as a .bin and a .dat or twice in one dump
  tables = tables.filter((table, at) => tables.findIndex((other) => other.data.equals(table.data)) === at);
  const leftOut = tables.filter((table) => LEFT_OUT[table.signature]).map((table) => table.signature);
  tables = tables.filter((table) => !LEFT_OUT[table.signature]);

  mkdirSync(to, { recursive: true });
  const written: string[] = [];
  let zeroed = 0;
  const files = new Set<string>();
  const numbered = new Map<string, number>();
  for (const table of tables) {
    let stem = table.name;
    if (!stem) {
      const n = (numbered.get(table.signature) ?? 0) + 1;
      numbered.set(table.signature, n);
      const several = tables.filter((other) => !other.name && other.signature === table.signature).length > 1;
      stem = `${table.signature.toLowerCase()}${several ? n : ''}`;
    }
    let file = `${stem}.dat`;
    for (let n = 2; files.has(file); n++) file = `${stem}-${n}.dat`;
    files.add(file);
    const scrubbed = zeroSosn(table.data);
    if (scrubbed.changed) zeroed++;
    writeFileSync(join(to, file), scrubbed.data);
    written.push(`${file} (${table.from})`);
  }
  // The disassembly where no binary of the same name is a table: when only the disassembly was shared, or a
  // binary was damaged. A text dump holds every table, so its folder's disassembly isn't needed
  for (const entry of fromDump ? [] : listed.filter((entry) => entry.path.toLowerCase().endsWith('.dsl') && !binaryNames.has(stemOf(entry.path)))) {
    const file = entry.path.split('/').pop()!;
    if (LEFT_OUT[file.slice(0, 4).toUpperCase()]) {
      leftOut.push(file.slice(0, 4).toUpperCase());
      continue;
    }
    const text = (await read(entry.path)).toString('latin1');
    const declared = text.match(/Signature\s*:?\s*"([A-Za-z0-9_]{4})"/)?.[1]?.toUpperCase();
    if (declared && LEFT_OUT[declared]) {
      leftOut.push(declared);
      continue;
    }
    const scrubbed = text.replace(/(Name \(SOSN, )0x[0-9A-Fa-f]+/g, '$10x0000000000000000');
    if (scrubbed !== text) zeroed++;
    writeFileSync(join(to, file), scrubbed, 'latin1');
    written.push(`${file} (${relative(entry.path)}, disassembled)`);
  }

  const origin = dump.pull
    ? `pull request #${dump.pull.number} by ${dump.pull.by} to ${dump.repo}, ${dump.pull.state === 'open' ? 'not merged yet' : 'closed without merging'} (${dump.url}), at commit ${dump.ref}, folder ${dump.dir}`
    : `${dump.repo} at commit ${dump.ref}, folder ${dump.dir} (${dump.url})`;
  writeFileSync(join(to, 'SOURCE.md'), [
    `# ${name}`, '',
    `ACPI tables from ${origin}.`, '',
    ...written.map((line) => `- ${line}`), '',
    ...(written.some((line) => line.includes('.dat ')) ? ['Disassemble with `iasl -d dsdt.dat`.', ''] : []),
    ...(zeroed ? [`The SOSN, the chip's serial number, is zeroed in ${zeroed} table${zeroed === 1 ? '' : 's'}, with each binary table's checksum fixed.`, ''] : []),
    ...(leftOut.length ? [`Left out: ${[...new Set(leftOut)].map((signature) => `${signature} (${LEFT_OUT[signature]})`).join(', ')}.`, ''] : []),
    'Copied by scripts/archive.ts.', '',
  ].join('\n'));
  copiedAcpi.set(dump.folder, `${source}/${name}`);
  console.log(`acpi/${source}/${name}: ${written.length} files${zeroed ? `, SOSN zeroed in ${zeroed}` : ''}${leftOut.length ? `, left out ${[...new Set(leftOut)].join(' ')}` : ''}`);
}

// Every model's ACPI dumps, wherever this repository keeps them
type Listed = Model & { acpi?: string[]; boot?: string };
const where = (folder: string) => {
  if (copiedAcpi.has(folder)) return copiedAcpi.get(folder)!;
  if (folder.startsWith('hardware-reports/')) return `../reports/${folder.slice('hardware-reports/'.length)}`;
  if (folder.startsWith('arewearmyet/')) return folder.slice('arewearmyet/'.length);
  return undefined;
};
const known = new Set(acpiDumps.map((dump) => dump.folder));
// Googlebooks start through Android's bootloader and have no ACPI tables
const rows = (models as Listed[]).filter((model) => model.boot !== 'android').map((model) => ({ model, dumps: (model.acpi ?? []).filter((folder) => known.has(folder)) }));
const claimed = new Set(rows.flatMap((row) => row.dumps));
const unclaimed = [...new Set(acpiDumps.map((dump) => dump.folder))].filter((folder) => !claimed.has(folder) && !ignored.has(folder));
writeFileSync('acpi/README.md', [
  '# ACPI dumps', '',
  'Every ACPI dump the board knows of, by model, with a link to this repository\'s copy. scripts/archive.ts copies the',
  'tables of the dumps in [aarch64-laptops](https://github.com/aarch64-laptops/build), merged or proposed in a pull',
  'request, into `aarch64-laptops/`, those of the Arm Surfaces in [linux-surface](https://github.com/linux-surface/acpidumps)',
  'into `linux-surface/`, and the hardware reports into [`../reports/`](../reports), every day. The other folders here are',
  'dumps published here first. Each folder\'s SOURCE.md says where its tables came from. The SoC serial number (SOSN)',
  'is zeroed, and MSDM and SLIC, which hold the Windows product key, are left out. Disassemble a table with `iasl -d`.', '',
  `${rows.filter((row) => row.dumps.length).length} of ${rows.length} models have at least one dump.`, '',
  '| Model | Dumps |',
  '| --- | --- |',
  ...rows.filter((row) => row.dumps.length).map(({ model, dumps }) => `| ${model.vendor} ${model.name} | ${dumps.map((folder) => (where(folder) ? `[${folder}](${where(folder)})` : folder)).join(', ')} |`),
  '',
  '## No dump yet', '',
  ...rows.filter((row) => !row.dumps.length).map(({ model }) => `- ${model.vendor} ${model.name}`),
  '',
  ...(unclaimed.length ? ['## Not matched to a model yet', '', ...unclaimed.map((folder) => `- ${where(folder) ? `[${folder}](${where(folder)})` : folder}`), ''] : []),
].join('\n'));
