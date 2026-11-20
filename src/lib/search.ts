// A link elsewhere on the page that searches one of its tables, like a model's cell in the bars.
// The link's own address does the same after a page load, so it still works in a new tab, without
// scripts, and before the table has woken up. A plain click asks the table directly instead
// (search-links.ts), and the table answers by cancelling the event.
export const SEARCH_EVENT = 'arewearmyet:search';
export type SearchAsked = { prefix: string; query: string };
