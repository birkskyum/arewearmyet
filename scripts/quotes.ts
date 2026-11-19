// Checks that a finding is backed by words that really stand in its source. Every finding that
// was collected from a public page carries a `quote`, the passage of the source it rests on.
// This fetches the source and looks for that passage, so no finding is taken on someone's word,
// a person's or an agent's.
//
//   node scripts/quotes.ts                 checks every finding in data/findings.yaml that has a quote
//   node scripts/quotes.ts verdicts.json   checks a list of {model, check, source, quote, ...} and writes
//                                          verdicts.checked.json beside it, each with `found`
//
// A quote of a few letters, like a line of a list ("- GPU") or a row of a table, would be found
// in any page. It only counts when it is a whole line of the source under a line that leads
// its list in ("Supported and tested:"), and that line is kept with it as `under`.
//
// It needs the network and the gh CLI (and pdftotext for a maker's PDF), so it is run by hand, not in CI.

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { parse } from 'yaml';

type Entry = { model: string; check: string; source?: string; quote?: string; [key: string]: unknown };

const CACHE = join(tmpdir(), 'arewearmyet-sources');
mkdirSync(CACHE, { recursive: true });

function gh(args: string[]): string {
  return execFileSync('gh', args, { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024, stdio: ['ignore', 'pipe', 'ignore'] });
}

const AGENT = 'arewearmyet-quote-check (+https://arewearmyet.com)';

/** A text file someone attached to an issue or a comment is part of what they reported. */
async function attachments(text: string): Promise<string[]> {
  const parts: string[] = [];
  for (const link of new Set(text.match(/https:\/\/github\.com\/user-attachments\/files\/\d+\/[^\s)\]"']+\.(?:txt|md|log)/g) ?? [])) {
    try {
      const response = await fetch(link, { headers: { 'user-agent': AGENT }, redirect: 'follow', signal: AbortSignal.timeout(30000) });
      if (response.ok) parts.push((await response.text()).slice(0, 4 << 20));
    } catch {
      // An attachment that is gone leaves the quote unfound, which is the right answer
    }
  }
  return parts;
}

/** Everything a reader of the page sees as text: the post, every comment and reply, a commit's message, a file's contents. */
async function read(url: string): Promise<string> {
  const file = join(CACHE, createHash('sha256').update(url).digest('hex').slice(0, 24));
  if (existsSync(file)) return readFileSync(file, 'utf8');
  let text = '';
  const thread = url.match(/^https:\/\/github\.com\/([^/]+)\/([^/]+)\/(issues|pull|discussions)\/(\d+)/);
  const commit = url.match(/^https:\/\/github\.com\/([^/]+)\/([^/]+)\/commit\/([0-9a-f]{7,40})/);
  const blob = url.match(/^https:\/\/github\.com\/([^/]+)\/([^/]+)\/(?:blob|tree)\/([^/]+)\/(.+?)(?:[#?].*)?$/);
  if (thread && thread[3] === 'discussions') {
    const [, owner, repo, , number] = thread;
    const query = `query{repository(owner:"${owner}",name:"${repo}"){discussion(number:${number}){title body comments(first:100){nodes{body replies(first:100){nodes{body}}}}}}}`;
    const data = JSON.parse(gh(['api', 'graphql', '-f', `query=${query}`])).data.repository.discussion;
    text = [data.title, data.body, ...data.comments.nodes.flatMap((c: { body: string; replies: { nodes: { body: string }[] } }) => [c.body, ...c.replies.nodes.map((r) => r.body)])].join('\n\n');
  } else if (thread) {
    const [, owner, repo, kind, number] = thread;
    const issue = JSON.parse(gh(['api', `repos/${owner}/${repo}/issues/${number}`]));
    const parts = [issue.title, issue.body ?? ''];
    parts.push(gh(['api', '--paginate', `repos/${owner}/${repo}/issues/${number}/comments`, '--jq', '.[].body']));
    if (kind === 'pull') {
      // What reviewers wrote on the lines, and the commits' own messages, are part of what the page says
      for (const path of [`pulls/${number}/comments`, `pulls/${number}/reviews`]) parts.push(gh(['api', '--paginate', `repos/${owner}/${repo}/${path}`, '--jq', '.[].body']));
      parts.push(gh(['api', '--paginate', `repos/${owner}/${repo}/pulls/${number}/commits`, '--jq', '.[].commit.message']));
    }
    text = parts.join('\n\n');
    text += '\n\n' + (await attachments(text)).join('\n\n');
  } else if (commit) {
    text = JSON.parse(gh(['api', `repos/${commit[1]}/${commit[2]}/commits/${commit[3]}`])).commit.message;
  } else if (blob) {
    text = gh(['api', '-H', 'Accept: application/vnd.github.raw', `repos/${blob[1]}/${blob[2]}/contents/${blob[4]}?ref=${blob[3]}`]);
  } else {
    // Patchwork shows a mail in full only as its raw text
    const address = /^https:\/\/patchwork\.kernel\.org\/project\/[^/]+\/(?:patch|cover)\/[^/]+\/?$/.test(url) ? url.replace(/\/?$/, '/mbox/') : url;
    const response = await fetch(address, { headers: { 'user-agent': AGENT, accept: 'text/html,text/plain,*/*' }, redirect: 'follow', signal: AbortSignal.timeout(30000) });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    if ((response.headers.get('content-type') ?? '').includes('pdf') || /\.pdf(?:[?#]|$)/i.test(url)) {
      writeFileSync(file + '.pdf', Buffer.from(await response.arrayBuffer()));
      text = execFileSync('pdftotext', ['-layout', file + '.pdf', '-'], { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
      unlinkSync(file + '.pdf');
    } else {
      text = (await response.text())
        .replace(/<script[\s\S]*?<\/script>|<style[\s\S]*?<\/style>/gi, ' ')
        // The end of a row, a paragraph or a list item ends a line, so a table's row stays one line
        .replace(/<\/(?:tr|p|li|h[1-6]|div|dt|dd|pre)>|<br\s*\/?>/gi, '\n')
        .replace(/<[^>]+>/g, ' ')
        .replace(/&nbsp;/g, ' ').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#0?39;|&apos;/g, "'")
        .replace(/&#(\d+);/g, (_, code) => String.fromCodePoint(Number(code)));
    }
    // A page that asks whether the reader is a person says nothing about the hardware
    if (/Making sure you're not a bot|Just a moment\.\.\.|Checking your browser|Enable JavaScript and cookies to continue/i.test(text)) throw new Error('bot check');
  }
  writeFileSync(file, text);
  return text;
}

/** Letters and digits only, so that markup, line breaks and the kind of quote marks don't decide whether a passage is found. */
const plain = (text: string) => text.toLowerCase().normalize('NFKD').replace(/[^\p{L}\p{N}]+/gu, ' ').trim();

type Found = { found: 'found' | 'missing' | 'too short' | 'unreachable'; under?: string };

/** A line as written, but for its spacing: a ticked box and an empty one, or a check mark and a cross, stay different. */
const exact = (line: string) => line.toLowerCase().replace(/\s+/g, '');

/**
 * Whether the quote stands in the source. about names what the finding is about (the model),
 * for a source that lists several models with the same lines under each.
 */
/** Whether a short passage says something by itself, as "Bluetooth works." does and "Bluetooth" doesn't. */
const SAYS = /\b(works?|working|worked|broken|fine|fails?|failed|ok|okay|supported|unsupported|partial|partly|not|no|never|missing|ran|runs?|comes up)\b|[✓✅✗❌]|\[[ xX]\]/i;

/**
 * Whether the quote stands in the source. about names what the finding is about (the model),
 * for a source that lists several models with the same lines under each.
 */
export async function found(source: string, quote: string, about = ''): Promise<Found> {
  const wanted = plain(quote);
  if (wanted.length < 3) return { found: 'too short' };
  let text: string;
  try {
    text = await read(source);
  } catch {
    return { found: 'unreachable' };
  }
  // A line of a list or a row of a table: the line that leads the list in says what the list
  // is ("Working:", or a heading)
  const lines = text.split('\n');
  const leads: string[] = [];
  if (wanted.length < 60 && !quote.trim().includes('\n')) {
    lines.forEach((line, at) => {
      if (exact(line) !== exact(quote)) return;
      // The nearest line above that leads a list in. A long paragraph that happens to end in a colon isn't one
      for (let back = at - 1; back >= Math.max(0, at - 60); back--) {
        const lead = lines[back].trim();
        if (lead.length <= 160 && (/:\s*$/.test(lead) || /^#{1,6}\s/.test(lead) || /^\*\*.+\*\*$/.test(lead))) {
          leads.push(lead.replace(/^[#>*\s]+|[*\s]+$/g, ''));
          return;
        }
      }
    });
  }
  // Under several lead-ins, the one that names what the finding is about
  const words = plain(about).split(' ').filter((word) => word.length > 2);
  const score = (lead: string) => words.filter((word) => plain(lead).split(' ').includes(word)).length;
  const under = leads.length ? leads.reduce((best, lead) => (score(lead) > score(best) ? lead : best)) : undefined;
  const item = /^\s*(?:[-*|]|\d+\.)\s/.test(quote);
  // A long passage, or a short one that says something itself, counts wherever it stands
  if (wanted.length >= 26 || (wanted.length >= 12 && SAYS.test(quote))) {
    return plain(text).includes(wanted) ? { found: 'found', ...(under && item ? { under } : {}) } : { found: 'missing' };
  }
  // A few words that say nothing by themselves would be found in any page. They count as
  // an item under a line that leads its list in.
  return under ? { found: 'found', under } : { found: 'too short' };
}

const input = process.argv[2];
let entries: Entry[];
if (input) {
  entries = JSON.parse(readFileSync(input, 'utf8')) as Entry[];
} else {
  type Kept = { checks?: Record<string, { source?: string; quote?: string; basis?: string }> };
  const findings = parse(readFileSync('data/findings.yaml', 'utf8')) as { families?: Record<string, Kept>; models: Record<string, Kept> };
  entries = [...Object.entries(findings.families ?? {}), ...Object.entries(findings.models)].flatMap(([model, entry]) =>
    Object.entries(entry.checks ?? {}).filter(([, f]) => f.quote).map(([check, f]) => ({ model, check, source: f.source, quote: f.quote, basis: f.basis })));
}

const tally: Record<string, number> = {};
const failed: string[] = [];
for (const entry of entries) {
  const answer: { found: string; under?: string } = entry.source && entry.quote ? await found(entry.source, entry.quote, `${entry.model} ${entry.name ?? ''}`) : { found: 'no quote' };
  const result = answer.found;
  entry.found = result;
  if (answer.under) entry.under = answer.under;
  tally[result] = (tally[result] ?? 0) + 1;
  if (result !== 'found' && result !== 'no quote') failed.push(`${result}: ${entry.model} ${entry.check} ${entry.source}\n    "${String(entry.quote).slice(0, 160)}"`);
}
if (input) writeFileSync(input.replace(/\.json$/, '.checked.json'), JSON.stringify(entries, null, 1));
console.log(Object.entries(tally).map(([k, n]) => `${n} ${k}`).join(', '));
if (failed.length) console.log(failed.slice(0, 40).join('\n'));
// Run over the site's own findings, a quote that isn't in its source is an error. A maker's
// spec page drawn by a script can't be read here, and says only what the model has or lacks.
if (!input && entries.some((entry) => entry.found !== 'found' && entry.basis !== 'maker-spec')) process.exit(1);
