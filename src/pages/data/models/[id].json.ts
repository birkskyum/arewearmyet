// One model's data, which GET /api/models/<id> adds owners' live results and fixes to.
import type { APIRoute } from 'astro';
import { modelData, pageModels } from '../../../lib/checks';
import type { Model } from '../../../lib/labels';

export function getStaticPaths() {
  return pageModels().map((model) => ({ params: { id: model.id }, props: { model } }));
}

export const GET: APIRoute = ({ props }) =>
  new Response(JSON.stringify(modelData(props.model as Model), null, 1) + '\n', { headers: { 'Content-Type': 'application/json' } });
