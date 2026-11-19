// The hw-test reference as plain Markdown, for agents to read: https://arewearmyet.com/docs/hw-test.md
import raw from '../../lib/hw-test.md?raw';

export function GET() {
  return new Response(raw, { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
}
