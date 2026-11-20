/**
 * Owners' reports on what works on their machine, from hw-test or an agent working
 * with the owner, kept in D1 and shown on the model pages next to the findings kept
 * by hand in data/findings.yaml.
 *
 * Nothing needs an account. A machine sends a random token it keeps (X-Machine-Token)
 * and is stored by that token's SHA-256, so its later runs replace its earlier ones in
 * the counts and only it can add to its own runs. Every result says how it was found:
 * measured by the test, confirmed by the owner, or the agent's own reading. The pages
 * show the three apart, so an agent's guess never reads as a measurement.
 *
 * The routes are listed in worker/routes.ts and described for people at /docs/api.
 */

import index from './models.json' with { type: 'json' };
import { DOCS, ROUTES } from './routes.ts';

type Index = { checks: string[]; kernels?: Record<string, string>; chids: Record<string, number[]>; models: { id: string; vendor: string; name: string; generation: string; boot: string; compatibles: string[]; modelNumbers: string[] }[] };
const INDEX = index as Index;
const CHECKS = new Set(INDEX.checks);
const MODELS = new Map(INDEX.models.map((m) => [m.id, m]));

const STATES = ['works', 'partial', 'broken', 'untested', 'absent'] as const;
const HOW = ['measured', 'owner', 'agent'] as const;
const MODES = ['quick', 'guided', 'agent'] as const;
const HARDWARE_KEYS = ['maker', 'product', 'family', 'sku', 'board', 'bios'] as const;
const PACKAGE_KEYS = ['linux-omarchy', 'linux-omarchy-dtbs', 'linux', 'linux-firmware', 'linux-firmware-qcom', 'mesa', 'systemd', 'omarchy', 'hyprland', 'pipewire', 'alsa-ucm-conf', 'bluez', 'libcamera', 'hw-test', 'linux-omarchy-nvidia-open', 'nvidia-utils', 'libva-nvidia-driver'] as const;
const TOKEN = /^[A-Za-z0-9_-]{32,128}$/;
// A hardware ID with its type, as systemd-analyze chid numbers them, like "3:8073dbed-501f-5f5e-a619-4cdd9c00e865"
const CHID = /^(ext)?\d{1,2}:[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/;
// The types made from the product name, the SKU or the mainboard, which tell models apart. The others
// (maker and family, maker and enclosure, maker alone) are shared by a maker's whole range.
const SPECIFIC_CHIDS = new Set(['0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '13', 'ext0', 'ext2']);
const RUN_ID = /^RUN-[A-Z0-9]{10}$/;
const NOTE_ID = /^NOTE-[A-Z0-9]{10}$/;
const LOG_NAME = /^[a-z0-9][a-z0-9._-]{0,59}$/;
// A note is something that goes wrong, a way around it, a pointer to work elsewhere, or anything else
const NOTE_KINDS = ['problem', 'workaround', 'upstream', 'note'] as const;
const NOTE_BODY = 8000;
const LOG_SIZE = 200 * 1024;
const LOGS_PER_NOTE = 4;
const MODEL_ID = /^[a-z0-9][a-z0-9-]{1,80}$/;
const CODE_ALPHABET = 'ABCDEFGHJKMNPQRSTVWXYZ23456789';
// Links a fix or a note may point to, which is where Linux, its firmware, Mesa and the distros take changes
// Links in answers always point at the public site, also when testing locally
const SITE = 'https://arewearmyet.com';
const LINK_HOSTS = ['github.com', 'gitlab.com', 'lore.kernel.org', 'patchwork.kernel.org', 'git.kernel.org', 'gitlab.freedesktop.org', 'lists.freedesktop.org', 'patchwork.freedesktop.org', 'lists.infradead.org', 'patchew.org', 'bugs.launchpad.net', 'codeberg.org', 'bugzilla.kernel.org', 'gitlab.archlinux.org', 'arewearmyet.com'];

export async function reports(request: Request, env: Env, url: URL): Promise<Response | null> {
  const path = url.pathname.slice('/api/'.length).replace(/\/$/, '');
  const parts = path.split('/');

  if (request.method === 'GET') {
    if (path === '') return json({ name: 'Are we ARM yet?', docs: DOCS, guide: `${SITE}/agents.md`, routes: ROUTES.map(([method, route, does]) => ({ method, path: route, does })) }, 200, 'public, max-age=300');
    if (path === 'checks') return asset(env, url, '/data/checks.json');
    if (path === 'models') return asset(env, url, '/data/models.json');
    if (parts[0] === 'models' && parts.length === 2 && MODEL_ID.test(parts[1])) return model(env, url, parts[1]);
    if (path === 'match') return json(match(url.searchParams));
    if (path === 'owners') return allOwners(env);
    if (parts[0] === 'runs' && parts.length === 2 && RUN_ID.test(parts[1])) return getRun(env, parts[1]);
    if (path === 'review') return review(request, env, url);
    if (path === 'notes') return listNotes(env, url);
    if (path === 'fixes') return fixesFor(env, url);
    if (parts[0] === 'notes' && parts.length === 2 && NOTE_ID.test(parts[1])) return getNote(env, parts[1]);
    if (parts[0] === 'notes' && parts.length === 4 && NOTE_ID.test(parts[1]) && parts[2] === 'logs' && LOG_NAME.test(parts[3])) return getLog(env, parts[1], parts[3]);
    if (parts[0] !== 'firmware') return json({ error: `Not found. The API is described at ${DOCS}` }, 404);
    return null;
  }

  if (request.method !== 'POST') return parts[0] === 'firmware' ? null : json({ error: `Not found. The API is described at ${DOCS}` }, 404);
  if (parts[0] === 'firmware') return null;
  if (path === 'review/hide') return hide(request, env);
  if (env.REPORTS !== 'on') return json({ error: 'Reports are closed for now.' }, 503);
  const ip = request.headers.get('cf-connecting-ip') ?? 'local';
  const { success } = await env.REPORT_LIMIT.limit({ key: ip });
  if (!success) return json({ error: 'Too many reports, try again in a minute.' }, 429);

  if (path === 'runs') return postRun(request, env, url);
  if (parts[0] === 'runs' && parts.length === 3 && RUN_ID.test(parts[1]) && parts[2] === 'results') return postResults(request, env, url, parts[1]);
  if (path === 'fixes') return postFix(request, env);
  if (path === 'notes') return postNote(request, env, null);
  if (parts[0] === 'notes' && parts.length === 3 && NOTE_ID.test(parts[1]) && parts[2] === 'replies') return postNote(request, env, parts[1]);
  if (parts[0] === 'notes' && parts.length === 3 && NOTE_ID.test(parts[1]) && parts[2] === 'status') return postStatus(request, env, parts[1]);
  return json({ error: `Not found. The API is described at ${DOCS}` }, 404);
}

// A static file the site builds, served under /api/ so agents need only one base URL
async function asset(env: Env, url: URL, file: string): Promise<Response> {
  const response = await env.ASSETS.fetch(new URL(file, url));
  if (!response.ok) return json({ error: 'Not found.' }, 404);
  return new Response(response.body, { headers: { 'content-type': 'application/json', 'cache-control': 'public, max-age=300' } });
}

/** The model's static page data plus what owners reported and the fixes they linked. */
async function model(env: Env, url: URL, id: string): Promise<Response> {
  const response = await env.ASSETS.fetch(new URL(`/data/models/${id}.json`, url));
  if (!response.ok) return json({ error: `No model ${id}. GET /api/models lists them, GET /api/match finds a machine's.` }, 404);
  const base = (await response.json()) as Record<string, unknown>;
  const [owners, fixes, notes] = await Promise.all([ownerResults(env, id), modelFixes(env, id), notesWhere(env, 'n.model = ?', [id], 100)]);
  return json({ ...base, owners, fixes, notes }, 200, 'public, max-age=60');
}

type ResultRow = { model?: string; check_id: string; state: string; how: string; note: string | null; evidence: string | null; kernel: string | null; os: string | null; created: string; machine: string; run: string };
type Change = { machines: number; from: string | null; to: string | null; since: string };
type OwnerCheck = {
  machines: number;
  states: Record<string, number>;
  how: Record<string, number>;
  // `os` says which system the result was seen on. hw-test also runs on other distros, and a result from
  // one shows a part can work, not that it does on Omarchy
  latest: { state: string; how: string; note: string | null; evidence: string | null; kernel: string | null; os: string | null; created: string; run: string }[];
  // Machines whose newest result turned from working to not, or the other way, with the kernels on both sides
  regressed?: Change;
  fixed?: Change;
};

const RESULT_ROWS = `SELECT runs.model, r.check_id, r.state, r.how, r.note, r.evidence, runs.kernel, runs.os, runs.created, runs.machine, runs.id AS run
       FROM results r JOIN runs ON runs.id = r.run JOIN machines m ON m.id = runs.machine
      WHERE runs.hidden = 0 AND m.hidden = 0 AND r.state != 'untested'`;
const NEWEST_FIRST = 'ORDER BY runs.created DESC, runs.rowid DESC';

/**
 * Each machine's newest result per check, counted per check. A newer run replaces
 * the same machine's older one, so one owner running the test ten times counts once.
 * The older results still say when something changed. A machine that reported a check
 * working and now reports it broken is a regression, with the kernels on both sides.
 */
function summarize(rows: ResultRow[]) {
  const good = (state: string) => state === 'works';
  const bad = (state: string) => state === 'broken' || state === 'partial';
  // Every machine's results for a check, newest first
  const history = new Map<string, ResultRow[]>();
  for (const row of rows) {
    const key = `${row.machine} ${row.check_id}`;
    const list = history.get(key);
    if (list) list.push(row);
    else history.set(key, [row]);
  }

  const picked: { row: ResultRow; older: ResultRow[] }[] = [];
  for (const list of history.values()) {
    // The newest result counts, with one exception. A later measurement that merely finds a
    // thing working doesn't take back what the owner reported as failing. The test measures
    // "it slept once", and the owner saw the second sleep crash. The owner's result stands until
    // the owner tests that again.
    let at = 0;
    if (list[0].how === 'measured' && good(list[0].state)) {
      const owner = list.findIndex((row) => row.how === 'owner');
      if (owner > 0 && bad(list[owner].state)) at = owner;
    }
    picked.push({ row: list[at], older: list.slice(at + 1) });
  }
  picked.sort((a, b) => (a.row.created < b.row.created ? 1 : a.row.created > b.row.created ? -1 : 0));

  const checks: Record<string, OwnerCheck> = {};
  const machines = new Set<string>();
  for (const { row, older } of picked) {
    machines.add(row.machine);
    const entry = (checks[row.check_id] ??= { machines: 0, states: {}, how: {}, latest: [] });
    entry.machines++;
    entry.states[row.state] = (entry.states[row.state] ?? 0) + 1;
    entry.how[row.how] = (entry.how[row.how] ?? 0) + 1;
    if (entry.latest.length < 5) entry.latest.push({ state: row.state, how: row.how, note: row.note, evidence: row.evidence, kernel: row.kernel, os: row.os, created: row.created, run: row.run });

    // Going back from that result, the first one with another state is where it turned. The
    // change counts once per machine and check, from that result's kernel to the first kernel after it
    let first = row;
    for (const before of older) {
      if (before.state === row.state) {
        first = before;
        continue;
      }
      const kind = bad(row.state) && good(before.state) ? 'regressed' : good(row.state) && bad(before.state) ? 'fixed' : null;
      if (kind) {
        const change = (entry[kind] ??= { machines: 0, from: before.kernel, to: first.kernel, since: first.created });
        change.machines++;
      }
      break;
    }
  }
  return { machines: machines.size, checks };
}

async function ownerResults(env: Env, id: string) {
  const { results } = await env.DB.prepare(`${RESULT_ROWS} AND runs.model = ? ${NEWEST_FIRST} LIMIT 5000`).bind(id).all<ResultRow>();
  return summarize(results);
}

/**
 * Every model's owners' results at once, and the machines that matched no model, for the
 * nightly digest (scripts/digest.ts). All of it is what the model pages already show.
 */
async function allOwners(env: Env): Promise<Response> {
  const [{ results }, unmatched] = await Promise.all([
    env.DB.prepare(`${RESULT_ROWS} AND runs.model IS NOT NULL ${NEWEST_FIRST} LIMIT 50000`).all<ResultRow>(),
    env.DB.prepare(`SELECT runs.hardware, runs.compatible, MAX(runs.chids) AS chids, COUNT(DISTINCT runs.machine) AS machines, COUNT(*) AS runs, MAX(runs.created) AS latest
                      FROM runs JOIN machines m ON m.id = runs.machine
                     WHERE runs.model IS NULL AND runs.hidden = 0 AND m.hidden = 0
                     GROUP BY runs.hardware, runs.compatible ORDER BY latest DESC LIMIT 200`).all<{ hardware: string | null; compatible: string | null; chids: string | null; machines: number; runs: number; latest: string }>(),
  ]);
  const byModel = new Map<string, ResultRow[]>();
  for (const row of results) {
    const rows = byModel.get(row.model!);
    if (rows) rows.push(row);
    else byModel.set(row.model!, [row]);
  }
  return json({
    models: Object.fromEntries([...byModel].map(([id, rows]) => [id, summarize(rows)])),
    unmatched: unmatched.results.map((row) => ({ ...row, hardware: JSON.parse(row.hardware ?? '{}'), chids: JSON.parse(row.chids ?? '[]') })),
  }, 200, 'public, max-age=300');
}

type Side = { state: string; how: string | null; evidence: string | null; note: string | null };
type FixChange = { check: string; from: Side; to: Side };
type FixRow = { id: string; model: string; check: string | null; url: string; note: string | null; created: string; changes: string | null; versions: string | null };

function fixOut(row: FixRow) {
  return { id: row.id, model: row.model, check: row.check, url: row.url, note: row.note, created: row.created, changes: row.changes ? (JSON.parse(row.changes) as FixChange[]) : [], versions: row.versions ? JSON.parse(row.versions) : undefined };
}

async function modelFixes(env: Env, id: string) {
  const { results } = await env.DB.prepare('SELECT id, model, check_id AS "check", url, note, created, changes, versions FROM fixes WHERE model = ? AND hidden = 0 ORDER BY created DESC LIMIT 100').bind(id).all<FixRow>();
  return results.map(fixOut);
}

// Fine to failing, so a change towards the front is an improvement
const RANK: Record<string, number> = { works: 0, partial: 1, broken: 2 };

/** What one pull request changed, over every machine that measured before and after it. */
async function fixesFor(env: Env, url: URL): Promise<Response> {
  const link = allowedLink(url.searchParams.get('url') ?? '');
  if (!link) return json({ error: 'Give the pull request as ?url=, an https link to where the change is reviewed.' }, 400);
  const { results } = await env.DB.prepare(
    `SELECT f.id, f.model, f.check_id AS "check", f.url, f.note, f.created, f.changes, f.versions, f.machine
       FROM fixes f LEFT JOIN machines m ON m.id = f.machine WHERE f.url = ? AND f.hidden = 0 AND COALESCE(m.hidden, 0) = 0 ORDER BY f.created DESC LIMIT 200`,
  ).bind(link).all<FixRow & { machine: string | null }>();
  const checks: Record<string, { improved: number; unchanged: number; worse: number }> = {};
  const machines = new Set<string>();
  for (const row of results) {
    const changes = row.changes ? (JSON.parse(row.changes) as FixChange[]) : [];
    if (changes.length && row.machine) machines.add(row.machine);
    for (const change of changes) {
      const from = RANK[change.from.state];
      const to = RANK[change.to.state];
      if (from === undefined || to === undefined) continue;
      const tally = (checks[change.check] ??= { improved: 0, unchanged: 0, worse: 0 });
      if (to < from) tally.improved++;
      else if (to > from) tally.worse++;
      else tally.unchanged++;
    }
  }
  const improved = Object.entries(checks).filter(([, t]) => t.improved > 0 && t.worse === 0).map(([check]) => check);
  const worse = Object.entries(checks).filter(([, t]) => t.worse > 0).map(([check]) => check);
  return json({ url: link, machines: machines.size, improved, worse, checks, fixes: results.map(fixOut) }, 200, 'public, max-age=30');
}

function parseSide(value: unknown): Side | null {
  const { state, how, evidence, note } = (value ?? {}) as Record<string, unknown>;
  if (!STATES.includes(state as never)) return null;
  return { state: state as string, how: HOW.includes(how as never) ? (how as string) : null, evidence: typeof evidence === 'string' ? clean(evidence, 400) : null, note: typeof note === 'string' ? clean(note, 600) : null };
}

/**
 * Which model a machine is. Its hardware IDs name it the way a boot image picks its device tree,
 * also when it booted with another model's tree. Without them, a booted device tree's compatible
 * string does, and then a model number from the firmware's product, SKU or family strings, like
 * 83ED for the Yoga Slim 7x or 14-fe0 for HP's OmniBook X 14-fe0xxx.
 */
function match(params: URLSearchParams) {
  // "family" is the line a pull request for the machine opens with. Every model the board matches has a
  // Snapdragon chip. Chromebooks and Googlebooks, which start through Google's own boot software, are
  // arm-dragon-google, and the ones that came with Windows arm-dragon-msft. NVIDIA machines, which start
  // through ACPI, are arm-nvidia. Apple machines (arm-apple) are not on the board, so the test names them itself
  const found = (model: Index['models'][number], by: string) => ({
    model: model.id, by, vendor: model.vendor, name: model.name, generation: model.generation, boot: model.boot,
    family: model.boot === 'acpi' ? 'arm-nvidia' : model.boot === 'depthcharge' || model.boot === 'android' ? 'arm-dragon-google' : 'arm-dragon-msft',
    page: `${SITE}/models/${model.id}`,
    // The linux-omarchy version on each of Omarchy's channels as of the site's last build, to compare the machine's with
    kernels: INDEX.kernels ?? {},
  });

  const byIds = matchChids(params.getAll('chid').flatMap((value) => value.split(',')));
  if (byIds) return found(byIds, 'hardware IDs');

  const compatible = (params.get('compatible') ?? '').split(/[\s\0]+/).filter(Boolean);
  for (const c of compatible) {
    const model = INDEX.models.find((m) => m.compatibles.includes(c));
    if (model) return found(model, 'compatible');
  }
  const text = ['product', 'sku', 'family'].map((k) => (params.get(k) ?? '').toUpperCase()).join(' ');
  const vendor = (params.get('vendor') ?? '').toLowerCase();
  let best: { id: string; length: number } | null = null;
  for (const m of INDEX.models) {
    if (vendor && !vendor.includes(m.vendor.toLowerCase().split(' ')[0]) && !m.vendor.toLowerCase().includes(vendor.split(' ')[0])) continue;
    for (const number of m.modelNumbers) {
      const prefix = number.toUpperCase().replace(/X{2,}$/, '');
      if (prefix.length >= 4 && text.includes(prefix) && (!best || prefix.length > best.length)) best = { id: m.id, length: prefix.length };
    }
  }
  if (best) return found(MODELS.get(best.id)!, 'model number');
  return { model: null, by: null, hint: 'Not on the board yet. Add it with a hardware report: https://arewearmyet.com/docs/help' };
}

/**
 * The model a machine's hardware IDs belong to, given most specific first as systemd-analyze chid
 * lists them. The first one the board knows decides when only one model has it. When models share
 * it, the model that has the most of the machine's IDs does, the more specific counting more.
 */
function matchChids(chids: string[]): Index['models'][number] | null {
  const tally = new Map<number, number>();
  let rank = 0;
  for (const item of chids) {
    const [type, chid] = item.trim().toLowerCase().split(':');
    if (!SPECIFIC_CHIDS.has(type) || !chid) continue;
    rank++;
    const owners = INDEX.chids[chid];
    if (!owners) continue;
    if (owners.length === 1 && tally.size === 0) return INDEX.models[owners[0]];
    for (const index of owners) tally.set(index, (tally.get(index) ?? 0) + Math.max(1, 20 - rank));
  }
  const ranked = [...tally].sort((a, b) => b[1] - a[1]);
  if (!ranked.length || (ranked.length > 1 && ranked[0][1] === ranked[1][1])) return null;
  return INDEX.models[ranked[0][0]];
}

type RunBody = {
  mode?: unknown; model?: unknown; compatible?: unknown; kernel?: unknown; os?: unknown; agent?: unknown;
  hardware?: unknown; packages?: unknown; results?: unknown; chids?: unknown;
};

/** Stores a run and its results. The machine is created on its first run. */
async function postRun(request: Request, env: Env, url: URL): Promise<Response> {
  const token = request.headers.get('x-machine-token') ?? '';
  if (!TOKEN.test(token)) return json({ error: 'Send X-Machine-Token, a random string of 32 to 128 letters, digits, - or _, kept on the machine (hw-test keeps one in ~/.local/state/arewearmyet/token).' }, 400);
  const body = (await readJson(request, 256 * 1024)) as RunBody | null;
  if (!body || typeof body !== 'object') return json({ error: 'Send the run as JSON. See https://arewearmyet.com/agents.md' }, 400);

  const mode = MODES.includes(body.mode as never) ? (body.mode as string) : null;
  if (!mode) return json({ error: `mode must be one of ${MODES.join(', ')}.` }, 400);
  const hardware = pick(body.hardware, HARDWARE_KEYS, 120);
  const compatible = typeof body.compatible === 'string' ? clean(body.compatible, 120) : null;
  const chids = Array.isArray(body.chids) ? body.chids.filter((c): c is string => typeof c === 'string' && CHID.test(c.toLowerCase())).slice(0, 40).map((c) => c.toLowerCase()) : [];

  // The model named in the body wins when it exists. Otherwise the machine's own strings decide
  let modelId = typeof body.model === 'string' && MODELS.has(body.model) ? body.model : null;
  if (!modelId) {
    const params = new URLSearchParams();
    for (const chid of chids) params.append('chid', chid);
    if (compatible) params.set('compatible', compatible);
    for (const k of ['product', 'sku', 'family'] as const) if (hardware[k]) params.set(k, hardware[k]);
    if (hardware.maker) params.set('vendor', hardware.maker);
    modelId = match(params).model;
  }

  const results = parseResults(body.results);
  if (typeof results === 'string') return json({ error: results }, 400);
  if (results.length === 0) return json({ error: 'results must hold at least one result.' }, 400);

  const now = new Date().toISOString();
  const machine = await sha256(token);
  const run = code('RUN-');
  const statements = [
    env.DB.prepare('INSERT INTO machines (id, model, first_seen, last_seen) VALUES (?1, ?2, ?3, ?3) ON CONFLICT(id) DO UPDATE SET last_seen = ?3, model = COALESCE(?2, model)').bind(machine, modelId, now),
    env.DB.prepare('INSERT INTO runs (id, machine, model, compatible, created, mode, kernel, os, packages, hardware, agent, chids) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)').bind(
      run, machine, modelId, compatible, now, mode,
      typeof body.kernel === 'string' ? clean(body.kernel, 80) : null,
      typeof body.os === 'string' ? clean(body.os, 80) : null,
      JSON.stringify(pick(body.packages, PACKAGE_KEYS, 60)),
      JSON.stringify(hardware),
      typeof body.agent === 'string' ? clean(body.agent, 60) : null,
      chids.length ? JSON.stringify(chids) : null,
    ),
    ...results.map((r) => env.DB.prepare('INSERT INTO results (run, check_id, state, how, note, evidence) VALUES (?, ?, ?, ?, ?, ?)').bind(run, r.check, r.state, r.how, r.note, r.evidence)),
  ];
  await env.DB.batch(statements);
  return json({
    run,
    model: modelId,
    stored: results.length,
    url: `${SITE}/api/runs/${run}`,
    page: modelId ? `${SITE}/models/${modelId}` : null,
    note: modelId ? undefined : 'The machine matched no model, so the run is kept but shows on no page yet. Send "model" with an id from GET /api/models if you know it.',
  }, 201);
}

async function postResults(request: Request, env: Env, url: URL, run: string): Promise<Response> {
  const token = request.headers.get('x-machine-token') ?? '';
  if (!TOKEN.test(token)) return json({ error: 'Send the machine token the run was made with, as X-Machine-Token.' }, 400);
  const row = await env.DB.prepare('SELECT machine FROM runs WHERE id = ?').bind(run).first<{ machine: string }>();
  if (!row || row.machine !== (await sha256(token))) return json({ error: 'No such run from this machine.' }, 404);
  const body = (await readJson(request, 128 * 1024)) as { results?: unknown } | null;
  const results = parseResults(body?.results);
  if (typeof results === 'string') return json({ error: results }, 400);
  if (results.length === 0) return json({ error: 'results must hold at least one result.' }, 400);
  await env.DB.batch(results.map((r) => env.DB.prepare(
    'INSERT INTO results (run, check_id, state, how, note, evidence) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(run, check_id) DO UPDATE SET state = excluded.state, how = excluded.how, note = excluded.note, evidence = COALESCE(excluded.evidence, results.evidence)',
  ).bind(run, r.check, r.state, r.how, r.note, r.evidence)));
  return json({ run, stored: results.length, url: `${SITE}/api/runs/${run}` });
}

async function getRun(env: Env, id: string): Promise<Response> {
  const run = await env.DB.prepare('SELECT id, model, compatible, created, mode, kernel, os, packages, hardware, agent, chids, hidden FROM runs WHERE id = ?').bind(id).first<Record<string, string | number | null>>();
  if (!run || run.hidden) return json({ error: 'No such run.' }, 404);
  const { results } = await env.DB.prepare('SELECT check_id AS "check", state, how, note, evidence FROM results WHERE run = ? ORDER BY check_id').bind(id).all();
  return json({ ...run, hidden: undefined, packages: JSON.parse(String(run.packages ?? '{}')), hardware: JSON.parse(String(run.hardware ?? '{}')), chids: JSON.parse(String(run.chids ?? '[]')), results });
}

async function postFix(request: Request, env: Env): Promise<Response> {
  const body = (await readJson(request, 64 * 1024)) as { model?: unknown; check?: unknown; url?: unknown; note?: unknown; changes?: unknown; before?: unknown; after?: unknown } | null;
  if (!body || typeof body.model !== 'string' || !MODELS.has(body.model)) return json({ error: 'model must be an id from GET /api/models.' }, 400);
  if (body.check !== undefined && body.check !== null && (typeof body.check !== 'string' || !CHECKS.has(body.check))) return json({ error: 'check must be an id from GET /api/checks, or left out.' }, 400);
  const link = typeof body.url === 'string' ? allowedLink(body.url) : null;
  if (!link) return json({ error: `url must be an https link to ${LINK_HOSTS.join(', ')}.` }, 400);
  const token = request.headers.get('x-machine-token') ?? '';
  const machine = TOKEN.test(token) ? await sha256(token) : null;

  // What the fix changed, as this machine measured it before and after
  const changes: FixChange[] = [];
  if (body.changes !== undefined) {
    if (!machine) return json({ error: 'Measured changes need X-Machine-Token, so each machine counts once.' }, 400);
    if (!Array.isArray(body.changes) || body.changes.length === 0 || body.changes.length > 40) return json({ error: 'changes must be a list of 1 to 40 {check, from: {state, how, evidence}, to: {state, how, evidence}}.' }, 400);
    for (const item of body.changes) {
      const { check, from, to } = (item ?? {}) as Record<string, unknown>;
      if (typeof check !== 'string' || !CHECKS.has(check) || changes.some((c) => c.check === check)) return json({ error: `In changes, ${String(check)} is no check from GET /api/checks, or is there twice.` }, 400);
      const before = parseSide(from);
      const after = parseSide(to);
      if (!before || !after) return json({ error: `In changes, ${check} needs from and to, each with a state of ${STATES.join(', ')}.` }, 400);
      changes.push({ check, from: before, to: after });
    }
  }
  const side = (value: unknown) => {
    const { kernel, packages } = (value ?? {}) as Record<string, unknown>;
    return { kernel: typeof kernel === 'string' ? clean(kernel, 80) : null, packages: pick(packages, PACKAGE_KEYS, 60) };
  };
  const versions = changes.length ? JSON.stringify({ before: side(body.before), after: side(body.after) }) : null;

  const id = code('FIX-');
  const statements = [
    env.DB.prepare('INSERT INTO fixes (id, model, check_id, url, note, machine, created, changes, versions) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)').bind(
      id, body.model, (body.check as string | undefined) ?? null, link, typeof body.note === 'string' ? clean(body.note, 600) : null,
      machine, new Date().toISOString(), changes.length ? JSON.stringify(changes) : null, versions,
    ),
  ];
  // A machine that measures the same pull request again replaces what it said before
  if (machine && changes.length) statements.unshift(env.DB.prepare('DELETE FROM fixes WHERE url = ? AND machine = ? AND changes IS NOT NULL').bind(link, machine));
  await env.DB.batch(statements);
  return json({ fix: id, url: `${SITE}/api/fixes?url=${encodeURIComponent(link)}`, page: `${SITE}/models/${body.model}` }, 201);
}

type NoteRow = { id: string; parent: string | null; model: string | null; check: string | null; kind: string; title: string | null; body: string; links: string | null; status: string; kernel: string | null; os: string | null; packages: string | null; agent: string | null; created: string; replies?: number; logs?: string | null };

const NOTE_COLUMNS = `n.id, n.parent, n.model, n.check_id AS "check", n.kind, n.title, n.body, n.links, n.status, n.kernel, n.os, n.packages, n.agent, n.created`;

function noteOut(row: NoteRow, full: boolean) {
  return {
    id: row.id,
    model: row.model,
    check: row.check,
    kind: row.kind,
    title: row.title,
    status: row.kind === 'problem' ? row.status : undefined,
    // A list gives the start of each note, and the note itself gives all of it
    body: full || row.body.length <= 600 ? row.body : row.body.slice(0, 600) + '…',
    links: row.links ? (JSON.parse(row.links) as string[]) : [],
    kernel: row.kernel,
    os: row.os,
    packages: row.packages ? JSON.parse(row.packages) : undefined,
    agent: row.agent,
    created: row.created,
    replies: row.replies,
    logs: row.logs ? String(row.logs).split(',').map((name) => ({ name, url: `${SITE}/api/notes/${row.id}/logs/${name}` })) : [],
    url: `${SITE}/api/notes/${row.id}`,
    page: `${SITE}/note?id=${row.id}`,
  };
}

/** Notes that aren't answers, newest first, with how many answers and which logs each has. */
async function notesWhere(env: Env, where: string, binds: unknown[], limit: number) {
  const { results } = await env.DB.prepare(
    `SELECT ${NOTE_COLUMNS},
            (SELECT COUNT(*) FROM notes r WHERE r.parent = n.id AND r.hidden = 0) AS replies,
            (SELECT group_concat(name) FROM note_logs WHERE note = n.id) AS logs
       FROM notes n LEFT JOIN machines m ON m.id = n.machine
      WHERE n.hidden = 0 AND COALESCE(m.hidden, 0) = 0 AND n.parent IS NULL AND ${where}
      ORDER BY n.created DESC, n.rowid DESC LIMIT ?`,
  ).bind(...binds, limit).all<NoteRow>();
  return results.map((row) => noteOut(row, false));
}

async function listNotes(env: Env, url: URL): Promise<Response> {
  const where: string[] = ['1 = 1'];
  const binds: unknown[] = [];
  const model = url.searchParams.get('model');
  const check = url.searchParams.get('check');
  const kind = url.searchParams.get('kind');
  const status = url.searchParams.get('status');
  const words = (url.searchParams.get('q') ?? '').trim().slice(0, 80);
  if (words) {
    // Every word has to be in the title or the text
    for (const word of words.split(/\s+/).slice(0, 6)) {
      where.push("(n.title LIKE ? ESCAPE '\\' OR n.body LIKE ? ESCAPE '\\')");
      const like = `%${word.replace(/[\\%_]/g, (c) => `\\${c}`)}%`;
      binds.push(like, like);
    }
  }
  if (model) {
    if (!MODELS.has(model)) return json({ error: 'model must be an id from GET /api/models.' }, 400);
    where.push('n.model = ?');
    binds.push(model);
  }
  if (check) {
    if (!CHECKS.has(check)) return json({ error: 'check must be an id from GET /api/checks.' }, 400);
    where.push('n.check_id = ?');
    binds.push(check);
  }
  if (kind) {
    if (!NOTE_KINDS.includes(kind as never)) return json({ error: `kind must be one of ${NOTE_KINDS.join(', ')}.` }, 400);
    where.push('n.kind = ?');
    binds.push(kind);
  }
  if (status) {
    if (status !== 'open' && status !== 'solved') return json({ error: 'status must be open or solved.' }, 400);
    where.push("n.kind = 'problem' AND n.status = ?");
    binds.push(status);
  }
  return json({ notes: await notesWhere(env, where.join(' AND '), binds, 100) }, 200, 'public, max-age=30');
}

async function getNote(env: Env, id: string): Promise<Response> {
  const visible = 'n.hidden = 0 AND COALESCE(m.hidden, 0) = 0';
  const logs = '(SELECT group_concat(name) FROM note_logs WHERE note = n.id) AS logs';
  const note = await env.DB.prepare(`SELECT ${NOTE_COLUMNS}, ${logs} FROM notes n LEFT JOIN machines m ON m.id = n.machine WHERE n.id = ? AND ${visible}`).bind(id).first<NoteRow>();
  if (!note) return json({ error: 'No such note.' }, 404);
  const { results } = await env.DB.prepare(`SELECT ${NOTE_COLUMNS}, ${logs} FROM notes n LEFT JOIN machines m ON m.id = n.machine WHERE n.parent = ? AND ${visible} ORDER BY n.created, n.rowid`).bind(id).all<NoteRow>();
  return json({ ...noteOut(note, true), replies: results.map((row) => noteOut(row, true)) }, 200, 'public, max-age=30');
}

async function getLog(env: Env, id: string, name: string): Promise<Response> {
  const row = await env.DB.prepare(
    'SELECT l.text FROM note_logs l JOIN notes n ON n.id = l.note LEFT JOIN machines m ON m.id = n.machine WHERE l.note = ? AND l.name = ? AND n.hidden = 0 AND COALESCE(m.hidden, 0) = 0',
  ).bind(id, name).first<{ text: string }>();
  if (!row) return json({ error: 'No such log.' }, 404);
  return new Response(row.text, { headers: { 'content-type': 'text/plain; charset=utf-8', 'cache-control': 'public, max-age=300', 'access-control-allow-origin': '*', 'x-content-type-options': 'nosniff' } });
}

type NoteBody = { model?: unknown; check?: unknown; kind?: unknown; title?: unknown; body?: unknown; links?: unknown; logs?: unknown; kernel?: unknown; os?: unknown; packages?: unknown; agent?: unknown };

/** Stores a note, or an answer to one when parent is given. Logs are cleaned like every other text, line by line. */
async function postNote(request: Request, env: Env, parent: string | null): Promise<Response> {
  const token = request.headers.get('x-machine-token') ?? '';
  if (!TOKEN.test(token)) return json({ error: 'Send X-Machine-Token, the random string kept on the machine in ~/.local/state/arewearmyet/token.' }, 400);
  const body = (await readJson(request, 1024 * 1024)) as NoteBody | null;
  if (!body || typeof body !== 'object') return json({ error: 'Send the note as JSON of at most 1 MB. See https://arewearmyet.com/agents.md' }, 400);

  const machine = await sha256(token);
  let model: string | null = null;
  let check: string | null = null;
  let kind = 'reply';
  let title: string | null = null;
  if (parent) {
    const row = await env.DB.prepare('SELECT model, check_id, parent FROM notes WHERE id = ? AND hidden = 0').bind(parent).first<{ model: string | null; check_id: string | null; parent: string | null }>();
    if (!row) return json({ error: 'No such note.' }, 404);
    if (row.parent) return json({ error: `That is an answer itself. Answer the note it belongs to: POST /api/notes/${row.parent}/replies` }, 400);
    model = row.model;
    check = row.check_id;
  } else {
    if (typeof body.model === 'string' && MODELS.has(body.model)) model = body.model;
    // A machine that has sent a run is known by its model, so the note may leave it out
    if (!model && body.model === undefined) model = (await env.DB.prepare('SELECT model FROM machines WHERE id = ?').bind(machine).first<{ model: string | null }>())?.model ?? null;
    if (!model) return json({ error: 'model must be an id from GET /api/models (GET /api/match finds a machine\'s).' }, 400);
    if (body.check !== undefined && body.check !== null) {
      if (typeof body.check !== 'string' || !CHECKS.has(body.check)) return json({ error: 'check must be an id from GET /api/checks, or left out when the note is about no single check.' }, 400);
      check = body.check;
    }
    if (!NOTE_KINDS.includes(body.kind as never)) return json({ error: `kind must be one of ${NOTE_KINDS.join(', ')}, for something that goes wrong, a way around it, a pointer to work elsewhere, or anything else.` }, 400);
    kind = body.kind as string;
    title = typeof body.title === 'string' ? clean(body.title.replace(/\s+/g, ' '), 140) : '';
    if (title.length < 8) return json({ error: 'title must say in a line what the note is about, in 8 to 140 characters.' }, 400);
  }
  const text = typeof body.body === 'string' ? cleanLong(body.body, NOTE_BODY) : '';
  if (text.length < 20) return json({ error: `body must say what was seen, in 20 to ${NOTE_BODY} characters. Longer output goes in logs.` }, 400);

  const links: string[] = [];
  if (body.links !== undefined) {
    if (!Array.isArray(body.links) || body.links.length > 8) return json({ error: 'links must be a list of at most 8 links.' }, 400);
    for (const item of body.links) {
      const link = typeof item === 'string' ? allowedLink(item) : null;
      if (!link) return json({ error: `Each link must be an https link to ${LINK_HOSTS.join(', ')}.` }, 400);
      links.push(link);
    }
  }
  const logs: { name: string; text: string }[] = [];
  if (body.logs !== undefined) {
    if (!Array.isArray(body.logs) || body.logs.length > LOGS_PER_NOTE) return json({ error: `logs must be a list of at most ${LOGS_PER_NOTE} {name, text}.` }, 400);
    for (const item of body.logs) {
      const { name, text: log } = (item ?? {}) as Record<string, unknown>;
      if (typeof name !== 'string' || !LOG_NAME.test(name) || logs.some((l) => l.name === name)) return json({ error: 'Each log needs its own name of lowercase letters, digits, dots and dashes, like kernel.log.' }, 400);
      if (typeof log !== 'string' || !log.trim() || log.length > LOG_SIZE) return json({ error: `A log is text of at most ${LOG_SIZE / 1024} KB, and ${name} is not. Send the part that shows the problem.` }, 400);
      logs.push({ name, text: cleanLong(log, LOG_SIZE, true) });
    }
  }

  const id = code('NOTE-');
  const now = new Date().toISOString();
  await env.DB.batch([
    env.DB.prepare('INSERT INTO machines (id, model, first_seen, last_seen) VALUES (?1, ?2, ?3, ?3) ON CONFLICT(id) DO UPDATE SET last_seen = ?3').bind(machine, parent ? null : model, now),
    env.DB.prepare('INSERT INTO notes (id, machine, parent, model, check_id, kind, title, body, links, kernel, os, packages, agent, created) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)').bind(
      id, machine, parent, model, check, kind, title, text, links.length ? JSON.stringify(links) : null,
      typeof body.kernel === 'string' ? clean(body.kernel, 80) : null,
      typeof body.os === 'string' ? clean(body.os, 80) : null,
      JSON.stringify(pick(body.packages, PACKAGE_KEYS, 60)),
      typeof body.agent === 'string' ? clean(body.agent, 60) : null,
      now,
    ),
    ...logs.map((log) => env.DB.prepare('INSERT INTO note_logs (note, name, text) VALUES (?, ?, ?)').bind(id, log.name, log.text)),
  ]);
  const thread = parent ?? id;
  return json({ note: id, of: parent ?? undefined, model, url: `${SITE}/api/notes/${thread}`, page: `${SITE}/note?id=${thread}` }, 201);
}

/** A problem's writer, or the admin, marks it solved or open again. */
async function postStatus(request: Request, env: Env, id: string): Promise<Response> {
  const body = (await readJson(request, 4096)) as { status?: unknown } | null;
  if (body?.status !== 'open' && body?.status !== 'solved') return json({ error: 'Send {"status": "solved"} or {"status": "open"}.' }, 400);
  const row = await env.DB.prepare("SELECT machine FROM notes WHERE id = ? AND hidden = 0 AND kind = 'problem'").bind(id).first<{ machine: string }>();
  const token = request.headers.get('x-machine-token') ?? '';
  const writer = row && TOKEN.test(token) && row.machine === (await sha256(token));
  if (!row || !(writer || admin(request, env))) return json({ error: 'No such problem written from this machine. Answer it instead: POST /api/notes/' + id + '/replies' }, 404);
  await env.DB.prepare('UPDATE notes SET status = ? WHERE id = ?').bind(body.status, id).run();
  return json({ note: id, status: body.status });
}

/** The newest runs and fixes, for the review page. Needs ADMIN_TOKEN. */
async function review(request: Request, env: Env, url: URL): Promise<Response> {
  if (!admin(request, env)) return json({ error: 'Needs the admin token.' }, 401);
  const before = url.searchParams.get('before') ?? '9999';
  const [runs, fixes, notes] = await Promise.all([
    env.DB.prepare(`SELECT runs.id, runs.machine, runs.model, runs.created, runs.mode, runs.kernel, runs.agent, runs.hidden, m.hidden AS machine_hidden,
                           (SELECT json_group_array(json_object('check', check_id, 'state', state, 'how', how, 'note', note)) FROM results WHERE run = runs.id) AS results
                      FROM runs JOIN machines m ON m.id = runs.machine WHERE runs.created < ? ORDER BY runs.created DESC LIMIT 100`).bind(before).all(),
    env.DB.prepare('SELECT id, model, check_id AS "check", url, note, created, hidden FROM fixes ORDER BY created DESC LIMIT 100').all(),
    env.DB.prepare('SELECT id, parent, model, check_id AS "check", kind, title, substr(body, 1, 400) AS body, agent, created, hidden, (SELECT group_concat(name) FROM note_logs WHERE note = notes.id) AS logs FROM notes ORDER BY created DESC LIMIT 100').all(),
  ]);
  return json({ runs: runs.results.map((r) => ({ ...r, results: JSON.parse(String(r.results)) })), fixes: fixes.results, notes: notes.results });
}

async function hide(request: Request, env: Env): Promise<Response> {
  if (!admin(request, env)) return json({ error: 'Needs the admin token.' }, 401);
  const body = (await readJson(request, 4096)) as { kind?: unknown; id?: unknown; hidden?: unknown } | null;
  const table = { run: 'runs', fix: 'fixes', machine: 'machines', note: 'notes' }[String(body?.kind)] as string | undefined;
  if (!table || typeof body?.id !== 'string') return json({ error: 'Send {"kind": "run" | "fix" | "machine" | "note", "id", "hidden": true | false}.' }, 400);
  const { meta } = await env.DB.prepare(`UPDATE ${table} SET hidden = ? WHERE id = ?`).bind(body.hidden === false ? 0 : 1, body.id).run();
  return json({ changed: meta.changes });
}

function admin(request: Request, env: Env): boolean {
  const secret = (env as unknown as { ADMIN_TOKEN?: string }).ADMIN_TOKEN;
  return Boolean(secret) && request.headers.get('authorization') === `Bearer ${secret}`;
}

type Result = { check: string; state: string; how: string; note: string | null; evidence: string | null };

function parseResults(value: unknown): Result[] | string {
  if (!Array.isArray(value) || value.length > 100) return 'results must be a list of at most 100 {check, state, how, note?, evidence?}.';
  const out: Result[] = [];
  const seen = new Set<string>();
  for (const item of value) {
    const { check, state, how, note, evidence } = (item ?? {}) as Record<string, unknown>;
    if (typeof check !== 'string' || !CHECKS.has(check)) return `Unknown check ${String(check)}. GET /api/checks lists them.`;
    if (seen.has(check)) return `${check} appears twice.`;
    seen.add(check);
    if (!STATES.includes(state as never)) return `For ${check}, state must be one of ${STATES.join(', ')}.`;
    if (!HOW.includes(how as never)) return `For ${check}, how must be measured (the test saw it), owner (the owner saw or heard it) or agent (your own reading of logs or output).`;
    out.push({ check, state: state as string, how: how as string, note: typeof note === 'string' ? clean(note, 600) : null, evidence: typeof evidence === 'string' ? clean(evidence, 400) : null });
  }
  return out;
}

function allowedLink(value: string): string | null {
  try {
    const link = new URL(value);
    if (link.protocol !== 'https:' || !LINK_HOSTS.some((host) => link.hostname === host || link.hostname.endsWith(`.${host}`))) return null;
    return link.toString().slice(0, 400);
  } catch {
    return null;
  }
}

function pick<K extends string>(value: unknown, keys: readonly K[], max: number): Partial<Record<K, string>> {
  const out: Partial<Record<K, string>> = {};
  if (!value || typeof value !== 'object') return out;
  for (const k of keys) {
    const v = (value as Record<string, unknown>)[k];
    if (typeof v === 'string' && v.trim()) out[k] = clean(v, max);
  }
  return out;
}

// Short plain text without control characters, addresses or home folders that would identify the owner
function clean(text: string, max: number): string {
  return text
    .replace(/[\u0000-\u0008\u000b-\u001f\u007f]/g, '')
    .replace(/\b([0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b/gi, 'xx:xx:xx:xx:xx:xx')
    .replace(/\/home\/[^/\s]+/g, '/home/user')
    .replace(/\b(serial|serialnumber|s\/n|uuid)[\s:=]+\S+/gi, '$1 [removed]')
    .trim()
    .slice(0, max);
}

// Longer text, a note or a log, is cleaned as above, and also of the network addresses a firewall or a
// driver writes into logs. Only addresses in private ranges, those named as a packet's source or
// destination and those with a port go, so version numbers with four parts stay readable.
// A kernel log also names the disk, the install and the devices around it, by disk, partition and
// file system IDs, a drive's serial number in its by-id name, the machine ID, network names made from a hardware address, and what the owner
// called a Bluetooth device. Those go too.
function cleanLong(text: string, max: number, log = false): string {
  const cleaned = clean(text, text.length)
    .replace(/\b(SRC|DST)=\S+/g, '$1=[removed]')
    .replace(/\b(?:10\.\d{1,3}|192\.168|172\.(?:1[6-9]|2\d|3[01])|100\.(?:6[4-9]|[7-9]\d|1[01]\d|12[0-7]))\.\d{1,3}\.\d{1,3}\b/g, 'x.x.x.x')
    .replace(/\b\d{1,3}(?:\.\d{1,3}){3}(:\d{2,5})\b/g, 'x.x.x.x$1')
    .replace(/\[[0-9a-f:]*:[0-9a-f:]*:[0-9a-f:]*\](:\d{2,5})\b/gi, '[removed]$1')
    .replace(/\bby(-|\\x2d)(uuid|partuuid|id)(\/|-)\S+/g, 'by$1$2$3[removed]')
    .replace(/\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi, 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx')
    .replace(/\b(PARTUUID=)[0-9a-f]{8}-[0-9a-f]{2}\b/gi, '$1xxxxxxxx-xx')
    .replace(/\b[0-9a-f]{32}\b/gi, 'x'.repeat(32))
    .replace(/\b(enx|wlx)[0-9a-f]{12}\b/gi, '$1xxxxxxxxxxxx');
  // Device names are only taken out of logs, since in a note's own text "Lenovo's firmware" is the owner's wording
  const named = log
    ? cleaned
        .replace(/\b(input: ).+?( \(AVRCP\) as | as \S*(?:\/uhid\/|\/bluetooth\/|\/0005:))/g, '$1[name removed]$2')
        .replace(/\b(0005:[0-9a-f]{4}:[0-9a-f]{4}\.[0-9a-f]{4}: [^\[\n]*\[)[^\]\n]*\]/gi, '$1name removed]')
        .replace(/\b[A-Z][\w-]*['’]s\b/g, "Someone's")
    : cleaned;
  return named.trim().slice(0, max);
}

async function sha256(text: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text));
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('');
}

function code(prefix: string): string {
  const bytes = crypto.getRandomValues(new Uint8Array(10));
  return prefix + Array.from(bytes, (b) => CODE_ALPHABET[b % CODE_ALPHABET.length]).join('');
}

async function readJson(request: Request, limit: number): Promise<unknown> {
  const length = Number(request.headers.get('content-length') ?? 0);
  if (length > limit) return null;
  const text = await request.text();
  if (text.length > limit) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function json(value: unknown, status = 200, cache = 'no-store'): Response {
  return Response.json(value, { status, headers: { 'cache-control': cache, 'access-control-allow-origin': '*' } });
}
