/**
 * A nightly copy of the reports database in the private bucket, as one JSON file per day under
 * backups/reports/. D1 can roll back 30 days by itself; these files outlast that. A month of daily
 * copies is kept, and the first of each month for good.
 */

const TABLES = ['machines', 'runs', 'results', 'fixes', 'notes', 'note_logs'] as const;
const KEEP_DAYS = 31;

export async function backup(env: Env, now = new Date()): Promise<{ key: string; rows: Record<string, number> }> {
  const tables: Record<string, unknown[]> = {};
  for (const table of TABLES) tables[table] = (await env.DB.prepare(`SELECT * FROM ${table}`).all()).results;
  const day = now.toISOString().slice(0, 10);
  const key = `backups/reports/${day}.json`;
  await env.FIRMWARE.put(key, JSON.stringify({ taken: now.toISOString(), tables }), { httpMetadata: { contentType: 'application/json' } });

  const old = new Date(now.getTime() - KEEP_DAYS * 86_400_000).toISOString().slice(0, 10);
  if (!old.endsWith('-01')) await env.FIRMWARE.delete(`backups/reports/${old}.json`);
  return { key, rows: Object.fromEntries(Object.entries(tables).map(([table, rows]) => [table, rows.length])) };
}
