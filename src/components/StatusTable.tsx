/** @jsxImportSource solid-js */
// The model table. TanStack Table does the sorting, filtering and per-option counts; each column
// header carries its own sort button and filter menu, and the view lives in the page address so
// a filtered table can be shared.
import { For, Show, createEffect, createSignal, onCleanup, onMount, untrack } from 'solid-js';
import {
  columnFacetingFeature,
  columnFilteringFeature,
  createColumnHelper,
  createFacetedRowModel,
  createFacetedUniqueValues,
  createFilteredRowModel,
  createSortedRowModel,
  createTable,
  rowSortingFeature,
  sortFn_alphanumeric,
  tableFeatures,
  type ColumnFiltersState,
  type SortingState,
} from '@tanstack/solid-table';
import { PILL, TONE, type Tone } from '../lib/tones';
import type { CellData, TableRow } from '../lib/rows';
import ExportButton from './ExportButton';

type Option = { key: string; label: string; tone?: Tone };
type Column = { id: string; header: string; question: string; options?: Option[] };
// A set of status columns the table can switch to, like what Omarchy ships or what's upstream
type View = { id: string; label: string; question: string; columns: Column[] };
type Props = {
  rows: TableRow[];
  // One set of status columns, or several views to switch between, the first one shown by default
  columns?: Column[];
  views?: View[];
  // Filter choices for the Model column, by chip generation
  chips: Option[];
  // Prefix for this table's address parameters; leave out to keep the view out of the address
  urlPrefix?: string;
  // The spreadsheet download's file name (without date and extension) and sheet name
  exportName: string;
  exportTitle: string;
};

const features = tableFeatures({
  columnFilteringFeature,
  rowSortingFeature,
  columnFacetingFeature,
  filteredRowModel: createFilteredRowModel(),
  sortedRowModel: createSortedRowModel(),
  facetedRowModel: createFacetedRowModel(),
  facetedUniqueValues: createFacetedUniqueValues(),
  sortFns: { alphanumeric: sortFn_alphanumeric },
});

type Row = { original: TableRow; getValue: (id: string) => unknown };
// A row passes when its value is one of the ticked choices
const inSet = (row: Row, id: string, value: unknown) => !Array.isArray(value) || value.length === 0 || value.includes(row.getValue(id));
// The search box matches a row that has every word of it, in any part of its text or as a full product number that starts with one of its series
const bySearch = (row: Row, _id: string, value: unknown) =>
  typeof value !== 'string' || !value || value.split(' ').every((word) => row.original.search.includes(word) || row.original.prefixes.some((prefix) => word.startsWith(prefix)));
// The search as the filter holds it: lowercase, single spaces, none at the ends
const normalized = (text: string) => text.trim().toLowerCase().split(/\s+/).join(' ');
const byRank = (a: Row, b: Row, id: string) => (a.original.cells[id]?.rank ?? Infinity) - (b.original.cells[id]?.rank ?? Infinity);
// A status cell as the table shows it
const cellText = (cell: CellData | undefined) => (!cell ? null : 'text' in cell ? cell.text || null : cell.label);

const VARIANT_TONE = { good: 'text-emerald-400', close: 'text-amber-400', wanted: 'text-rose-400', muted: 'text-zinc-500' };
const ICON_BUTTON = 'inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100';

function Pill(props: { cell: CellData; column: string }) {
  const cell = props.cell;
  if ('text' in cell) return <span class="text-xs text-zinc-400">{cell.text}</span>;
  const classes = `${PILL} ${TONE[cell.tone]}`;
  return cell.href ? (
    <a href={cell.href} data-tip={cell.title} data-tip-title={props.column} class={`${classes} hover:brightness-125`}>
      {cell.label}
    </a>
  ) : (
    <span data-tip={cell.title} data-tip-title={props.column} class={classes}>
      {cell.label}
    </span>
  );
}

export default function StatusTable(props: Props) {
  const views: View[] = props.views ?? [{ id: 'default', label: '', question: '', columns: props.columns ?? [] }];
  const [viewId, setViewId] = createSignal(views[0].id);
  const view = () => views.find((v) => v.id === viewId()) ?? views[0];
  const statusColumns = () => view().columns;
  // Every view's columns exist in the table, so a view switch keeps sorting and counting in one place
  const allColumns = [...new Map(views.flatMap((v) => v.columns).map((column) => [column.id, column])).values()];
  const helper = createColumnHelper<typeof features, TableRow>();
  const columns = helper.columns([
    helper.accessor('vendor', { id: 'brand', header: 'Brand', sortFn: 'alphanumeric', filterFn: inSet }),
    helper.accessor('name', { id: 'model', header: 'Model', sortFn: 'alphanumeric' }),
    helper.accessor('generation', { id: 'chip', filterFn: inSet }),
    // Status columns filter and count by state, and sort best first by rank
    ...allColumns.map((column) => helper.accessor((row) => row.cells[column.id]?.key ?? '', { id: column.id, header: column.header, filterFn: inSet, sortFn: byRank, sortDescFirst: false })),
    helper.accessor('search', { id: 'q', filterFn: bySearch }),
  ]);

  const [sorting, setSorting] = createSignal<SortingState>([]);
  const [filters, setFilters] = createSignal<ColumnFiltersState>([]);
  const table = createTable({
    features,
    columns,
    get data() {
      return props.rows;
    },
    state: {
      get sorting() {
        return sorting();
      },
      get columnFilters() {
        return filters();
      },
    },
    onSortingChange: setSorting,
    onColumnFiltersChange: setFilters,
  });

  const brands = [...new Set(props.rows.map((row) => row.vendor))].sort().map((vendor) => ({ key: vendor, label: vendor }));
  // Every header: what it sorts by, and which column and choices its filter menu uses
  const fixedHeaders = [
    { id: 'brand', header: 'Brand', question: 'Who makes it', filterColumn: 'brand', options: brands as Option[] },
    { id: 'model', header: 'Model', question: 'The model and its variants', filterColumn: props.chips.length ? 'chip' : undefined, options: props.chips },
  ];
  const headerFor = (column: Column) => ({ ...column, filterColumn: column.options ? column.id : undefined });
  const headers = () => [...fixedHeaders, ...statusColumns().map(headerFor)];

  const selected = (id: string) => (filters().find((filter) => filter.id === id)?.value as string[] | undefined) ?? [];
  const setSelected = (id: string, keys: string[]) => setFilters((current) => [...current.filter((filter) => filter.id !== id), ...(keys.length ? [{ id, value: keys }] : [])]);
  const toggle = (id: string, key: string) => setSelected(id, selected(id).includes(key) ? selected(id).filter((k) => k !== key) : [...selected(id), key]);
  const query = () => (filters().find((filter) => filter.id === 'q')?.value as string | undefined) ?? '';
  const setQuery = (value: string) => setFilters((current) => [...current.filter((filter) => filter.id !== 'q'), ...(value ? [{ id: 'q', value }] : [])]);
  // The search box keeps the text as typed, so a space at the end stays while the next word is typed
  const [typed, setTyped] = createSignal('');
  createEffect(() => {
    if (normalized(untrack(typed)) !== query()) setTyped(query());
  });
  const counts = (id: string) => table.getColumn(id)?.getFacetedUniqueValues() ?? new Map<unknown, number>();
  const shown = () => table.getRowModel().rows.length;
  // The spreadsheet holds the rows the table shows, in its order, with the labels it shows
  const exportHeader = () => ['Brand', 'Model', 'Details', ...statusColumns().map((column) => column.header), 'Variants', 'Notes', 'Link'];
  const exportRows = () =>
    table.getRowModel().rows.map(({ original: model }) => [
      model.vendor,
      model.name,
      model.subtitle || null,
      ...statusColumns().map((column) => cellText(model.cells[column.id])),
      model.variants.map((variant) => `${variant.text}: ${variant.state}`).join('; ') || null,
      model.notes,
      model.href,
    ]);
  const filtering = () => filters().length > 0;
  // The active filters as the bar above the table names them: the column, and each ticked choice with its pill
  const filterLabel: Record<string, string> = { brand: 'Brand', chip: 'Chip' };
  const activeFilters = () =>
    filters()
      .filter((filter) => filter.id !== 'q')
      .map((filter) => {
        const header = headers().find((h) => h.filterColumn === filter.id);
        const keys = (filter.value as string[]) ?? [];
        const options = (header?.options ?? []) as Option[];
        return {
          id: filter.id,
          column: filterLabel[filter.id] ?? header?.header ?? filter.id,
          choices: keys.map((key) => options.find((option) => option.key === key) ?? { key, label: key }),
        };
      });

  // Switching views drops the filters and sorting on columns the new view doesn't have
  const switchView = (id: string) => {
    const kept = new Set(['brand', 'chip', 'q', ...(views.find((v) => v.id === id)?.columns ?? []).map((column) => column.id)]);
    setFilters((current) => current.filter((filter) => kept.has(filter.id)));
    setSorting((current) => current.filter((sort) => kept.has(sort.id) || sort.id === 'model'));
    setViewId(id);
  };
  // A page section marked data-views shows the legend of the view in use
  let root!: HTMLDivElement;
  onMount(() => createEffect(() => root.closest('[data-views]')?.setAttribute('data-view', viewId())));

  // An open menu stays under its filter button when the page scrolls or resizes, or the columns
  // change width after filtering; it closes once the button is off screen
  const follow = () => {
    for (const menu of document.querySelectorAll<HTMLElement>('[data-filter-menu]:popover-open')) {
      const rect = document.querySelector(`[popovertarget="${menu.id}"]`)!.getBoundingClientRect();
      if (rect.bottom < 0 || rect.top > innerHeight) {
        menu.hidePopover();
      } else {
        menu.style.top = `${rect.bottom + 6}px`;
        menu.style.left = `${Math.max(8, Math.min(rect.left, innerWidth - 272))}px`;
      }
    }
  };
  onMount(() => {
    addEventListener('scroll', follow, { passive: true });
    addEventListener('resize', follow);
    onCleanup(() => {
      removeEventListener('scroll', follow);
      removeEventListener('resize', follow);
    });
    createEffect(() => {
      filters();
      requestAnimationFrame(follow);
    });
  });

  // Read the view from the address once, then keep this table's part of the address in step
  onMount(() => {
    if (props.urlPrefix === undefined) return;
    const prefix = props.urlPrefix;
    const ids = [...fixedHeaders, ...allColumns.map(headerFor)].map((header) => header.filterColumn).filter(Boolean) as string[];
    const params = new URLSearchParams(location.search);
    const fromAddress = params.get(prefix + 'view');
    if (fromAddress && views.some((v) => v.id === fromAddress)) setViewId(fromAddress);
    setFilters([
      ...ids.filter((id) => params.get(prefix + id)).map((id) => ({ id, value: params.get(prefix + id)!.split(',') })),
      ...(params.get(prefix + 'q') ? [{ id: 'q', value: normalized(params.get(prefix + 'q')!) }] : []),
    ]);
    const sort = params.get(prefix + 'sort');
    if (sort) setSorting([{ id: sort.replace(/^-/, ''), desc: sort.startsWith('-') }]);
    createEffect(() => {
      const next = new URLSearchParams(location.search);
      for (const key of [...ids, 'q', 'sort', 'view']) next.delete(prefix + key);
      if (viewId() !== views[0].id) next.set(prefix + 'view', viewId());
      for (const filter of filters()) next.set(prefix + filter.id, Array.isArray(filter.value) ? filter.value.join(',') : String(filter.value));
      const [first] = sorting();
      if (first) next.set(prefix + 'sort', `${first.desc ? '-' : ''}${first.id}`);
      const search = next.toString().replace(/%2C/g, ',');
      history.replaceState(null, '', `${location.pathname}${search ? `?${search}` : ''}${location.hash}`);
    });
  });

  return (
    <div ref={root}>
      <div class="flex flex-wrap items-center gap-3">
        <Show when={views.length > 1}>
          <div class="inline-flex rounded-lg bg-zinc-900 p-0.5 ring-1 ring-zinc-700" role="tablist" aria-label="View">
            <For each={views}>
              {(v) => (
                <button
                  type="button"
                  role="tab"
                  aria-selected={viewId() === v.id}
                  onClick={() => switchView(v.id)}
                  data-tip={v.question}
                  data-tip-title={v.label}
                  data-view-button={v.id}
                  class={`rounded-md px-3 py-1 text-sm font-medium ${viewId() === v.id ? 'bg-zinc-100 text-zinc-900' : 'text-zinc-300 hover:bg-zinc-800 hover:text-zinc-100'}`}
                >
                  {v.label}
                </button>
              )}
            </For>
          </div>
        </Show>
        <input
          type="search"
          placeholder="Search by name or model number"
          value={typed()}
          onInput={(event) => {
            setTyped(event.currentTarget.value);
            setQuery(normalized(event.currentTarget.value));
          }}
          class="w-72 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 ring-1 ring-zinc-700 placeholder:text-zinc-500 focus:outline-none focus:ring-zinc-400"
        />
        <details class="relative text-sm text-zinc-500">
          <summary class="cursor-pointer list-none hover:text-zinc-300">Where's my model number?</summary>
          <div class="absolute left-0 z-20 mt-2 w-80 max-w-[calc(100vw-2rem)] rounded-md bg-zinc-900 p-3 text-zinc-300 shadow-lg ring-1 ring-zinc-700">
            <p>It's on the sticker underneath. In Windows, open System Information and look at System Model and System SKU. On Linux, run</p>
            <pre class="mt-2 overflow-x-auto rounded bg-zinc-950 p-2 text-xs">{"cat /sys/class/dmi/id/product_name\ncat /sys/class/dmi/id/product_sku"}</pre>
            <p class="mt-2">A full number like <span class="whitespace-nowrap">83ED0049US</span> or <span class="whitespace-nowrap">14-fe0013dx</span> finds its series.</p>
          </div>
        </details>
        <span class="text-sm text-zinc-500">Filter and sort from the column headers.</span>
        <span class={`ml-auto text-sm tabular-nums ${filtering() ? 'font-medium text-sky-300' : 'text-zinc-500'}`} data-count>
          {shown()} of {props.rows.length}
        </span>
        <ExportButton name={props.exportName} title={props.exportTitle} header={exportHeader()} rows={exportRows()} />
      </div>

      <Show when={filtering()}>
        <div class="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-sky-400/10 px-3 py-2 text-sm ring-1 ring-sky-400/40" data-active-filters>
          <span class="font-medium text-sky-200">
            Showing {shown()} of {props.rows.length}, filtered by
          </span>
          <For each={activeFilters()}>
            {(filter) => (
              <span class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 py-0.5 pl-2 pr-1 ring-1 ring-zinc-700">
                <span class="text-xs text-zinc-400">{filter.column}</span>
                <For each={filter.choices.filter((choice) => choice.tone)}>{(choice) => <span class={`${PILL} ${TONE[choice.tone!]}`}>{choice.label}</span>}</For>
                <Show when={filter.choices.some((choice) => !choice.tone)}>
                  <span class="text-zinc-200">{filter.choices.filter((choice) => !choice.tone).map((choice) => choice.label).join(', ')}</span>
                </Show>
                <button type="button" aria-label={`Remove the ${filter.column} filter`} class="rounded px-1 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100" onClick={() => setSelected(filter.id, [])} data-remove-filter={filter.id}>
                  ×
                </button>
              </span>
            )}
          </For>
          <Show when={query()}>
            <span class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 py-0.5 pl-2 pr-1 ring-1 ring-zinc-700">
              <span class="text-xs text-zinc-400">Search</span>
              <span class="text-zinc-200">{query()}</span>
              <button type="button" aria-label="Clear the search" class="rounded px-1 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100" onClick={() => setQuery('')}>
                ×
              </button>
            </span>
          </Show>
          <button type="button" onClick={() => setFilters([])} class="ml-auto rounded-md px-2.5 py-1 text-sm text-sky-100 ring-1 ring-sky-400/50 hover:bg-sky-400/20" data-clear>
            Clear all
          </button>
        </div>
      </Show>

      <div class="mt-4 overflow-x-auto rounded-xl ring-1 ring-zinc-800">
        <table class="w-full text-left text-sm">
          <thead class="bg-zinc-900 text-xs uppercase tracking-wide text-zinc-400">
            <tr>
              <For each={headers()}>
                {(header) => {
                  const direction = () => (sorting()[0]?.id === header.id ? (sorting()[0].desc ? 'desc' : 'asc') : null);
                  const menuId = `${props.urlPrefix || 'table-'}filter-${header.id}`;
                  let button!: HTMLButtonElement;
                  // The menu sits in the top layer, so place it under its button by hand
                  const place = (menu: HTMLElement, event: Event) => {
                    if ((event as ToggleEvent).newState !== 'open') return;
                    const rect = button.getBoundingClientRect();
                    menu.style.top = `${rect.bottom + 6}px`;
                    menu.style.left = `${Math.max(8, Math.min(rect.left, innerWidth - 272))}px`;
                  };
                  return (
                    <th class={`${header.id === 'brand' ? 'px-4 py-3 font-medium' : 'px-3 py-3 font-medium'} ${header.filterColumn && selected(header.filterColumn).length ? 'bg-sky-400/10 text-sky-200' : ''}`} data-tip={header.question} data-tip-title={header.header} aria-sort={direction() === 'asc' ? 'ascending' : direction() === 'desc' ? 'descending' : 'none'}>
                      <span class="inline-flex items-center gap-1">
                        <button type="button" class="inline-flex items-center gap-1 uppercase tracking-wide hover:text-zinc-100" onClick={table.getColumn(header.id)!.getToggleSortingHandler()} data-sort={header.id}>
                          {header.header}
                          <span class={direction() ? 'text-zinc-200' : 'text-zinc-600'} aria-hidden="true">
                            {direction() === 'asc' ? '↑' : direction() === 'desc' ? '↓' : '↕'}
                          </span>
                        </button>
                        <Show when={header.filterColumn}>
                          <button
                            type="button"
                            ref={button}
                            {...{ popovertarget: menuId }}
                            aria-label={`Filter ${header.header}`}
                            class={`${ICON_BUTTON} ${selected(header.filterColumn!).length ? 'text-sky-300' : ''}`}
                            data-filter-button={header.filterColumn}
                          >
                            <svg class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                              <path d="M2.5 4.25A.75.75 0 0 1 3.25 3.5h13.5a.75.75 0 0 1 .58 1.22L12 11.3v4.45a.75.75 0 0 1-.42.67l-2.5 1.25A.75.75 0 0 1 8 17v-5.7L2.67 4.72a.75.75 0 0 1-.17-.47Z" />
                            </svg>
                            <Show when={selected(header.filterColumn!).length}>
                              <span class="text-[10px] tabular-nums">{selected(header.filterColumn!).length}</span>
                            </Show>
                          </button>
                          <div
                            id={menuId}
                            {...{ popover: 'auto' }}
                            data-filter-menu={header.filterColumn}
                            ref={(menu: HTMLElement) => menu.addEventListener('beforetoggle', (event) => place(menu, event))}
                            class="fixed m-0 w-64 rounded-lg bg-zinc-900 p-3 text-sm font-normal normal-case tracking-normal text-zinc-200 shadow-2xl ring-1 ring-zinc-700"
                          >
                            <div class="mb-2 flex items-center justify-between text-xs text-zinc-400">
                              <span>Show only</span>
                              <Show when={selected(header.filterColumn!).length}>
                                <button type="button" class="hover:text-zinc-100" onClick={() => setSelected(header.filterColumn!, [])}>
                                  Clear
                                </button>
                              </Show>
                            </div>
                            <ul class="space-y-1">
                              <For each={header.options}>
                                {(option) => {
                                  const count = () => counts(header.filterColumn!).get(option.key) ?? 0;
                                  return (
                                    <li>
                                      <label class={`flex cursor-pointer items-center gap-2 rounded px-1.5 py-1 hover:bg-zinc-800 ${count() === 0 && !selected(header.filterColumn!).includes(option.key) ? 'opacity-40' : ''}`}>
                                        <input
                                          type="checkbox"
                                          class="accent-zinc-200"
                                          checked={selected(header.filterColumn!).includes(option.key)}
                                          onChange={() => toggle(header.filterColumn!, option.key)}
                                          data-option={option.key}
                                        />
                                        {option.tone ? <span class={`${PILL} ${TONE[option.tone]}`}>{option.label}</span> : <span>{option.label}</span>}
                                        <span class="ml-auto text-xs tabular-nums text-zinc-500">{count()}</span>
                                      </label>
                                    </li>
                                  );
                                }}
                              </For>
                            </ul>
                          </div>
                        </Show>
                      </span>
                    </th>
                  );
                }}
              </For>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-800">
            <For each={table.getRowModel().rows}>
              {(row) => {
                const model = row.original;
                return (
                  <tr class="align-top hover:bg-zinc-900/60" data-row data-chip={model.generation} data-outcome={model.outcome}>
                    <td class="whitespace-nowrap px-4 py-3 text-zinc-300">{model.vendor}</td>
                    <td class="px-3 py-3">
                      <div class="font-medium text-zinc-100">
                        {model.href ? (
                          <a href={model.href} class="hover:underline hover:decoration-zinc-500 hover:underline-offset-2">
                            {model.name}
                          </a>
                        ) : (
                          model.name
                        )}
                        <Show when={model.announced}>
                          <span data-tip={model.announced!} class={`ml-1.5 align-middle ${PILL} ${TONE.info}`}>
                            Announced
                          </span>
                        </Show>
                      </div>
                      <div class="mt-0.5 text-xs text-zinc-500">{model.subtitle}</div>
                      <Show when={model.variants.length > 0}>
                        <ul class="mt-1.5 space-y-0.5 text-xs text-zinc-400">
                          <For each={model.variants}>
                            {(variant) => (
                              <li>
                                {variant.text} <span class={VARIANT_TONE[variant.tone]}>{variant.state}</span>
                              </li>
                            )}
                          </For>
                        </ul>
                      </Show>
                      <Show when={model.notes || model.discussion}>
                        <p class="mt-1.5 max-w-xs text-xs text-zinc-500">
                          {model.notes}
                          <Show when={model.discussion}>
                            {' '}
                            <a href={model.discussion!} class="text-zinc-300 underline decoration-zinc-600 underline-offset-2 hover:text-zinc-100">
                              Owner guide
                            </a>
                          </Show>
                        </p>
                      </Show>
                    </td>
                    <For each={statusColumns()}>
                      {(column) => (
                        <td class="px-3 py-3">
                          <Pill cell={model.cells[column.id]} column={column.header} />
                        </td>
                      )}
                    </For>
                  </tr>
                );
              }}
            </For>
          </tbody>
        </table>
      </div>
      <Show when={shown() === 0}>
        <p class="mt-4 text-sm text-zinc-500">No models match.</p>
      </Show>
    </div>
  );
}
