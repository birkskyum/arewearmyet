// Saves owners' results as the site serves them (GET /api/owners) to data/owners.json, so the
// repository keeps their history next to the findings. They hold counts and notes per model and
// check, nothing that names a machine or its owner. Run with `pnpm owners`.

import { writeFileSync } from 'node:fs';

const response = await fetch('https://arewearmyet.com/api/owners');
if (!response.ok) throw new Error(`${response.status} from /api/owners`);
const owners = (await response.json()) as { models: Record<string, unknown>; unmatched: unknown[] };
// Models in a fixed order, so a day without new reports changes nothing
const models = Object.fromEntries(Object.entries(owners.models).sort(([a], [b]) => a.localeCompare(b)));
writeFileSync('data/owners.json', JSON.stringify({ models, unmatched: owners.unmatched }, null, 1) + '\n');
console.log(`${Object.keys(models).length} models with owners' results, ${owners.unmatched.length} unmatched machines`);
