// The newest owners' runs and linked fixes, with buttons to hide wrong or abusive ones.
// Needs the Worker's ADMIN_TOKEN, kept only in this browser.
import { createResource, createSignal, For, Show } from 'solid-js';

type Result = { check: string; state: string; how: string; note: string | null };
type Run = { id: string; machine: string; model: string | null; created: string; mode: string; kernel: string | null; agent: string | null; hidden: number; machine_hidden: number; results: Result[] };
type Fix = { id: string; model: string; check: string | null; url: string; note: string | null; created: string; hidden: number };

const KEY = 'arewearmyet-admin-token';
const read = () => { try { return localStorage.getItem(KEY) ?? ''; } catch { return ''; } };
const write = (value: string) => { try { localStorage.setItem(KEY, value); } catch { /* private window */ } };
const BUTTON = 'rounded px-2 py-0.5 text-xs ring-1 ring-zinc-700 hover:bg-zinc-800';
const TONE: Record<string, string> = { works: 'text-emerald-300', partial: 'text-amber-300', broken: 'text-rose-300', absent: 'text-zinc-500', untested: 'text-zinc-500' };

export default function Review() {
  const [token, setToken] = createSignal(read());
  const [data, { refetch }] = createResource(token, async (t) => {
    if (!t) return null;
    const response = await fetch('/api/review', { headers: { authorization: `Bearer ${t}` } });
    if (!response.ok) throw new Error((await response.json().catch(() => ({}))).error ?? response.statusText);
    return (await response.json()) as { runs: Run[]; fixes: Fix[] };
  });
  const hide = async (kind: 'run' | 'fix' | 'machine', id: string, hidden: boolean) => {
    await fetch('/api/review/hide', { method: 'POST', headers: { authorization: `Bearer ${token()}`, 'content-type': 'application/json' }, body: JSON.stringify({ kind, id, hidden }) });
    refetch();
  };

  return (
    <div class="space-y-8">
      <form class="flex gap-2" onSubmit={(e) => { e.preventDefault(); const value = (e.currentTarget.elements.namedItem('token') as HTMLInputElement).value.trim(); write(value); setToken(value); }}>
        <input name="token" type="password" value={token()} placeholder="Admin token" class="w-72 rounded bg-zinc-900 px-3 py-1.5 text-sm text-zinc-100 ring-1 ring-zinc-700" />
        <button class={BUTTON} type="submit">Load</button>
      </form>
      <Show when={data.error}><p class="text-sm text-rose-300">{String(data.error?.message ?? data.error)}</p></Show>
      <Show when={data()}>
        {(d) => (
          <>
            <section>
              <h2 class="text-lg font-semibold text-zinc-100">Runs</h2>
              <ul class="mt-3 space-y-3">
                <For each={d().runs}>
                  {(run) => (
                    <li class={`rounded-lg bg-zinc-900 p-4 ring-1 ring-zinc-800 ${run.hidden || run.machine_hidden ? 'opacity-50' : ''}`}>
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                        <a class="font-mono text-zinc-200 hover:underline" href={`/api/runs/${run.id}`}>{run.id}</a>
                        {run.model ? <a class="text-zinc-300 hover:underline" href={`/models/${run.model}`}>{run.model}</a> : <span class="text-amber-300">no model</span>}
                        <span class="text-xs text-zinc-500">{run.created.slice(0, 16).replace('T', ' ')} · {run.mode}{run.kernel ? ` · ${run.kernel}` : ''}{run.agent ? ` · ${run.agent}` : ''} · machine {run.machine.slice(0, 8)}</span>
                        <span class="ml-auto flex gap-2">
                          <button class={BUTTON} onClick={() => hide('run', run.id, !run.hidden)}>{run.hidden ? 'Show run' : 'Hide run'}</button>
                          <button class={BUTTON} onClick={() => hide('machine', run.machine, !run.machine_hidden)}>{run.machine_hidden ? 'Show machine' : 'Hide machine'}</button>
                        </span>
                      </div>
                      <ul class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs">
                        <For each={run.results}>
                          {(r) => <li title={r.note ?? ''}><span class="text-zinc-400">{r.check}</span> <span class={TONE[r.state] ?? ''}>{r.state}</span> <span class="text-zinc-600">{r.how}</span></li>}
                        </For>
                      </ul>
                    </li>
                  )}
                </For>
              </ul>
            </section>
            <section>
              <h2 class="text-lg font-semibold text-zinc-100">Fixes</h2>
              <ul class="mt-3 space-y-2 text-sm">
                <For each={d().fixes}>
                  {(fix) => (
                    <li class={`flex flex-wrap items-center gap-3 ${fix.hidden ? 'opacity-50' : ''}`}>
                      <a class="text-zinc-300 hover:underline" href={`/models/${fix.model}`}>{fix.model}</a>
                      <span class="text-zinc-500">{fix.check ?? 'general'}</span>
                      <a class="text-zinc-200 underline decoration-zinc-600" href={fix.url}>{fix.note ?? fix.url}</a>
                      <button class={`${BUTTON} ml-auto`} onClick={() => hide('fix', fix.id, !fix.hidden)}>{fix.hidden ? 'Show' : 'Hide'}</button>
                    </li>
                  )}
                </For>
              </ul>
            </section>
          </>
        )}
      </Show>
    </div>
  );
}
