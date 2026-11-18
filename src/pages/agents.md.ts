// The agent guide as plain Markdown, for agents to read: https://arewearmyet.com/agents.md
import raw from '../lib/agents.md?raw';

export function GET() {
  return new Response(raw, { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
}
