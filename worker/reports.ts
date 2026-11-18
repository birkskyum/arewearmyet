/**
 * Owners' reports on what works on their machine, from probe.sh or an agent working
 * with the owner, kept in D1 and shown on the model pages next to the findings kept
 * by hand in data/findings.yaml.
 *
 * Nothing needs an account. A machine sends a random token it keeps (X-Machine-Token)
 * and is stored by that token's SHA-256, so its later runs replace its earlier ones in
 * the counts and only it can add to its own runs. Every result says how it was found:
 * measured by the probe, confirmed by the owner, or the agent's own reading. The pages
 * show the three apart, so an agent's guess never reads as a measurement.
 *
 * Routes, all JSON:
 *   GET  /api/checks                  what each check means and how to check it
 *   GET  /api/models                  every model with its page and API URL
 *   GET  /api/models/<id>             one model: pieces, findings, owners' results, fixes, what to test next
 *   GET  /api/match?chid=&compatible=&product=&sku=&family=&vendor=   which model a machine is
 *   GET  /api/owners                  every model's owners' results, and the machines that matched no model
 *   POST /api/runs                    a probe run or an agent's report; answers {run, model, url}
 *   GET  /api/runs/<id>               one run as stored, for the owner to check
 *   POST /api/runs/<id>/results       more results for a run, from the same machine
 *   POST /api/fixes                   a link to a fix (pull request, patch) for a model
 *   GET  /api/review, POST /api/review/hide   recent reports and hiding them, with ADMIN_TOKEN
 */

import index from './models.json' with { type: 'json' };

type Index = { checks: string[]; chids: Record<string, number[]>; models: { id: string; vendor: string; name: string; generation: string; boot: string; compatibles: string[]; modelNumbers: string[] }[] };
const INDEX = index as Index;
const CHECKS = new Set(INDEX.checks);
const MODELS = new Map(INDEX.models.map((m) => [m.id, m]));

const STATES = ['works', 'partial', 'broken', 'untested', 'absent'] as const;
const HOW = ['measured', 'owner', 'agent'] as const;
const MODES = ['quick', 'guided', 'agent'] as const;
const HARDWARE_KEYS = ['maker', 'product', 'family', 'sku', 'board', 'bios'] as const;
const PACKAGE_KEYS = ['linux-omarchy', 'linux', 'linux-firmware', 'linux-firmware-qcom', 'mesa', 'systemd', 'omarchy', 'hyprland', 'pipewire', 'alsa-ucm-conf', 'bluez', 'libcamera'] as const;
const TOKEN = /^[A-Za-z0-9_-]{32,128}$/;
// A hardware ID with its type, as systemd-analyze chid numbers them: "3:8073dbed-501f-5f5e-a619-4cdd9c00e865"
const CHID = /^(ext)?\d{1,2}:[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/;
// The types made from the product name, the SKU or the mainboard, which tell models apart. The others
// (maker and family, maker and enclosure, maker alone) are shared by a maker's whole range.
const SPECIFIC_CHIDS = new Set(['0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '13', 'ext0', 'ext2']);
const RUN_ID = /^RUN-[A-Z0-9]{10}$/;
const MODEL_ID = /^[a-z0-9][a-z0-9-]{1,80}$/;
const CODE_ALPHABET = 'ABCDEFGHJKMNPQRSTVWXYZ23456789';
// Links a fix or a note may point to: where Linux, its firmware, Mesa and the distros take changes
// Links in answers always point at the public site, also when testing locally
const SITE = 'https://arewearmyet.com';
const LINK_HOSTS = ['github.com', 'gitlab.com', 'lore.kernel.org', 'patchwork.kernel.org', 'git.kernel.org', 'gitlab.freedesktop.org', 'lists.freedesktop.org', 'patchew.org', 'bugs.launchpad.net', 'codeberg.org', 'bugzilla.kernel.org', 'gitlab.archlinux.org', 'arewearmyet.com'];

export async function reports(request: Request, env: Env, url: URL): Promise<Response | null> {
  const path = url.pathname.slice('/api/'.length).replace(/\/$/, '');
  const parts = path.split('/');

  if (request.method === 'GET') {
    if (path === 'checks') return asset(env, url, '/data/checks.json');
    if (path === 'models') return asset(env, url, '/data/models.json');
    if (parts[0] === 'models' && parts.length === 2 && MODEL_ID.test(parts[1])) return model(env, url, parts[1]);
    if (path === 'match') return json(match(url.searchParams));
    if (path === 'owners') return allOwners(env);
    if (parts[0] === 'runs' && parts.length === 2 && RUN_ID.test(parts[1])) return getRun(env, parts[1]);
    if (path === 'review') return review(request, env, url);
    if (parts[0] !== 'firmware') return json({ error: 'Not found. The API is described at https://arewearmyet.com/agents.md' }, 404);
    return null;
  }

  if (request.method !== 'POST') return parts[0] === 'firmware' ? null : json({ error: 'Not found.' }, 404);
  if (parts[0] === 'firmware') return null;
  if (path === 'review/hide') return hide(request, env);
  if (env.REPORTS !== 'on') return json({ error: 'Reports are closed for now.' }, 503);
  const ip = request.headers.get('cf-connecting-ip') ?? 'local';
  const { success } = await env.REPORT_LIMIT.limit({ key: ip });
  if (!success) return json({ error: 'Too many reports, try again in a minute.' }, 429);

  if (path === 'runs') return postRun(request, env, url);
  if (parts[0] === 'runs' && parts.length === 3 && RUN_ID.test(parts[1]) && parts[2] === 'results') return postResults(request, env, url, parts[1]);
  if (path === 'fixes') return postFix(request, env);
  return json({ error: 'Not found.' }, 404);
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
  const [owners, fixes] = await Promise.all([ownerResults(env, id), modelFixes(env, id)]);
  return json({ ...base, owners, fixes }, 200, 'public, max-age=60');
}

type ResultRow = { model?: string; check_id: string; state: string; how: string; note: string | null; evidence: string | null; kernel: string | null; created: string; machine: string; run: string };
type Change = { machines: number; from: string | null; to: string | null; since: string };
type OwnerCheck = {
  machines: number;
  states: Record<string, number>;
  how: Record<string, number>;
  latest: { state: string; how: string; note: string | null; evidence: string | null; kernel: string | null; created: string; run: string }[];
  // Machines whose newest result turned from working to not, or the other way, with the kernels on both sides
  regressed?: Change;
  fixed?: Change;
};

const RESULT_ROWS = `SELECT runs.model, r.check_id, r.state, r.how, r.note, r.evidence, runs.kernel, runs.created, runs.machine, runs.id AS run
       FROM results r JOIN runs ON runs.id = r.run JOIN machines m ON m.id = runs.machine
      WHERE runs.hidden = 0 AND m.hidden = 0 AND r.state != 'untested'`;
const NEWEST_FIRST = 'ORDER BY runs.created DESC, runs.rowid DESC';

/**
 * Each machine's newest result per check, counted per check. A newer run replaces
 * the same machine's older one, so one owner running the probe ten times counts once.
 * The older results still say when something changed: a machine that reported a check
 * working and now reports it broken is a regression, with the kernels on both sides.
 */
function summarize(rows: ResultRow[]) {
  const newest = new Map<string, ResultRow>();
  const changed = new Set<string>();
  // The oldest result so far that still has the machine's newest state
  const turned = new Map<string, ResultRow>();
  const checks: Record<string, OwnerCheck> = {};
  const machines = new Set<string>();
  const good = (state: string) => state === 'works';
  const bad = (state: string) => state === 'broken' || state === 'partial';
  for (const row of rows) {
    const key = `${row.machine} ${row.check_id}`;
    const latest = newest.get(key);
    if (!latest) {
      newest.set(key, row);
      machines.add(row.machine);
      const entry = (checks[row.check_id] ??= { machines: 0, states: {}, how: {}, latest: [] });
      entry.machines++;
      entry.states[row.state] = (entry.states[row.state] ?? 0) + 1;
      entry.how[row.how] = (entry.how[row.how] ?? 0) + 1;
      if (entry.latest.length < 5) entry.latest.push({ state: row.state, how: row.how, note: row.note, evidence: row.evidence, kernel: row.kernel, created: row.created, run: row.run });
      continue;
    }
    // Going back from the newest result, the first one with another state is where it turned:
    // the change counts once per machine and check, from that result's kernel to the first kernel after it
    if (changed.has(key)) continue;
    if (row.state === latest.state) {
      turned.set(key, row);
      continue;
    }
    changed.add(key);
    const kind = bad(latest.state) && good(row.state) ? 'regressed' : good(latest.state) && bad(row.state) ? 'fixed' : null;
    if (!kind) continue;
    const first = turned.get(key) ?? latest;
    const change = (checks[row.check_id][kind] ??= { machines: 0, from: row.kernel, to: first.kernel, since: first.created });
    change.machines++;
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

async function modelFixes(env: Env, id: string) {
  const { results } = await env.DB.prepare('SELECT id, check_id AS "check", url, note, created FROM fixes WHERE model = ? AND hidden = 0 ORDER BY created DESC LIMIT 100').bind(id).all();
  return results;
}

/**
 * Which model a machine is. Its hardware IDs name it the way a boot image picks its device tree,
 * also when it booted with another model's tree. Without them, a booted device tree's compatible
 * string does, and then a model number from the firmware's product, SKU or family strings, like
 * 83ED for the Yoga Slim 7x or 14-fe0 for HP's OmniBook X 14-fe0xxx.
 */
function match(params: URLSearchParams) {
  const found = (model: Index['models'][number], by: string) => ({ model: model.id, by, vendor: model.vendor, name: model.name, generation: model.generation, page: `${SITE}/models/${model.id}` });

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
  return { model: null, by: null, hint: 'Not on the board yet. Add it with a hardware report: https://arewearmyet.com/help' };
}

/**
 * The model a machine's hardware IDs belong to, given most specific first as systemd-analyze chid
 * lists them. The first one the board knows decides when only one model has it; when models share
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

/** Stores a run and its results; the machine is created on its first run. */
async function postRun(request: Request, env: Env, url: URL): Promise<Response> {
  const token = request.headers.get('x-machine-token') ?? '';
  if (!TOKEN.test(token)) return json({ error: 'Send X-Machine-Token: a random string of 32 to 128 letters, digits, - or _, kept on the machine (probe.sh keeps one in ~/.local/state/arewearmyet/token).' }, 400);
  const body = (await readJson(request, 256 * 1024)) as RunBody | null;
  if (!body || typeof body !== 'object') return json({ error: 'Send the run as JSON. See https://arewearmyet.com/agents.md' }, 400);

  const mode = MODES.includes(body.mode as never) ? (body.mode as string) : null;
  if (!mode) return json({ error: `mode must be one of ${MODES.join(', ')}.` }, 400);
  const hardware = pick(body.hardware, HARDWARE_KEYS, 120);
  const compatible = typeof body.compatible === 'string' ? clean(body.compatible, 120) : null;
  const chids = Array.isArray(body.chids) ? body.chids.filter((c): c is string => typeof c === 'string' && CHID.test(c.toLowerCase())).slice(0, 40).map((c) => c.toLowerCase()) : [];

  // The model named in the body wins when it exists; otherwise the machine's own strings decide
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
  const body = (await readJson(request, 16 * 1024)) as { model?: unknown; check?: unknown; url?: unknown; note?: unknown } | null;
  if (!body || typeof body.model !== 'string' || !MODELS.has(body.model)) return json({ error: 'model must be an id from GET /api/models.' }, 400);
  if (body.check !== undefined && body.check !== null && (typeof body.check !== 'string' || !CHECKS.has(body.check))) return json({ error: 'check must be an id from GET /api/checks, or left out.' }, 400);
  const link = typeof body.url === 'string' ? allowedLink(body.url) : null;
  if (!link) return json({ error: `url must be an https link to ${LINK_HOSTS.join(', ')}.` }, 400);
  const token = request.headers.get('x-machine-token') ?? '';
  const id = code('FIX-');
  await env.DB.prepare('INSERT INTO fixes (id, model, check_id, url, note, machine, created) VALUES (?, ?, ?, ?, ?, ?, ?)').bind(
    id, body.model, (body.check as string | undefined) ?? null, link, typeof body.note === 'string' ? clean(body.note, 600) : null,
    TOKEN.test(token) ? await sha256(token) : null, new Date().toISOString(),
  ).run();
  return json({ fix: id }, 201);
}

/** The newest runs and fixes, for the review page; needs ADMIN_TOKEN. */
async function review(request: Request, env: Env, url: URL): Promise<Response> {
  if (!admin(request, env)) return json({ error: 'Needs the admin token.' }, 401);
  const before = url.searchParams.get('before') ?? '9999';
  const [runs, fixes] = await Promise.all([
    env.DB.prepare(`SELECT runs.id, runs.machine, runs.model, runs.created, runs.mode, runs.kernel, runs.agent, runs.hidden, m.hidden AS machine_hidden,
                           (SELECT json_group_array(json_object('check', check_id, 'state', state, 'how', how, 'note', note)) FROM results WHERE run = runs.id) AS results
                      FROM runs JOIN machines m ON m.id = runs.machine WHERE runs.created < ? ORDER BY runs.created DESC LIMIT 100`).bind(before).all(),
    env.DB.prepare('SELECT id, model, check_id AS "check", url, note, created, hidden FROM fixes ORDER BY created DESC LIMIT 100').all(),
  ]);
  return json({ runs: runs.results.map((r) => ({ ...r, results: JSON.parse(String(r.results)) })), fixes: fixes.results });
}

async function hide(request: Request, env: Env): Promise<Response> {
  if (!admin(request, env)) return json({ error: 'Needs the admin token.' }, 401);
  const body = (await readJson(request, 4096)) as { kind?: unknown; id?: unknown; hidden?: unknown } | null;
  const table = { run: 'runs', fix: 'fixes', machine: 'machines' }[String(body?.kind)] as string | undefined;
  if (!table || typeof body?.id !== 'string') return json({ error: 'Send {"kind": "run" | "fix" | "machine", "id", "hidden": true | false}.' }, 400);
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
    if (!STATES.includes(state as never)) return `${check}: state must be one of ${STATES.join(', ')}.`;
    if (!HOW.includes(how as never)) return `${check}: how must be measured (the probe saw it), owner (the owner saw or heard it) or agent (your own reading of logs or output).`;
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
    .replace(/\b(serial|s\/n|uuid)[\s:=]+\S+/gi, '$1 [removed]')
    .trim()
    .slice(0, max);
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
