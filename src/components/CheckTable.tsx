// A model's checks, grouped, from two sources kept apart: the notes written by hand in
// data/findings.yaml, and what hw-test measured on people's machines. The second is what owners'
// runs say, which comes live from /api/models/<id> so a new run shows without a rebuild.
import { createSignal, For, onMount, Show } from 'solid-js';
import { PILL, TONE } from '../lib/tones';

type State = 'works' | 'partial' | 'broken' | 'untested' | 'absent';
type Finding = { state: State; note?: string; version?: string; checked?: string; source?: string };
type Row = { id: string; group: string; title: string; works: string; probe?: string; person?: string; debug?: string; optional?: boolean; finding: Finding | null; from: 'model' | 'family' | null; familySource: string | null };
type Change = { machines: number; from: string | null; to: string | null; since: string };
type Owner = { machines: number; states: Record<string, number>; how: Record<string, number>; regressed?: Change; fixed?: Change; latest: { state: State; how: string; note: string | null; evidence: string | null; kernel: string | null; created: string; run: string }[] };
type Note = { id: string; check: string | null; kind: string; title: string; status?: string; created: string; replies: number; logs: { name: string }[]; kernel: string | null };
type Live = { owners: { machines: number; checks: Record<string, Owner> }; fixes: { id: string; check: string | null; url: string; note: string | null; created: string }[]; notes?: Note[] };
const KIND: Record<string, string> = { problem: 'Problem', workaround: 'Workaround', upstream: 'Work elsewhere', note: 'Note' };

const LABEL: Record<State, string> = { works: 'Works', partial: 'Partly', broken: 'Broken', untested: 'Untested', absent: 'Not on this model' };
const TONES: Record<State, keyof typeof TONE> = { works: 'good', partial: 'close', broken: 'wanted', untested: 'muted', absent: 'muted' };
const HOW: Record<string, string> = { measured: 'measured by the test', owner: 'seen by the person testing', agent: "an agent's reading" };
const ORDER: State[] = ['works', 'partial', 'broken', 'absent', 'untested'];
const LINK = 'underline decoration-zinc-600 underline-offset-2 hover:text-zinc-100 hover:decoration-zinc-300';

// "Worked on 7.3.0-67, broken since 7.3.0-72 (2026-11-17), on 2 machines"
function changeText(change: Change, kind: 'regressed' | 'fixed'): string {
  const on = change.machines === 1 ? 'on one machine' : `on ${change.machines} machines`;
  const from = change.from ?? 'an earlier kernel';
  const to = change.to ?? 'a newer kernel';
  return kind === 'regressed'
    ? `Worked on ${from}, not working since ${to} (${change.since.slice(0, 10)}), ${on}.`
    : `Didn't work on ${from}, works since ${to} (${change.since.slice(0, 10)}), ${on}.`;
}

async function load(id: string): Promise<Live | null> {
  try {
    const response = await fetch(`/api/models/${id}`);
    return response.ok ? ((await response.json()) as Live) : null;
  } catch {
    return null;
  }
}

function Pill(props: { state: State; dim?: boolean; prefix?: string; tip?: string }) {
  return (
    <span class={`${PILL} ${TONE[TONES[props.state]]} ${props.dim ? 'opacity-70' : ''}`} data-tip={props.tip}>
      {props.prefix ? `${props.prefix}: ` : ''}
      {LABEL[props.state]}
    </span>
  );
}

export default function CheckTable(props: { model: string; groups: { id: string; title: string }[]; rows: Row[] }) {
  // Fetched in the browser only: the page is built ahead of time, and owners' runs change after that
  const [live, setLive] = createSignal<Live | null>(null);
  const [state, setState] = createSignal<'loading' | 'done' | 'failed'>('loading');
  onMount(async () => {
    const data = await load(props.model);
    setLive(data);
    setState(data ? 'done' : 'failed');
  });
  const owner = (id: string) => live()?.owners.checks[id];
  const fixesFor = (id: string) => live()?.fixes.filter((fix) => fix.check === id) ?? [];
  const notesFor = (id: string) => live()?.notes?.filter((note) => note.check === id) ?? [];
  const noteLine = (note: Note) => (
    <li class="flex flex-wrap items-baseline gap-2">
      <span class={`${PILL} ${TONE[note.kind === 'problem' ? (note.status === 'solved' ? 'good' : 'wanted') : note.kind === 'workaround' ? 'close' : 'muted']}`}>{note.kind === 'problem' && note.status === 'solved' ? 'Solved' : (KIND[note.kind] ?? note.kind)}</span>
      <a class={LINK} href={`/note?id=${note.id}`}>{note.title}</a>
      <span class="text-xs text-zinc-500">
        {note.created.slice(0, 10)}
        {note.kernel ? ` · ${note.kernel}` : ''}
        {note.replies ? ` · ${note.replies} ${note.replies === 1 ? 'answer' : 'answers'}` : ''}
        {note.logs.length ? ` · ${note.logs.length} ${note.logs.length === 1 ? 'log' : 'logs'}` : ''}
      </span>
    </li>
  );

  return (
    <div class="space-y-6">
      <p class="text-sm text-zinc-400">
        <Show when={live()} fallback={<span>{state() === 'loading' ? 'Loading test results…' : 'Test results are unavailable right now.'}</span>}>
          {(data) => (
            <span>
              {data().owners.machines === 0 ? 'Nobody has run hw-test on this model yet.' : `Tested with hw-test on ${data().owners.machines} machine${data().owners.machines === 1 ? '' : 's'}, newest result per machine.`}
            </span>
          )}
        </Show>
      </p>
      <For each={props.groups}>
        {(group) => (
          <section>
            <h3 class="text-sm font-semibold uppercase tracking-wide text-zinc-500">{group.title}</h3>
            <div class="mt-2 divide-y divide-zinc-800 overflow-hidden rounded-xl bg-zinc-900 ring-1 ring-zinc-800">
              <For each={props.rows.filter((row) => row.group === group.id)}>
                {(row) => (
                  <details class="group/check">
                    <summary class="grid cursor-pointer list-none grid-cols-[1fr_auto] items-start gap-x-4 gap-y-1 px-4 py-3 hover:bg-zinc-800/50 sm:grid-cols-[14rem_1fr_auto] [&::-webkit-details-marker]:hidden">
                      <span class="font-medium text-zinc-100">{row.title}</span>
                      <span class="col-span-2 row-start-2 text-sm text-zinc-400 sm:col-span-1 sm:row-start-auto">
                        {/* The two sources, each named: what someone wrote down, and what hw-test found */}
                        <Show when={row.finding}>
                          {(f) => (
                            <span class="block">
                              <span class="text-zinc-500">Notes{f().checked ? ` (${f().checked})` : ''}: </span>
                              {f().note ?? (row.from === 'family' ? 'Holds across this chip family.' : LABEL[f().state] + '.')}
                              <Show when={row.from === 'family'}><span class="text-zinc-500"> (across this chip family)</span></Show>
                            </span>
                          )}
                        </Show>
                        <Show when={owner(row.id)?.latest[0]}>
                          {(r) => (
                            <span class="block">
                              <span class="text-zinc-500">Tested ({r().created.slice(0, 10)}): </span>
                              {r().note ?? r().evidence ?? LABEL[r().state] + '.'}
                            </span>
                          )}
                        </Show>
                        <Show when={!row.finding && !owner(row.id)}>
                          <span class="text-zinc-500">Not tested yet, and no notes.</span>
                        </Show>
                      </span>
                      <span class="col-start-2 row-start-1 flex flex-wrap justify-end gap-1.5 sm:col-start-3">
                        <Show when={row.finding} fallback={<Show when={!owner(row.id)}><Pill state="untested" /></Show>}>
                          {(f) => <Pill state={f().state} dim={row.from === 'family'} prefix="Notes" tip="Written by hand, from discussions and reports. Not measured on this page." />}
                        </Show>
                        <Show when={owner(row.id)}>
                          {(o) => (
                            <span class={`${PILL} ${TONE[TONES[ORDER.reduce((a, b) => ((o().states[b] ?? 0) > (o().states[a] ?? 0) ? b : a))]]}`} data-tip={`Found by hw-test on ${o().machines} machine${o().machines === 1 ? '' : 's'}, newest result per machine.`}>
                              Tested: {ORDER.filter((s) => o().states[s]).map((s) => `${o().states[s]} ${LABEL[s].toLowerCase()}`).join(', ')}
                            </span>
                          )}
                        </Show>
                        <Show when={owner(row.id)?.regressed}>
                          {(change) => <span class={`${PILL} ${TONE.wanted}`} data-tip={changeText(change(), 'regressed')}>Regressed</span>}
                        </Show>
                        <Show when={notesFor(row.id).length}>
                          <span class={`${PILL} ${TONE.muted}`} data-tip="Problems, workarounds and pointers written by owners and their agents. Open the check to read them.">
                            {notesFor(row.id).length} {notesFor(row.id).length === 1 ? 'note' : 'notes'}
                          </span>
                        </Show>
                      </span>
                    </summary>
                    <div class="space-y-3 border-t border-zinc-800 bg-zinc-950/40 px-4 py-3 text-sm text-zinc-400">
                      <p><span class="text-zinc-300">Works means:</span> {row.works}</p>
                      <Show when={row.probe}><p><span class="text-zinc-300">The probe measures:</span> {row.probe}</p></Show>
                      <Show when={row.person}><p><span class="text-zinc-300">The person testing checks:</span> {row.person}</p></Show>
                      <Show when={row.debug}><p><span class="text-zinc-300">When it fails, look at:</span> {row.debug}</p></Show>
                      <Show when={row.finding}>
                        {(f) => (
                          <p class="text-xs text-zinc-500">
                            Notes written {[f().version ? `on ${f().version}` : null, f().checked ? `in ${f().checked}` : null].filter(Boolean).join(' ')}
                            <Show when={f().source ?? row.familySource}>{(href) => <>{' · '}<a class={LINK} href={href()}>source</a></>}</Show>
                          </p>
                        )}
                      </Show>
                      <Show when={owner(row.id)?.regressed}>{(change) => <p class="text-rose-300">{changeText(change(), 'regressed')}</p>}</Show>
                      <Show when={owner(row.id)?.fixed}>{(change) => <p class="text-emerald-300">{changeText(change(), 'fixed')}</p>}</Show>
                      <Show when={owner(row.id)}>
                        {(o) => (
                          <ul class="space-y-1.5">
                            <For each={o().latest}>
                              {(r) => (
                                <li class="flex flex-wrap items-baseline gap-2">
                                  <Pill state={r.state} />
                                  <span class="text-xs text-zinc-500">{HOW[r.how] ?? r.how}{r.kernel ? ` · ${r.kernel}` : ''} · {r.created.slice(0, 10)}</span>
                                  <Show when={r.note}><span>{r.note}</span></Show>
                                  <Show when={r.evidence}><code class="rounded bg-zinc-800 px-1 py-0.5 text-xs text-zinc-300">{r.evidence}</code></Show>
                                </li>
                              )}
                            </For>
                          </ul>
                        )}
                      </Show>
                      <Show when={notesFor(row.id).length}>
                        <ul class="space-y-1.5">
                          <For each={notesFor(row.id)}>{noteLine}</For>
                        </ul>
                      </Show>
                      <Show when={fixesFor(row.id).length}>
                        <ul class="space-y-1">
                          <For each={fixesFor(row.id)}>
                            {(fix) => <li>Fix: <a class={LINK} href={fix.url}>{fix.note ?? fix.url}</a></li>}
                          </For>
                        </ul>
                      </Show>
                    </div>
                  </details>
                )}
              </For>
            </div>
          </section>
        )}
      </For>
      <Show when={live()?.notes?.filter((note) => !note.check).length}>
        <section>
          <h3 class="text-sm font-semibold uppercase tracking-wide text-zinc-500">Notes about no single check</h3>
          <ul class="mt-2 space-y-1.5 text-sm text-zinc-400">
            <For each={live()!.notes!.filter((note) => !note.check)}>{noteLine}</For>
          </ul>
        </section>
      </Show>
      <Show when={live()?.fixes.filter((fix) => !fix.check).length}>
        <section>
          <h3 class="text-sm font-semibold uppercase tracking-wide text-zinc-500">Linked fixes</h3>
          <ul class="mt-2 space-y-1 text-sm text-zinc-400">
            <For each={live()!.fixes.filter((fix) => !fix.check)}>
              {(fix) => <li><a class={LINK} href={fix.url}>{fix.note ?? fix.url}</a> <span class="text-xs text-zinc-500">{fix.created.slice(0, 10)}</span></li>}
            </For>
          </ul>
        </section>
      </Show>
    </div>
  );
}
