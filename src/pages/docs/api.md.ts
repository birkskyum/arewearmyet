// The API reference as plain Markdown, for agents to read: https://arewearmyet.com/docs/api.md
import raw from '../../lib/api.md?raw';

export function GET() {
  return new Response(raw, { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
}
