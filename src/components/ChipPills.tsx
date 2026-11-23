// The buttons in the page's sticky bar that choose a chip for the whole page, grouped by maker. The bars count the
// chosen chip, the sections that hold other chips hide (index.astro's rules on data-progress), and the tables of two
// chips show its rows only. Bring-up has no bars for NVIDIA's and Google's machines, so their buttons hide there
import { createEffect, createSignal, For, onCleanup, onMount } from 'solid-js';
import { chosenChip, PROGRESS_PARAM, setChosenChip, setPageChip } from '../lib/chip-choice';
import { MODE_EVENT, readMode, type Mode } from '../lib/mode';
import type { ProgressGroup } from '../lib/progress';

type Props = {
  groups: ProgressGroup[];
  counts: { all: number; uefi: number; groups: Record<string, number> };
};

export default function ChipPills(props: Props) {
  let row!: HTMLDivElement;
  const [mode, setMode] = createSignal<Mode>('hardware');
  const uefi = (key: string) => props.groups.find((group) => group.key === key)?.uefi ?? false;
  const pressed = () => (mode() === 'booting' && !uefi(chosenChip()) ? 'all' : chosenChip());
  const makers = [...new Set(props.groups.map((group) => group.maker))].map((maker) => ({ maker, groups: props.groups.filter((group) => group.maker === maker) }));

  const choose = (key: string) => {
    setChosenChip(key);
    const params = new URLSearchParams(location.search);
    if (key === 'all') params.delete(PROGRESS_PARAM);
    else params.set(PROGRESS_PARAM, key);
    const search = params.toString();
    history.replaceState(null, '', `${location.pathname}${search ? `?${search}` : ''}${location.hash}`);
  };
  onMount(() => {
    const asked = new URLSearchParams(location.search).get(PROGRESS_PARAM);
    if (asked && props.groups.some((group) => group.key === asked)) setChosenChip(asked);
    const heard = () => setMode(readMode());
    heard();
    addEventListener(MODE_EVENT, heard);
    onCleanup(() => removeEventListener(MODE_EVENT, heard));
    // A link to a section the chosen chip hides, like the Snapdragon group's link to the Chromebooks, shows every chip again
    const leave = (event: MouseEvent) => {
      const link = (event.target as Element | null)?.closest?.('a[href^="#"]') as HTMLAnchorElement | null;
      const target = link && document.getElementById(decodeURIComponent(link.hash.slice(1)));
      if (target && chosenChip() !== 'all' && target.closest('[data-chips]') && !target.checkVisibility()) choose('all');
    };
    addEventListener('click', leave);
    onCleanup(() => removeEventListener('click', leave));
  });
  createEffect(() => {
    const chip = pressed();
    setPageChip(chip);
    if (chip === 'all') delete document.documentElement.dataset.progress;
    else document.documentElement.dataset.progress = chip;
    // On a phone the buttons are one row that scrolls sideways, which brings the chosen one into view
    const button = row.querySelector<HTMLElement>(`[data-chip="${chip}"]`);
    if (button && row.scrollWidth > row.clientWidth && (button.offsetLeft < row.scrollLeft || button.offsetLeft + button.offsetWidth > row.scrollLeft + row.clientWidth)) {
      row.scrollLeft = button.offsetLeft - (row.clientWidth - button.offsetWidth) / 2;
    }
  });

  const pill = (active: boolean) =>
    `inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-sm ring-1 ${active ? 'bg-zinc-100 font-medium text-zinc-900 ring-zinc-100' : 'text-zinc-300 ring-zinc-700 hover:bg-zinc-800 hover:text-zinc-100'}`;
  return (
    <div ref={row} class="relative flex items-center gap-x-4 gap-y-2 max-sm:-mx-4 max-sm:overflow-x-auto max-sm:px-4 max-sm:py-0.5 max-sm:[scrollbar-width:none] sm:flex-wrap" role="group" aria-label="Which chip the page shows">
      <button type="button" data-chip="all" aria-pressed={pressed() === 'all'} onClick={() => choose('all')} class={pill(pressed() === 'all')}>
        All
        <span class="text-zinc-500">
          <span class="hidden hw:inline">{props.counts.all}</span>
          <span class="hw:hidden">{props.counts.uefi}</span>
        </span>
      </button>
      <For each={makers}>
        {(entry) => (
          <div class={`shrink-0 items-center gap-1.5 ${entry.groups.every((group) => group.uefi) ? 'flex' : 'hidden hw:flex'}`}>
            {entry.maker && <span class="mr-0.5 text-xs font-medium text-zinc-500">{entry.maker}</span>}
            <For each={entry.groups}>
              {(group) => (
                <button type="button" data-chip={group.key} aria-pressed={pressed() === group.key} aria-label={`${group.label}, ${props.counts.groups[group.key]} models`} onClick={() => choose(group.key)} class={pill(pressed() === group.key)} data-tip-title={group.label} data-tip={group.about ?? `${props.counts.groups[group.key]} models.`}>
                  {group.short}
                  <span class="text-zinc-500">{props.counts.groups[group.key]}</span>
                </button>
              )}
            </For>
          </div>
        )}
      </For>
    </div>
  );
}
