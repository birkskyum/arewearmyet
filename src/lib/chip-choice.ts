// The chip chosen with the buttons in the page's sticky bar (ChipPills.tsx), for the whole page. The islands load this
// module once, so they share its signals: the bars count the chosen chip, and a table that holds two chips follows the
// page's. Both are 'all' until the buttons have read the address, which keeps the first render the same as the built page
import { createSignal } from 'solid-js';

export const PROGRESS_PARAM = 'progress';
// The chip the buttons show as chosen
export const [chosenChip, setChosenChip] = createSignal('all');
// The chip the page shows, which is 'all' in Bring-up while an NVIDIA or Google chip is chosen
export const [pageChip, setPageChip] = createSignal('all');
