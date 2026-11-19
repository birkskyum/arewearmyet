// Holds the API reference (src/lib/api.md, shown at /docs/api) to the routes the worker serves:
// every route in worker/routes.ts has a section, every section names a route, and every route
// that worker/reports.ts answers is in the list.

import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reports } from '../worker/reports.ts';
import { ROUTES } from '../worker/routes.ts';

const reference = readFileSync('src/lib/api.md', 'utf8');
const sections = [...reference.matchAll(/^### `(GET|POST|PUT) (\/api[^`]*)`$/gm)].map((m) => `${m[1]} ${m[2]}`);
const listed = ROUTES.map(([method, path]) => `${method} ${path}`);

test('the reference has a section for every route, in the same order', () => {
  assert.deepEqual(sections, listed);
});

test('the agent guide names no route the list lacks', () => {
  const guide = readFileSync('src/lib/agents.md', 'utf8');
  const named = [...guide.matchAll(/`(GET|POST|PUT) (\/api[A-Za-z/<>-]*)/g)].map((m) => `${m[1]} ${m[2].replace('<NOTE-ID>', '<id>')}`);
  assert.ok(named.length > 10);
  for (const route of named) assert.ok(listed.includes(route), `${route} is in the guide and not in worker/routes.ts`);
});

test('every listed route outside the firmware drop is answered by the worker', async () => {
  // Enough of an environment for a route to refuse the request by itself: a route that isn't
  // served is answered with where the API is described, before it touches any of this
  const none = { first: async () => null, all: async () => ({ results: [] }), run: async () => ({ meta: { changes: 0 } }) };
  const env = {
    REPORTS: 'on',
    ADMIN_TOKEN: 'admin',
    REPORT_LIMIT: { limit: async () => ({ success: true }) },
    ASSETS: { fetch: async () => Response.json({}) },
    DB: { prepare: () => ({ ...none, bind: () => none }), batch: async () => [] },
  };
  const sample: Record<string, string> = { '/api/models/<id>': '/api/models/some-model', '<id>': 'ID-AAAAAAAAAA', '<name>': 'kernel.log' };
  for (const [method, route] of ROUTES) {
    if (route.startsWith('/api/firmware/')) continue;
    let path: string = route;
    if (sample[path]) path = sample[path];
    path = path.replace('/runs/<id>', '/runs/RUN-AAAAAAAAAA').replace('/notes/<id>', '/notes/NOTE-AAAAAAAAAA').replace('<name>', sample['<name>']);
    const url = new URL(`https://arewearmyet.com${path}`);
    const response = await reports(new Request(url, { method, headers: { 'content-type': 'application/json' }, body: method === 'GET' ? undefined : '{}' }), env as never, url);
    assert.ok(response, `${method} ${route} is not served`);
    const text = await response.text();
    assert.ok(!text.includes('The API is described at'), `${method} ${route} is listed and not served: ${text}`);
  }
});

test('GET /api lists the routes and says where they are described', async () => {
  const url = new URL('https://arewearmyet.com/api');
  const response = (await reports(new Request(url), {} as never, url))!;
  const body = (await response.json()) as { docs: string; routes: { method: string; path: string; does: string }[] };
  assert.equal(body.docs, 'https://arewearmyet.com/docs/api');
  assert.deepEqual(body.routes.map((r) => `${r.method} ${r.path}`), listed);
  assert.ok(body.routes.every((r) => r.does.length > 10 || r.path === '/api'));
});
