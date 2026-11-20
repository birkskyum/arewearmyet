// Every model on the board, for the search in the bar at the top of each page, with its name, what
// the search matches on, and its page.
import { googlebookRows, nvidiaRows, snapdragonRows, type TableRow } from '../../lib/rows';

export function GET() {
  const entry = (row: TableRow, href: string) => ({ name: `${row.vendor} ${row.name}`, about: row.subtitle, href, text: row.search, series: row.prefixes });
  const models = [
    ...snapdragonRows().map((row) => entry(row, `/models/${row.id}`)),
    ...nvidiaRows().map((row) => entry(row, `/models/${row.id}`)),
    ...googlebookRows('android').map((row) => entry(row, `/models/${row.id}`)),
    ...googlebookRows('depthcharge').map((row) => entry(row, `/models/${row.id}`)),
  ].sort((a, b) => a.name.localeCompare(b.name));
  return new Response(JSON.stringify(models), { headers: { 'Content-Type': 'application/json' } });
}
