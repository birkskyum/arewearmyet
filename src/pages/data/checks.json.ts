// data/checks.yaml as JSON, served as GET /api/checks. It says what each check means, what the test measures and what the owner does.
import { CHECKS, GROUPS } from '../../lib/checks';

export function GET() {
  return new Response(JSON.stringify({ states: ['works', 'partial', 'broken', 'untested', 'absent'], how: ['measured', 'owner', 'agent'], groups: GROUPS, checks: CHECKS }, null, 1) + '\n', {
    headers: { 'Content-Type': 'application/json' },
  });
}
