// The page's side of search.ts. A plain click on a link marked data-search asks its table to search
import { SEARCH_EVENT, type SearchAsked } from './search';

addEventListener('click', (event) => {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  const link = (event.target as Element | null)?.closest?.('a[data-search]') as HTMLAnchorElement | null;
  if (!link) return;
  // With no table to answer, the link loads the page as before
  const asked = new CustomEvent<SearchAsked>(SEARCH_EVENT, { cancelable: true, detail: { prefix: link.dataset.prefix ?? '', query: link.dataset.search! } });
  if (!dispatchEvent(asked)) event.preventDefault();
});
