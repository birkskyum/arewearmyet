// The bars at the top of the home page, which count every model on it. The buttons above them narrow
// them to one kind of chip. What works shows the parts past booting, and Bring-up the pieces a Snapdragon
// PC that starts through UEFI needs. Each model is one cell, coloured by its state and linking to its table
import { createMemo, createSignal, For, onCleanup, onMount, Show, type JSX } from 'solid-js';
import { MODE_EVENT, readMode, type Mode } from '../lib/mode';
import type { PieceKind, ProgressGroup, ProgressModel } from '../lib/progress';

type Props = {
  models: ProgressModel[];
  groups: ProgressGroup[];
  parts: { title: string; question: string }[];
};

// The chip the bars count sits in the address, so a link can open them on it
const PARAM = 'progress';
const LINK = 'underline decoration-zinc-600 underline-offset-2 hover:text-zinc-100 hover:decoration-zinc-300';

const PART_KINDS = [
  { key: 'omarchy-works', color: 'bg-emerald-400', label: 'Works on Omarchy' },
  { key: 'omarchy-partial', color: 'bg-amber-400', label: 'Partly' },
  { key: 'omarchy-broken', color: 'bg-rose-400', label: 'Broken' },
  { key: 'unknown', color: 'bg-zinc-800', label: 'Not tested' },
];
const partKind = (key: string) => PART_KINDS.find((kind) => kind.key === key) ?? PART_KINDS[3];
const PIECE_KINDS: { key: PieceKind; color: string }[] = [
  { key: 'upstream', color: 'bg-sky-400' },
  { key: 'added', color: 'bg-emerald-400' },
  { key: 'pull', color: 'bg-amber-400' },
  { key: 'available', color: 'bg-zinc-600' },
  { key: 'none', color: 'bg-zinc-800' },
];
const PIECES = [
  { label: 'Device tree', ours: `Trees Omarchy's kernel adds.`, theirs: 'In a Linux release.', none: 'Nobody has written its tree yet.' },
  { label: 'Hardware IDs', ours: `IDs Omarchy's kernel adds, or systemd's that only Omarchy's systemd has.`, theirs: 'In a systemd release.', none: 'Nobody has sent its hardware IDs yet.' },
  { label: 'Firmware', ours: 'Firmware Omarchy packages.', theirs: 'Every file in a linux-firmware release.', none: 'Its firmware has only turned up in Windows so far.' },
  { label: 'All three', ours: 'Device tree, hardware IDs and firmware, all in Omarchy, where upstream lacks at least one.', theirs: 'All three in the releases.', none: `At least one piece doesn't exist yet.` },
];
const pieceTip = (piece: (typeof PIECES)[number], kind: PieceKind) =>
  ({
    upstream: `${piece.theirs} Omarchy ships it too.`,
    added: 'Only Omarchy ships it so far.',
    pull: 'An open pull request on omarchy-pkgs adds it.',
    available: `It exists elsewhere, but Omarchy doesn't carry it yet.`,
    none: piece.none,
  })[kind];
// A channel's count includes the models a more stable channel already has, as each channel ships what the one before it does
const CHANNELS = ['stable', 'rc', 'edge'];
const channelNote = (channels: (string | null)[]) => {
  const upTo = (name: string) => channels.filter((channel) => channel !== null && CHANNELS.indexOf(channel) <= CHANNELS.indexOf(name)).length;
  return `Stable ${upTo('stable')}, RC ${upTo('rc')}, edge ${upTo('edge')}.`;
};

const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name);
// A bar is one row of cells while each cell stays wide enough to point at, and wraps to more rows past that, filling each
// column top to bottom so the bar still reads from left to right. Narrow bars take half as many cells per row
function grid(count: number): JSX.CSSProperties {
  const shape = (perRow: number) => {
    const rows = Math.max(1, Math.ceil(count / perRow));
    return { cols: `repeat(${Math.max(1, Math.ceil(count / rows))}, minmax(0, 1fr))`, rows: `repeat(${rows}, minmax(0, 1fr))`, height: rows === 1 ? '0.75rem' : `${rows * 5 + (rows - 1) * 2}px` };
  };
  const wide = shape(90);
  const narrow = shape(45);
  return { '--cw': wide.cols, '--rw': wide.rows, '--hw': wide.height, '--cn': narrow.cols, '--rn': narrow.rows, '--hn': narrow.height };
}
const BAR = 'grid-flow-col gap-0.5 grid-cols-(--cn) grid-rows-(--rn) h-(--hn) @lg:grid-cols-(--cw) @lg:grid-rows-(--rw) @lg:h-(--hw)';
const CELL = 'rounded-[2px] transition hover:scale-y-150 hover:brightness-150';
const COUNTER = 'w-28 shrink-0 whitespace-nowrap tabular-nums text-zinc-300';

export default function Progress(props: Props) {
  const [chosen, setChosen] = createSignal('all');
  const [mode, setMode] = createSignal<Mode>('hardware');
  const uefi = (key: string) => props.groups.find((group) => group.key === key)?.uefi ?? false;
  // Bring-up has no bars for NVIDIA's and Google's machines, so with one of them chosen it counts every model it has bars for
  const pieceChoice = () => (uefi(chosen()) ? chosen() : 'all');
  const pressed = () => (mode() === 'booting' ? pieceChoice() : chosen());
  const inChoice = (choice: string) => (model: ProgressModel) => choice === 'all' || model.group === choice;

  onMount(() => {
    const asked = new URLSearchParams(location.search).get(PARAM);
    if (asked && props.groups.some((group) => group.key === asked)) setChosen(asked);
    const heard = () => setMode(readMode());
    heard();
    addEventListener(MODE_EVENT, heard);
    onCleanup(() => removeEventListener(MODE_EVENT, heard));
  });
  const choose = (key: string) => {
    setChosen(key);
    const params = new URLSearchParams(location.search);
    if (key === 'all') params.delete(PARAM);
    else params.set(PARAM, key);
    const search = params.toString();
    history.replaceState(null, '', `${location.pathname}${search ? `?${search}` : ''}${location.hash}`);
  };

  const withPieces = props.models.filter((model) => model.pieces);
  // Each bar puts its cells in the legend's order, and by name within each state. Filtering hides cells and keeps that order
  const partBars = props.parts.map((part, i) => ({
    part,
    cells: props.models
      .flatMap((model) => (model.parts[i] ? [{ model, name: model.name, ...model.parts[i]! }] : []))
      .map((cell) => ({ ...cell, ...partKind(cell.kind) }))
      .sort((a, b) => PART_KINDS.indexOf(partKind(a.kind)) - PART_KINDS.indexOf(partKind(b.kind)) || byName(a, b)),
  }));
  const pieceBars = PIECES.map((piece, i) => ({
    piece,
    cells: withPieces
      .map((model) => ({ model, name: model.name, ...model.pieces![i] }))
      .map((cell) => ({ ...cell, color: PIECE_KINDS.find((kind) => kind.key === cell.kind)!.color, tip: pieceTip(piece, cell.kind) }))
      .sort((a, b) => PIECE_KINDS.findIndex((kind) => kind.key === a.kind) - PIECE_KINDS.findIndex((kind) => kind.key === b.kind) || byName(a, b)),
  }));

  const button = (key: string, active: () => boolean) =>
    `items-center gap-1.5 rounded-full px-3 py-1 text-sm ring-1 ${active() ? 'bg-zinc-100 font-medium text-zinc-900 ring-zinc-100' : 'text-zinc-300 ring-zinc-700 hover:bg-zinc-800 hover:text-zinc-100'} ${key !== 'all' && !uefi(key) ? 'hidden hw:inline-flex' : 'inline-flex'}`;

  return (
    <section class="mt-12 scroll-mt-20" id="progress" aria-label="How far the models are">
      <div class="flex flex-wrap items-baseline justify-between gap-x-8 gap-y-2">
        <h2 class="hidden text-lg font-semibold text-zinc-50 hw:block">Past booting, part by part</h2>
        <h2 class="text-lg font-semibold text-zinc-50 hw:hidden">Omarchy and upstream</h2>
        <ul class="hidden flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-400 hw:flex">
          <For each={PART_KINDS}>{(kind) => <li class="flex items-center gap-1.5"><span class={`size-2.5 rounded-[2px] ${kind.color}`}></span>{kind.label}</li>}</For>
        </ul>
        <ul class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-400 hw:hidden">
          <li class="flex items-center gap-1.5"><span class="size-2.5 rounded-[2px] bg-sky-400"></span>Upstream releases</li>
          <li class="flex items-center gap-1.5"><span class="size-2.5 rounded-[2px] bg-emerald-400"></span>Omarchy adds</li>
          <li class="flex items-center gap-1.5"><span class="size-2.5 rounded-[2px] bg-amber-400"></span><a class={LINK} href="https://github.com/omacom/omarchy-pkgs/pulls">Open pull requests</a></li>
          <li class="flex items-center gap-1.5"><span class="size-2.5 rounded-[2px] bg-zinc-600"></span>Exists elsewhere</li>
        </ul>
      </div>
      <div class="mt-4 flex flex-wrap gap-2" role="group" aria-label="Which models the bars count">
        <button type="button" aria-pressed={pressed() === 'all'} onClick={() => choose('all')} class={button('all', () => pressed() === 'all')}>
          All
          <span class="text-zinc-500">
            <span class="hidden hw:inline">{props.models.length}</span>
            <span class="hw:hidden">{withPieces.length}</span>
          </span>
        </button>
        <For each={props.groups}>
          {(group) => (
            <button type="button" aria-pressed={pressed() === group.key} onClick={() => choose(group.key)} class={button(group.key, () => pressed() === group.key)}>
              {group.label}
              <span class="text-zinc-500">{props.models.filter(inChoice(group.key)).length}</span>
            </button>
          )}
        </For>
      </div>

      <dl class="mt-5 hidden gap-x-8 gap-y-5 sm:grid-cols-2 hw:grid">
        <For each={partBars}>
          {(bar) => {
            const shown = createMemo(() => bar.cells.filter((cell) => inChoice(chosen())(cell.model)));
            const count = (key: string) => shown().filter((cell) => cell.kind === key).length;
            const tested = () => count('omarchy-works') + count('omarchy-partial') + count('omarchy-broken');
            const tip = () =>
              tested() ? `Tested on Omarchy on ${tested()} so far. It works on ${count('omarchy-works')}, partly on ${count('omarchy-partial')} and is broken on ${count('omarchy-broken')}.` : 'Nobody has tested it on Omarchy yet.';
            return (
              <div>
                <dt class="text-sm font-medium text-zinc-200">{bar.part.title}</dt>
                <dd class="mt-2 flex items-center gap-3 text-xs">
                  <div class="@container min-w-0 flex-1">
                    <Show when={shown().length === 0}>
                      <p class="text-zinc-500">None of these models has it.</p>
                    </Show>
                    <div class={`${BAR} ${shown().length ? 'grid' : 'hidden'}`} style={grid(shown().length)}>
                      <For each={bar.cells}>
                        {(cell) => (
                          <a
                            href={`/?${cell.model.prefix}q=${encodeURIComponent(cell.name)}#${cell.model.anchor}`}
                            data-search={cell.name}
                            data-prefix={cell.model.prefix}
                            class={`${CELL} ${cell.color}`}
                            classList={{ hidden: !inChoice(chosen())(cell.model) }}
                            data-tip-title={`${bar.part.title} on the ${cell.name}`}
                            data-tip={cell.tip}
                            aria-label={`${bar.part.title} on the ${cell.name}, ${cell.label.toLowerCase()}`}
                          ></a>
                        )}
                      </For>
                    </div>
                  </div>
                  <span class={COUNTER} classList={{ invisible: shown().length === 0 }} data-tip-title={`${bar.part.title} works on ${count('omarchy-works')} of the ${shown().length} models that have it`} data-tip={`${tip()} ${bar.part.question}`}>
                    {count('omarchy-works')} <span class="text-zinc-500">/ {shown().length}</span>
                  </span>
                </dd>
              </div>
            );
          }}
        </For>
      </dl>

      <dl class="mt-5 grid gap-x-8 gap-y-5 sm:grid-cols-2 hw:hidden">
        <For each={pieceBars}>
          {(bar, i) => {
            const shown = createMemo(() => bar.cells.filter((cell) => inChoice(pieceChoice())(cell.model)));
            const count = (key: PieceKind) => shown().filter((cell) => cell.kind === key).length;
            const omarchy = () => count('upstream') + count('added');
            // The last bar, all three pieces, has no channel of its own
            const ours = () => (i() < 3 ? `${bar.piece.ours} ${channelNote(shown().map((cell) => cell.channel))}` : bar.piece.ours);
            return (
              <div>
                <dt class="text-sm font-medium text-zinc-200">{bar.piece.label}</dt>
                <dd class="mt-2 flex items-center gap-3 text-xs">
                  <div class="@container min-w-0 flex-1">
                    <div class={`${BAR} grid`} style={grid(shown().length)}>
                      <For each={bar.cells}>
                        {(cell) => (
                          <a
                            href={`/?view=bring-up&${cell.model.prefix}q=${encodeURIComponent(cell.name)}#${cell.model.anchor}`}
                            data-search={cell.name}
                            data-prefix={cell.model.prefix}
                            class={`${CELL} ${cell.color}`}
                            classList={{ hidden: !inChoice(pieceChoice())(cell.model) }}
                            data-tip-title={cell.name}
                            data-tip={cell.tip}
                            aria-label={`${cell.name}. ${cell.tip}`}
                          ></a>
                        )}
                      </For>
                    </div>
                  </div>
                  <span class={COUNTER} data-tip-title={`In Omarchy for ${omarchy()} of ${shown().length}`} data-tip={ours()}>
                    {omarchy()}
                    <Show when={count('pull') > 0}><span class="text-amber-300"> +{count('pull')}</span></Show>
                    <Show when={count('available') > 0}><span class="text-zinc-500"> +{count('available')}</span></Show>{' '}
                    <span class="text-zinc-500">/ {shown().length}</span>
                  </span>
                </dd>
              </div>
            );
          }}
        </For>
      </dl>
    </section>
  );
}
