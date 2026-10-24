// Colour classes for the status labels, kept apart from the data so the table island stays small
export type Tone = 'good' | 'close' | 'wanted' | 'muted' | 'info';

export const TONE: Record<Tone, string> = {
  good: 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30',
  close: 'bg-amber-500/15 text-amber-300 ring-amber-500/30',
  wanted: 'bg-rose-500/15 text-rose-300 ring-rose-500/30',
  muted: 'bg-zinc-500/10 text-zinc-400 ring-zinc-500/20',
  info: 'bg-sky-500/15 text-sky-300 ring-sky-500/30',
};

export const PILL = 'inline-flex shrink-0 items-center whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset';
