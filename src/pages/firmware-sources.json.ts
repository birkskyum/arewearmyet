// data/firmware-sources.yaml as JSON, for tools on any distro to read.
import { parse } from 'yaml';
import raw from '../../data/firmware-sources.yaml?raw';

export function GET() {
  return new Response(JSON.stringify(parse(raw), null, 1) + '\n', {
    headers: { 'Content-Type': 'application/json' },
  });
}
