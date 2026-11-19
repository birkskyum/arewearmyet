// Every model with a page, served as GET /api/models.
import { coverage, nvidiaPageModels, pageModels } from '../../lib/checks';

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
  // NVIDIA machines start through ACPI, so they have no chips, model numbers or compatibles to list
  const nvidia = nvidiaPageModels().map((model) => ({
    id: model.id,
    vendor: model.vendor,
    name: model.name,
    generation: model.generation,
    chips: [],
    modelNumbers: model.modelNumbers ?? [],
    compatibles: [],
    outcome: model.omarchy,
    bootable: model.kernel ?? null,
    checksWrittenUp: coverage(model).known,
    page: `https://arewearmyet.com/models/${model.id}`,
    api: `https://arewearmyet.com/api/models/${model.id}`,
  }));
  models.push(...(nvidia as never[]));
  return new Response(JSON.stringify({ models }, null, 1) + '\n', { headers: { 'Content-Type': 'application/json' } });
}
