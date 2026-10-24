/** @jsxImportSource solid-js */
// The model table. TanStack Table does the sorting, filtering and per-option counts; each column
// header carries its own sort button and filter menu, and the view lives in the page address so
// a filtered table can be shared.
import { For, Show, createEffect, createSignal, onCleanup, onMount } from 'solid-js';
import {
  columnFacetingFeature,
  columnFilteringFeature,
  createColumnHelper,
  createFacetedRowModel,
  createFacetedUniqueValues,
  createFilteredRowModel,
  createSortedRowModel,
  createTable,
  filterFn_includesString,
  rowSortingFeature,
  sortFn_alphanumeric,
  tableFeatures,
  type ColumnFiltersState,
  type SortingState,
} from '@tanstack/solid-table';
import { PILL, TONE, type Tone } from '../lib/tones';
import type { CellData, TableRow } from '../lib/rows';

type Option = { key: string; label: string; tone?: Tone };
type Column = { id: string; header: string; question: string; options?: Option[] };
type Props = {
  rows: TableRow[];
  columns: Column[];
  // Filter choices for the Model column, by chip generation
  chips: Option[];
  // Prefix for this table's address parameters; leave out to keep the view out of the address
  urlPrefix?: string;
};

const features = tableFeatures({
  columnFilteringFeature,
  rowSortingFeature,
  columnFacetingFeature,
  filteredRowModel: createFilteredRowModel(),
  sortedRowModel: createSortedRowModel(),
  facetedRowModel: createFacetedRowModel(),
  facetedUniqueValues: createFacetedUniqueValues(),
  filterFns: { includesString: filterFn_includesString },
  sortFns: { alphanumeric: sortFn_alphanumeric },
});

type Row = { original: TableRow; getValue: (id: string) => unknown };
// A row passes when its value is one of the ticked choices
const inSet = (row: Row, id: string, value: unknown) => !Array.isArray(value) || value.length === 0 || value.includes(row.getValue(id));
const byRank = (a: Row, b: Row, id: string) => (a.original.cells[id]?.rank ?? Infinity) - (b.original.cells[id]?.rank ?? Infinity);

const VARIANT_TONE = { good: 'text-emerald-400', close: 'text-amber-400', wanted: 'text-rose-400', muted: 'text-zinc-500' };
const ICON_BUTTON = 'inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-100';

function Pill(props: { cell: CellData; column: string }) {
  const cell = props.cell;
  if ('text' in cell) return <span class="text-xs text-zinc-400">{cell.text}</span>;
  const title = `${props.column}: ${cell.title}`;
  const classes = `${PILL} ${TONE[cell.tone]}`;
  return cell.href ? (
    <a href={cell.href} title={title} class={`${classes} hover:brightness-125`}>
      {cell.label}
    </a>
  ) : (
    <span title={title} class={classes}>
      {cell.label}
    </span>
  );
}

export default function StatusTable(props: Props) {
  const helper = createColumnHelper<typeof features, TableRow>();
  const columns = helper.columns([
    helper.accessor('vendor', { id: 'brand', header: 'Brand', sortFn: 'alphanumeric', filterFn: inSet }),
    helper.accessor('name', { id: 'model', header: 'Model', sortFn: 'alphanumeric' }),
    helper.accessor('generation', { id: 'chip', filterFn: inSet }),
    // Status columns filter and count by state, and sort best first by rank
    ...props.columns.map((column) => helper.accessor((row) => row.cells[column.id]?.key ?? '', { id: column.id, header: column.header, filterFn: inSet, sortFn: byRank, sortDescFirst: false })),
    helper.accessor('search', { id: 'q', filterFn: 'includesString' }),
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
  const headers = [
    { id: 'brand', header: 'Brand', question: 'Who makes it', filterColumn: 'brand', options: brands as Option[] },
    { id: 'model', header: 'Model', question: 'The model and its variants', filterColumn: 'chip', options: props.chips },
    ...props.columns.map((column) => ({ ...column, filterColumn: column.options ? column.id : undefined })),
  ];

  const selected = (id: string) => (filters().find((filter) => filter.id === id)?.value as string[] | undefined) ?? [];
  const setSelected = (id: string, keys: string[]) => setFilters((current) => [...current.filter((filter) => filter.id !== id), ...(keys.length ? [{ id, value: keys }] : [])]);
  const toggle = (id: string, key: string) => setSelected(id, selected(id).includes(key) ? selected(id).filter((k) => k !== key) : [...selected(id), key]);
  const query = () => (filters().find((filter) => filter.id === 'q')?.value as string | undefined) ?? '';
  const setQuery = (value: string) => setFilters((current) => [...current.filter((filter) => filter.id !== 'q'), ...(value ? [{ id: 'q', value }] : [])]);
  const counts = (id: string) => table.getColumn(id)?.getFacetedUniqueValues() ?? new Map<unknown, number>();
  const shown = () => table.getRowModel().rows.length;
  const filtering = () => filters().length > 0;

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
    const ids = headers.map((header) => header.filterColumn).filter(Boolean) as string[];
    const params = new URLSearchParams(location.search);
    setFilters([
      ...ids.filter((id) => params.get(prefix + id)).map((id) => ({ id, value: params.get(prefix + id)!.split(',') })),
      ...(params.get(prefix + 'q') ? [{ id: 'q', value: params.get(prefix + 'q')!.toLowerCase() }] : []),
    ]);
    const sort = params.get(prefix + 'sort');
    if (sort) setSorting([{ id: sort.replace(/^-/, ''), desc: sort.startsWith('-') }]);
    createEffect(() => {
      const next = new URLSearchParams(location.search);
      for (const key of [...ids, 'q', 'sort']) next.delete(prefix + key);
      for (const filter of filters()) next.set(prefix + filter.id, Array.isArray(filter.value) ? filter.value.join(',') : String(filter.value));
      const [first] = sorting();
      if (first) next.set(prefix + 'sort', `${first.desc ? '-' : ''}${first.id}`);
      const search = next.toString().replace(/%2C/g, ',');
      history.replaceState(null, '', `${location.pathname}${search ? `?${search}` : ''}${location.hash}`);
    });
  });

  return (
    <div>
      <div class="flex flex-wrap items-center gap-3">
        <input
          type="search"
          placeholder="Search models"
          value={query()}
          onInput={(event) => setQuery(event.currentTarget.value.trim().toLowerCase())}
          class="w-64 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 ring-1 ring-zinc-700 placeholder:text-zinc-500 focus:outline-none focus:ring-zinc-400"
        />
        <span class="text-sm text-zinc-500">Filter and sort from the column headers.</span>
        <Show when={filtering()}>
          <button type="button" onClick={() => setFilters([])} class="rounded-md px-2.5 py-1 text-sm text-zinc-300 ring-1 ring-zinc-700 hover:bg-zinc-800" data-clear>
            Clear filters
          </button>
        </Show>
        <span class="ml-auto text-sm tabular-nums text-zinc-500" data-count>
          {shown()} of {props.rows.length}
        </span>
      </div>

      <div class="mt-4 overflow-x-auto rounded-xl ring-1 ring-zinc-800">
        <table class="w-full text-left text-sm">
          <thead class="bg-zinc-900 text-xs uppercase tracking-wide text-zinc-400">
            <tr>
              <For each={headers}>
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
                    <th class={header.id === 'brand' ? 'px-4 py-3 font-medium' : 'px-3 py-3 font-medium'} title={header.question} aria-sort={direction() === 'asc' ? 'ascending' : direction() === 'desc' ? 'descending' : 'none'}>
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
                          <span title={model.announced!} class={`ml-1.5 align-middle ${PILL} ${TONE.info}`}>
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
                      <Show when={model.notes}>
                        <p class="mt-1.5 max-w-xs text-xs text-zinc-500">{model.notes}</p>
                      </Show>
                    </td>
                    <For each={props.columns}>
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
