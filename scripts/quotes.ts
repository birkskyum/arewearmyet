// Checks that a finding is backed by words that really stand in its source. Every finding that
// was collected from a public page carries a `quote`, the passage of the source it rests on.
// This fetches the source and looks for that passage, so no finding is taken on someone's word,
// a person's or an agent's.
//
//   node scripts/quotes.ts                 checks every finding in data/findings.yaml that has a quote
//   node scripts/quotes.ts verdicts.json   checks a list of {model, check, source, quote, ...} and writes
//                                          verdicts.checked.json beside it, each with `found`
//
// It needs the network and the gh CLI, so it is run by hand, not in CI.

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
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
  } else if (commit) {
    text = JSON.parse(gh(['api', `repos/${commit[1]}/${commit[2]}/commits/${commit[3]}`])).commit.message;
  } else if (blob) {
    text = gh(['api', '-H', 'Accept: application/vnd.github.raw', `repos/${blob[1]}/${blob[2]}/contents/${blob[4]}?ref=${blob[3]}`]);
  } else {
    const response = await fetch(url, { headers: { 'user-agent': 'arewearmyet-quote-check (+https://arewearmyet.com)', accept: 'text/html,text/plain,*/*' }, redirect: 'follow', signal: AbortSignal.timeout(30000) });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    text = (await response.text())
      .replace(/<script[\s\S]*?<\/script>|<style[\s\S]*?<\/style>/gi, ' ')
      .replace(/<[^>]+>/g, ' ')
      .replace(/&nbsp;/g, ' ').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#0?39;|&apos;/g, "'")
      .replace(/&#(\d+);/g, (_, code) => String.fromCodePoint(Number(code)));
    // A page that asks whether the reader is a person says nothing about the hardware
    if (/Making sure you're not a bot|Just a moment\.\.\.|Checking your browser|Enable JavaScript and cookies to continue/i.test(text)) throw new Error('bot check');
  }
  writeFileSync(file, text);
  return text;
}

/** Letters and digits only, so that markup, line breaks and the kind of quote marks don't decide whether a passage is found. */
const plain = (text: string) => text.toLowerCase().normalize('NFKD').replace(/[^\p{L}\p{N}]+/gu, ' ').trim();

export async function found(source: string, quote: string): Promise<'found' | 'missing' | 'too short' | 'unreachable'> {
  const wanted = plain(quote);
  // A couple of words would be found in any page and prove nothing
  if (wanted.length < 12) return 'too short';
  let text: string;
  try {
    text = await read(source);
  } catch {
    return 'unreachable';
  }
  return plain(text).includes(wanted) ? 'found' : 'missing';
}

const input = process.argv[2];
let entries: Entry[];
if (input) {
  entries = JSON.parse(readFileSync(input, 'utf8')) as Entry[];
} else {
  const findings = parse(readFileSync('data/findings.yaml', 'utf8')) as { models: Record<string, { checks?: Record<string, { source?: string; quote?: string }> }> };
  entries = Object.entries(findings.models).flatMap(([model, entry]) => Object.entries(entry.checks ?? {}).filter(([, f]) => f.quote).map(([check, f]) => ({ model, check, source: f.source, quote: f.quote })));
}

const tally: Record<string, number> = {};
const failed: string[] = [];
for (const entry of entries) {
  const result = entry.source && entry.quote ? await found(entry.source, entry.quote) : 'no quote';
  entry.found = result;
  tally[result] = (tally[result] ?? 0) + 1;
  if (result !== 'found' && result !== 'no quote') failed.push(`${result}: ${entry.model} ${entry.check} ${entry.source}\n    "${String(entry.quote).slice(0, 160)}"`);
}
if (input) writeFileSync(input.replace(/\.json$/, '.checked.json'), JSON.stringify(entries, null, 1));
console.log(Object.entries(tally).map(([k, n]) => `${n} ${k}`).join(', '));
if (failed.length) console.log(failed.slice(0, 40).join('\n'));
// Run over the site's own findings, a quote that isn't in its source is an error
if (!input && failed.length) process.exit(1);
