// The page-wide switch between Booting and Hardware. The mode lives on the <html> element, where
// CSS reads it (the hw: variant in global.css), and every table on the page follows it.
export type Mode = 'booting' | 'hardware';
export const MODE_EVENT = 'arewearmyet:mode';

export const readMode = (): Mode => (document.documentElement.dataset.mode === 'hardware' ? 'hardware' : 'booting');

export function setMode(mode: Mode) {
  if (mode === readMode()) return;
  if (mode === 'hardware') document.documentElement.dataset.mode = 'hardware';
  else delete document.documentElement.dataset.mode;
  dispatchEvent(new CustomEvent(MODE_EVENT));
}
