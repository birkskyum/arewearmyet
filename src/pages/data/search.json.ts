// Every model on the board, for the search in the bar at the top of each page: its name, what
// the search matches on, and where it leads. A model with a page of its own leads there, and one
// without (Chromebooks and Googlebooks) to its row on the home page.
import { googlebookRows, nvidiaRows, snapdragonRows, type TableRow } from '../../lib/rows';

export function GET() {
  const entry = (row: TableRow, href: string) => ({ name: `${row.vendor} ${row.name}`, about: row.subtitle, href, text: row.search, series: row.prefixes });
  const onHome = (prefix: string, anchor: string) => (row: TableRow) => entry(row, `/?${prefix}q=${encodeURIComponent(row.name.toLowerCase())}#${anchor}`);
  const models = [
    ...snapdragonRows().map((row) => entry(row, `/models/${row.id}`)),
    ...nvidiaRows().map((row) => entry(row, `/models/${row.id}`)),
    ...googlebookRows('android').map(onHome('googlebook-', 'googlebooks')),
    ...googlebookRows('depthcharge').map(onHome('chromebook-', 'chromebooks')),
  ].sort((a, b) => a.name.localeCompare(b.name));
  return new Response(JSON.stringify(models), { headers: { 'Content-Type': 'application/json' } });
}
