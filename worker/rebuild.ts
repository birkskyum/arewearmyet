/**
 * Asks GitHub to build the site again after a change to what /api/owners serves, so the overview's
 * part bars and Boots column follow owners' results within minutes instead of the next daily run.
 * It starts .github/workflows/refresh.yml with `results`, which only copies the results, builds and
 * deploys. REBUILD_TOKEN is a fine-grained token with Actions write on the repository, a secret
 * (wrangler secret put REBUILD_TOKEN). Without it, or when GitHub refuses, the site waits for the
 * daily run. One ask a minute at most: a build that copied the results before the rest of a burst
 * came in asks again itself when it ends.
 */

const WORKFLOW = 'https://api.github.com/repos/birkskyum/arewearmyet/actions/workflows/refresh.yml/dispatches';
const EVERY_MS = 60_000;

export async function askForRebuild(env: Env, now = new Date(), send: typeof fetch = fetch): Promise<boolean> {
  const token = (env as unknown as { REBUILD_TOKEN?: string }).REBUILD_TOKEN;
  if (!token) return false;
  try {
    const { meta } = await env.DB.prepare('UPDATE rebuild SET asked = ?1 WHERE id = 1 AND asked < ?2')
      .bind(now.toISOString(), new Date(now.getTime() - EVERY_MS).toISOString()).run();
    if (!meta.changes) return false;
    const response = await send(WORKFLOW, {
      method: 'POST',
      headers: { authorization: `Bearer ${token}`, accept: 'application/vnd.github+json', 'user-agent': 'arewearmyet', 'x-github-api-version': '2022-11-28' },
      body: JSON.stringify({ ref: 'main', inputs: { results: true } }),
      signal: AbortSignal.timeout(10_000),
    });
    if (!response.ok) console.log(`GitHub refused the rebuild: ${response.status} ${await response.text()}`);
    return response.ok;
  } catch (error) {
    console.log(`asking for a rebuild failed: ${error}`);
    return false;
  }
}
