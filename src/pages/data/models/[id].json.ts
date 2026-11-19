// One model's data, which GET /api/models/<id> adds owners' live results and fixes to.
import type { APIRoute } from 'astro';
import { modelData, nvidiaData, nvidiaPageModels, pageModels } from '../../../lib/checks';
import type { Model, NvidiaModel } from '../../../lib/labels';

export function getStaticPaths() {
  return [
    ...pageModels().map((model) => ({ params: { id: model.id }, props: { model } })),
    ...nvidiaPageModels().map((model) => ({ params: { id: model.id }, props: { nvidia: model } })),
  ];
}

export const GET: APIRoute = ({ props }) =>
  new Response(JSON.stringify(props.nvidia ? nvidiaData(props.nvidia as NvidiaModel) : modelData(props.model as Model), null, 1) + '\n', { headers: { 'Content-Type': 'application/json' } });
