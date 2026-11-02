// Spreadsheet downloads for the tables. The writer loads on the first click, so the pages don't
// carry it.
import type { SheetData } from 'write-excel-file/browser';

export type Value = string | number | null;

const empty = (value: Value) => value === null || value === '';

// A bold header row over the rows, without columns that are empty in every row
export function sheet(header: string[], rows: Value[][]) {
  const keep = header.map((_, i) => rows.some((row) => !empty(row[i])));
  const pick = <T,>(cells: T[]) => cells.filter((_, i) => keep[i]);
  const kept = rows.map(pick);
  const columns = pick(header);
  const data: SheetData = [columns.map((value) => ({ value, fontWeight: 'bold' as const })), ...kept];
  // Each column as wide as its longest value, up to 60 characters
  const widths = columns.map((title, i) => ({ width: Math.min(60, Math.max(title.length, ...kept.map((row) => String(row[i] ?? '').length)) + 2) }));
  return { data, widths };
}

export async function downloadXlsx(name: string, title: string, header: string[], rows: Value[][]) {
  const { default: writeExcelFile } = await import('write-excel-file/browser');
  const { data, widths } = sheet(header, rows);
  await writeExcelFile(data, { sheet: title, stickyRowsCount: 1, columns: widths }).toFile(`${name}-${new Date().toISOString().slice(0, 10)}.xlsx`);
}
