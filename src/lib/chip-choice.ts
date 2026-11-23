// The chip chosen with the buttons above the bars (Progress.tsx), for the whole page. The islands load this module
// once, so they share the one signal, and a table that holds two chips follows it. It is 'all' until the bars have
// read the address, which keeps the first render the same as the built page
import { createSignal } from 'solid-js';

export const PROGRESS_PARAM = 'progress';
export const [pageChip, setPageChip] = createSignal('all');
