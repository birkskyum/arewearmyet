// Keeps a copy of every device tree the board links that isn't in Linux yet, in device-trees/<model id>/:
// a posted series as its mbox plus the .dts and .dtsi files it adds, or the out-of-tree files a model lists
// in devicetree_files, with a SOURCE.md naming where each came from. device-trees/index.json lists them
// for status.ts. A devicetree_files entry is a file URL, or a patch URL and #path to cut one new file out.
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { parse } from 'yaml';

type Model = { id: string; vendor: string; name: string; devicetree_posted?: string; devicetree_out_of_tree?: string; devicetree_files?: string[] };
type Entry = { state: 'posted' | 'out-of-tree'; source: string; files: string[]; note?: string };

const models: Model[] = parse(readFileSync('data/models.yaml', 'utf8')).models;
const DT_DIR = 'arch/arm64/boot/dts/qcom/';

async function get(url: string) {
  const response = await fetch(url, { headers: { 'User-Agent': 'arewearmyet (+https://arewearmyet.com)' } });
  if (!response.ok) throw new Error(`${url}: HTTP ${response.status}`);
  return response.text();
}

// The device tree files a patch or mbox creates, by name
function newTrees(patch: string) {
  const trees = new Map<string, string>();
  for (const section of patch.split(/^diff --git /m).slice(1)) {
    const path = section.match(/^\+\+\+ b\/(\S+)$/m)?.[1];
    if (!path?.startsWith(DT_DIR) || !/\.dtsi?$/.test(path) || !/^--- \/dev\/null$/m.test(section)) continue;
    const body = section.split(/^@@[^\n]*@@[^\n]*\n/m).slice(1).join('');
    const lines = body.split('\n').filter((line) => line.startsWith('+')).map((line) => line.slice(1));
    trees.set(path.slice(DT_DIR.length), lines.join('\n').replace(/\n*$/, '\n'));
  }
  return trees;
}

async function posted(model: Model, dir: string): Promise<Entry> {
  const url = model.devicetree_posted!;
  const msgid = decodeURIComponent(url.match(/\/patch\/([^/]+)\/?$/)?.[1] ?? '');
  const [patch] = JSON.parse(await get(`https://patchwork.kernel.org/api/1.3/patches/?project=linux-arm-msm&msgid=${encodeURIComponent(msgid)}`));
  if (!patch) throw new Error(`${model.id}: patchwork has no patch ${msgid}`);
  const series = patch.series?.[0];
  const mbox = await get(series?.mbox ?? patch.mbox);
  writeFileSync(`${dir}/series.mbox`, mbox);
  const trees = newTrees(mbox);
  for (const [file, text] of trees) writeFileSync(`${dir}/${file}`, text);
  const files = [...trees.keys()];
  writeFileSync(`${dir}/SOURCE.md`, [
    `# ${model.vendor} ${model.name}`, '',
    `Posted to linux-arm-msm, not in Linux yet: [${series?.name ?? patch.name}](${url})`, '',
    `- Version: ${series?.version ?? 1}, sent ${patch.date.slice(0, 10)} by ${patch.submitter.name}`,
    `- Patchwork state of this patch: ${patch.state}`,
    `- Files: series.mbox (the whole series)${files.length ? ', ' + files.join(', ') : ''}`, '',
    'Copied by scripts/trees.ts. The device tree files keep their own licence lines.', '',
  ].join('\n'));
  return { state: 'posted', source: url, files: ['series.mbox', ...files] };
}

async function outOfTree(model: Model, dir: string): Promise<Entry> {
  const files: string[] = [];
  const patches = new Map<string, Map<string, string>>();
  for (const entry of model.devicetree_files ?? []) {
    const [url, path] = entry.split('#');
    if (path) {
      if (!patches.has(url)) patches.set(url, newTrees(await get(url)));
      const text = patches.get(url)!.get(path);
      if (!text) throw new Error(`${model.id}: ${url} creates no ${path}`);
      writeFileSync(`${dir}/${path}`, text);
      files.push(path);
    } else {
      const file = url.split('/').pop()!;
      writeFileSync(`${dir}/${file}`, await get(url));
      files.push(file);
    }
  }
  const note = files.length ? undefined : 'No public source of the tree found yet, only the link above.';
  writeFileSync(`${dir}/SOURCE.md`, [
    `# ${model.vendor} ${model.name}`, '',
    `Carried outside Linux and not sent there yet: ${model.devicetree_out_of_tree}`, '',
    ...(files.length ? (model.devicetree_files ?? []).map((source, i) => `- ${files[i]}: from ${source}`) : [`- ${note}`]), '',
    'Copied by scripts/trees.ts. The device tree files keep their own licence lines.', '',
  ].join('\n'));
  return { state: 'out-of-tree', source: model.devicetree_out_of_tree!, files: ['SOURCE.md', ...files], note };
}

const index: Record<string, Entry> = {};
rmSync('device-trees', { recursive: true, force: true });
for (const model of models) {
  if (!model.devicetree_posted && !model.devicetree_out_of_tree) continue;
  const dir = `device-trees/${model.id}`;
  mkdirSync(dir, { recursive: true });
  index[model.id] = model.devicetree_posted ? await posted(model, dir) : await outOfTree(model, dir);
  console.log(`${model.id}: ${index[model.id].state}, ${index[model.id].files.join(', ')}`);
}
writeFileSync('device-trees/index.json', JSON.stringify(index, null, 2) + '\n');
writeFileSync('device-trees/README.md', [
  '# Device trees not in Linux yet', '',
  'A copy of every device tree the board links that Linux doesn\'t have yet: the series posted to linux-arm-msm',
  'and the trees other kernels carry. scripts/trees.ts rewrites this folder from data/models.yaml every day.',
  'Each folder\'s SOURCE.md says where its files came from.', '',
].join('\n'));
