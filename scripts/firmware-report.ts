// Looks up a firmware report an owner sent with the ARM installer, by the code it gave them.
// The reports are private: they sit in the site's own storage, which only a signed-in wrangler
// can read, so this runs by hand and nothing of a report is written into the repository.
//
//   node scripts/firmware-report.ts               the codes owners have posted, by issue
//   node scripts/firmware-report.ts FW-K7Q2M9XH4P what the report holds, next to what the model's
//                                                 device tree loads and what the maker's download has
//
// A fetched report is kept in ~/.cache/arewearmyet/firmware-reports, outside the repository.
// It needs wrangler signed in to the account, and the gh CLI for the list of codes.

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { parse } from 'yaml';

type File = { folder: string; name: string; size: number; sha256: string; uploaded?: boolean };
type Report = { code: string; received: string; installer?: string; machine: Record<string, string>; files: File[] };
type Model = { id: string; vendor: string; name: string; modelNumbers?: string[]; variants?: { firmwareNeeded?: string[] }[] };
type Folder = { path: string; models?: string[]; files?: { name: string; size: number; sha256: string }[] };

const BUCKET = 'arewearmyet-firmware';
const CACHE = join(homedir(), '.cache', 'arewearmyet', 'firmware-reports');
const CODE = /^FW-[0-9A-Z]{10}$/;
const REPOS = ['turbineBMW/omarchy-hardware-reports', 'birkskyum/arewearmyet'];
// The files a device tree names: the DSP and GPU images, their configuration and the video engine's
const LOADED = /^(qc(adsp|cdsp|dxkmsuc|vss|av1e)\w*\.mbn|(adsp|cdsp)_dtbs\.elf|(adsp|cdsp|battmgr)\w*\.jsn|wpss\.mbn)$/i;

function gh(args: string[]): string {
  return execFileSync('gh', args, { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024, stdio: ['ignore', 'pipe', 'ignore'] });
}

/** Every code an owner posted, with the issue it stands in. */
function posted(): { code: string; where: string; title: string }[] {
  const found = new Map<string, { code: string; where: string; title: string }>();
  for (const repo of REPOS) {
    const issues = JSON.parse(gh(['api', '--paginate', '--slurp', `repos/${repo}/issues?state=all&per_page=100`])).flat() as { number: number; title: string; body: string | null; html_url: string }[];
    const titles = new Map(issues.map((issue) => [issue.html_url, issue.title]));
    const note = (text: string | null, url: string) => {
      for (const code of text?.match(/FW-[0-9A-Z]{10}/g) ?? []) if (!found.has(code)) found.set(code, { code, where: url, title: titles.get(url.replace(/#.*$/, '')) ?? '' });
    };
    for (const issue of issues) note(issue.body, issue.html_url);
    const comments = JSON.parse(gh(['api', '--paginate', '--slurp', `repos/${repo}/issues/comments?per_page=100`])).flat() as { body: string | null; html_url: string }[];
    for (const comment of comments) note(comment.body, comment.html_url);
  }
  return [...found.values()];
}

function fetched(code: string): Report {
  mkdirSync(CACHE, { recursive: true });
  const file = join(CACHE, `${code}.json`);
  if (!existsSync(file)) {
    try {
      execFileSync('pnpm', ['exec', 'wrangler', 'r2', 'object', 'get', `${BUCKET}/reports/${code}.json`, '--remote', '--file', file], { stdio: ['ignore', 'ignore', 'pipe'] });
    } catch {
      throw new Error(`No report ${code} in the store (or wrangler is not signed in).`);
    }
  }
  return JSON.parse(readFileSync(file, 'utf8')) as Report;
}

const short = (hash: string) => hash.slice(0, 12);
const size = (bytes: number) => (bytes >= 1 << 20 ? `${(bytes / (1 << 20)).toFixed(1)} MB` : bytes >= 1024 ? `${Math.round(bytes / 1024)} kB` : `${bytes} B`);

function show(code: string) {
  const report = fetched(code);
  const status = JSON.parse(readFileSync('src/data/status.json', 'utf8')) as { models: Model[] };
  const folders = (parse(readFileSync('data/firmware-sources.yaml', 'utf8')) as { folders: Folder[] }).folders;
  const m = report.machine;

  console.log(`${report.code}, received ${report.received.slice(0, 16).replace('T', ' ')} UTC${report.installer ? `, installer ${report.installer}` : ''}`);
  console.log(`  ${[m.manufacturer, m.product].filter(Boolean).join(' ')}${m.family ? ` (${m.family})` : ''}`);
  console.log(`  board ${m.baseboard ?? '?'}, BIOS ${m.bios ?? '?'}${m.sku ? `, SKU ${m.sku}` : ''}`);
  const missing = report.files.filter((file) => !file.uploaded);
  console.log(`  ${report.files.length} files from ${new Set(report.files.map((file) => file.folder)).size} driver folders, ${missing.length ? `${missing.length} not uploaded` : 'all uploaded'}`);

  // The model, by a model number that stands in what the firmware says of itself
  const said = Object.values(m).join(' ').toLowerCase();
  const models = status.models.filter((model) => (model.modelNumbers ?? []).some((number) => number.length >= 4 && said.includes(number.toLowerCase())));
  console.log(models.length ? `\nModel: ${models.map((model) => `${model.vendor} ${model.name} (${model.id})`).join(', ')}` : '\nModel: none on the board matches its model numbers');

  const byName = new Map<string, File[]>();
  for (const file of report.files) byName.set(file.name.toLowerCase(), [...(byName.get(file.name.toLowerCase()) ?? []), file]);
  const known = new Map<string, string[]>();
  for (const folder of folders) for (const file of folder.files ?? []) known.set(file.sha256, [...(known.get(file.sha256) ?? []), folder.path]);

  const own = folders.filter((folder) => (folder.models ?? []).some((id) => models.some((model) => model.id === id)));
  for (const folder of own) {
    console.log(`\n${folder.path}, as its device tree loads it and the maker's download has it:`);
    for (const file of folder.files ?? []) {
      const sent = byName.get(file.name.toLowerCase()) ?? [];
      const verdict = !sent.length ? 'not in the report' : sent.some((s) => s.sha256 === file.sha256) ? 'same' + (sent.length > 1 ? ` (and ${sent.length - 1} other version on the laptop)` : '') : `DIFFERENT: the report has ${sent.map((s) => short(s.sha256)).join(', ')}`;
      console.log(`  ${file.name.padEnd(24)} ${short(file.sha256)}  ${verdict}`);
    }
  }

  // Without a firmware folder of its own: what a tree would load, and whether another model has the same bytes
  if (!own.length) {
    console.log('\nNo firmware folder for it yet. The files a device tree would load:');
    for (const file of report.files.filter((f) => LOADED.test(f.name.split('/').pop()!)).sort((a, b) => a.name.localeCompare(b.name))) {
      const shared = [...new Set(known.get(file.sha256) ?? [])];
      console.log(`  ${file.name.padEnd(24)} ${short(file.sha256)} ${size(file.size).padStart(8)}  ${file.folder.replace(/\.inf_arm64_.*$/, '')}${shared.length ? `  = ${shared.length > 3 ? `${shared.length} folders, like ${shared.slice(0, 2).join(' and ')}` : shared.join(', ')}` : ''}`);
    }
  }
  console.log(`\nThe whole list: ${join(CACHE, `${code}.json`)}`);
}

const wanted = process.argv[2];
if (!wanted) {
  for (const entry of posted()) console.log(`${entry.code}  ${entry.title.replace(/^Hardware report: /, '')}\n              ${entry.where}`);
} else if (!CODE.test(wanted)) {
  console.error('A code looks like FW-K7Q2M9XH4P.');
  process.exit(2);
} else {
  try {
    show(wanted);
  } catch (error) {
    console.error((error as Error).message);
    process.exit(1);
  }
}
