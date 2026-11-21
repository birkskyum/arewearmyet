// The page-wide switch between Hardware (whether a model boots and what works on it) and Bring-up
// (the device tree, hardware IDs and firmware it needs), called booting here. The page opens in
// Hardware, which the home page's <html> element carries from the start. The mode lives there, where
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
