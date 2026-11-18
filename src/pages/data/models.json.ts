// Every model with a page, served as GET /api/models.
import { coverage, pageModels } from '../../lib/checks';

export function GET() {
  const models = pageModels().map((model) => ({
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    generation: model.generation,
    chips: model.chips,
    modelNumbers: model.modelNumbers,
    compatibles: model.variants.map((v) => v.compatible).filter(Boolean),
    outcome: model.outcome,
    bootable: model.bootable,
    checksWrittenUp: coverage(model).known,
    page: `https://arewearmyet.com/models/${model.id}`,
    api: `https://arewearmyet.com/api/models/${model.id}`,
  }));
  return new Response(JSON.stringify({ models }, null, 1) + '\n', { headers: { 'Content-Type': 'application/json' } });
}
