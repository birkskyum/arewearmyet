// Keeps a copy of what the board links that no durable archive holds yet, next to device-trees/:
// - hwids/<model id>/: every hardware ID file that systemd's list doesn't have, from Omarchy's kernel
//   package or from Ubuntu's stubble (merged or in a pull request), in stubble's format, with a SOURCE.md.
// - reports/<maker>/<model>/<report>/: each hardware report in turbineBMW/omarchy-hardware-reports, minus its
//   device list, driver files and registry export, which stay there. The SoC serial number (SOSN) is zeroed
//   in its ACPI tables.
// It reads data/sources.json, so it runs after scripts/collect.ts.
import { execFileSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { parse } from 'yaml';

type Model = { id: string; vendor: string; name: string; variants?: { compatible?: string; ids?: string[] }[] };
type Ids = { file: string; name: string; compatible: string; hwids?: string[] };

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
const reportDirs = walk(join(checkout, 'reports')).filter((file) => file.endsWith('/report.json')).map((file) => dirname(file)).sort();
for (const report of reportDirs) {
  const from = join(checkout, 'reports', report);
  const to = join('reports', report);
  const files = walk(from).filter((file) => KEEP.test(file)).sort();
  let zeroed = 0;
  for (const file of files) {
    mkdirSync(dirname(join(to, file)), { recursive: true });
    if (file.startsWith('acpi/')) {
      const { data, changed } = zeroSosn(readFileSync(join(from, file)));
      if (changed) zeroed++;
      writeFileSync(join(to, file), data);
    } else {
      cpSync(join(from, file), join(to, file));
    }
  }
  const url = `https://github.com/${REPORTS}/tree/${commit}/reports/${report}`;
  writeFileSync(join(to, 'SOURCE.md'), [
    `# Hardware report ${report}`, '',
    `From ${url}, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC`,
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
writeFileSync('reports/README.md', [
  '# Hardware reports', '',
  `A copy of every hardware report in ${REPORTS}: ACPI tables with the SoC serial number zeroed,`,
  'SMBIOS table, panel EDID, firmware list, hardware IDs and the report\'s own record. The device list, driver files',
  'and registry export stay in that repository. scripts/archive.ts rewrites this folder every day.', '',
].join('\n'));
