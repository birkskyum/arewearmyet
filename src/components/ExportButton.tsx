/** @jsxImportSource solid-js */
// Downloads a table as an .xlsx file. The rows are read on click, so a filtered table exports
// what it shows.
import { createSignal } from 'solid-js';
import { downloadXlsx, type Value } from '../lib/xlsx';

type Props = {
  // File name without the date and extension, which the download adds
  name: string;
  // The sheet's name in the workbook
  title: string;
  header: string[];
  rows: Value[][];
};

export default function ExportButton(props: Props) {
  const [busy, setBusy] = createSignal(false);
  const download = async () => {
    setBusy(true);
    try {
      await downloadXlsx(props.name, props.title, props.header, props.rows);
    } finally {
      setBusy(false);
    }
  };
  return (
    <button type="button" onClick={download} disabled={busy()} class="rounded-md px-2.5 py-1 text-sm text-zinc-300 ring-1 ring-zinc-700 hover:bg-zinc-800 disabled:opacity-50" data-export>
      Export .xlsx
    </button>
  );
}
