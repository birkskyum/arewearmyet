/** @jsxImportSource solid-js */
// The model table: TanStack Table does the sorting and filtering, and the filters and sort
// live in the page address so a filtered view can be shared
import { For, Show, createEffect, createSignal, onMount } from 'solid-js';
import {
  columnFilteringFeature,
  createColumnHelper,
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
import { PILL, TONE } from '../lib/tones';
import type { CellData, TableRow } from '../lib/rows';

type Column = { id: string; header: string; question: string };
type Choice = { value: string; label: string };
type Props = {
  rows: TableRow[];
  columns: Column[];
  // Filters and their address parameters are only shown when choices are given
  chips?: Choice[];
  outcomes?: Choice[];
  vendors?: string[];
};

const features = tableFeatures({
  columnFilteringFeature,
  rowSortingFeature,
  filteredRowModel: createFilteredRowModel(),
  sortedRowModel: createSortedRowModel(),
  filterFns: { includesString: filterFn_includesString },
  sortFns: { alphanumeric: sortFn_alphanumeric },
});

const BUTTON = 'rounded-md px-2.5 py-1 text-sm text-zinc-300 ring-1 ring-zinc-700 hover:bg-zinc-800 aria-pressed:bg-zinc-200 aria-pressed:text-zinc-900 aria-pressed:ring-zinc-200';
const VARIANT_TONE = { good: 'text-emerald-400', close: 'text-amber-400', wanted: 'text-rose-400', muted: 'text-zinc-500' };
const equals = (row: { getValue: (id: string) => unknown }, id: string, value: unknown) => row.getValue(id) === value;

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
  const rank = (cell: CellData | undefined) => cell?.rank ?? Number.MAX_SAFE_INTEGER;
  const columns = helper.columns([
    helper.accessor('vendor', { id: 'vendor', header: 'Brand', sortFn: 'alphanumeric', filterFn: equals }),
    helper.accessor('name', { id: 'model', header: 'Model', sortFn: 'alphanumeric' }),
    // Ranks put the best state first, so the first click sorts best to worst
    ...props.columns.map((column) => helper.accessor((row) => rank(row.cells[column.id]), { id: column.id, header: column.header, sortDescFirst: false })),
    helper.accessor('generation', { id: 'chip', filterFn: equals }),
    helper.accessor('outcome', { id: 'omarchy-status', filterFn: equals }),
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

  const filterValue = (id: string) => (filters().find((filter) => filter.id === id)?.value as string | undefined) ?? '';
  const setFilter = (id: string, value: string) => setFilters((current) => [...current.filter((filter) => filter.id !== id), ...(value ? [{ id, value }] : [])]);
  const shown = () => table.getRowModel().rows.length;
  const hasFilters = Boolean(props.chips || props.outcomes || props.vendors);

  // Read the view from the address once, then keep the address in step with it
  const PARAMS = ['chip', 'omarchy-status', 'vendor', 'q'];
  onMount(() => {
    if (!hasFilters) return;
    const params = new URLSearchParams(location.search);
    setFilters(PARAMS.filter((id) => params.get(id)).map((id) => ({ id, value: id === 'q' ? params.get(id)!.toLowerCase() : params.get(id)! })));
    const sort = params.get('sort');
    if (sort) setSorting([{ id: sort.replace(/^-/, ''), desc: sort.startsWith('-') }]);
    createEffect(() => {
      const next = new URLSearchParams();
      for (const filter of filters()) next.set(filter.id, String(filter.value));
      const [first] = sorting();
      if (first) next.set('sort', `${first.desc ? '-' : ''}${first.id}`);
      const query = next.toString();
      history.replaceState(null, '', `${location.pathname}${query ? `?${query}` : ''}${location.hash}`);
    });
  });

  return (
    <div>
      <Show when={hasFilters}>
        <div class="flex flex-wrap items-center gap-2">
          <input
            type="search"
            placeholder="Search models"
            value={filterValue('q')}
            onInput={(event) => setFilter('q', event.currentTarget.value.trim().toLowerCase())}
            class="w-56 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 ring-1 ring-zinc-700 placeholder:text-zinc-500 focus:outline-none focus:ring-zinc-400"
          />
          <Show when={props.chips}>
            <span class="ml-2 text-sm text-zinc-500">Chip</span>
            <For each={[{ value: '', label: 'All' }, ...props.chips!]}>
              {(choice) => (
                <button class={BUTTON} data-filter="chip" data-value={choice.value} aria-pressed={filterValue('chip') === choice.value} onClick={() => setFilter('chip', choice.value)}>
                  {choice.label}
                </button>
              )}
            </For>
          </Show>
          <Show when={props.outcomes}>
            <span class="ml-2 text-sm text-zinc-500">Omarchy</span>
            <For each={[{ value: '', label: 'All' }, ...props.outcomes!]}>
              {(choice) => (
                <button class={BUTTON} data-filter="omarchy-status" data-value={choice.value} aria-pressed={filterValue('omarchy-status') === choice.value} onClick={() => setFilter('omarchy-status', choice.value)}>
                  {choice.label}
                </button>
              )}
            </For>
          </Show>
          <Show when={props.vendors}>
            <select
              aria-label="Brand"
              value={filterValue('vendor')}
              onChange={(event) => setFilter('vendor', event.currentTarget.value)}
              class="ml-2 rounded-md bg-zinc-900 px-2 py-1.5 text-sm text-zinc-200 ring-1 ring-zinc-700"
            >
              <option value="">All brands</option>
              <For each={props.vendors}>{(vendor) => <option value={vendor}>{vendor}</option>}</For>
            </select>
          </Show>
          <span class="ml-auto text-sm tabular-nums text-zinc-500" data-count>
            {shown()} of {props.rows.length}
          </span>
        </div>
      </Show>

      <div class="mt-4 overflow-x-auto rounded-xl ring-1 ring-zinc-800">
        <table class="w-full text-left text-sm">
          <thead class="bg-zinc-900 text-xs uppercase tracking-wide text-zinc-400">
            <tr>
              <For each={[{ id: 'vendor', header: 'Brand', question: 'Who makes it' }, { id: 'model', header: 'Model', question: 'The model and its variants' }, ...props.columns]}>
                {(column) => {
                  const tableColumn = table.getColumn(column.id)!;
                  const direction = () => (sorting()[0]?.id === column.id ? (sorting()[0].desc ? 'desc' : 'asc') : null);
                  return (
                    <th class={column.id === 'vendor' ? 'px-4 py-3 font-medium' : 'px-3 py-3 font-medium'} title={column.question} aria-sort={direction() === 'asc' ? 'ascending' : direction() === 'desc' ? 'descending' : 'none'}>
                      <button type="button" class="inline-flex items-center gap-1 uppercase tracking-wide hover:text-zinc-100" onClick={tableColumn.getToggleSortingHandler()}>
                        {column.header}
                        <span class={direction() ? 'text-zinc-200' : 'text-zinc-600'} aria-hidden="true">
                          {direction() === 'asc' ? '↑' : direction() === 'desc' ? '↓' : '↕'}
                        </span>
                      </button>
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
