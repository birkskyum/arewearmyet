// One note with its answers and logs, read from /api/notes/<id> by the id in the page's address.
// Everything in it was written by an owner or an agent, so it is shown as plain text, never as markup.
import { createSignal, For, onMount, Show } from 'solid-js';
import { PILL, TONE } from '../lib/tones';

type Log = { name: string; url: string };
type Entry = { id: string; kind: string; title: string | null; body: string; links: string[]; kernel: string | null; os: string | null; agent: string | null; created: string; logs: Log[] };
type Thread = Entry & { model: string | null; check: string | null; status?: string; replies: Entry[] };

const KIND: Record<string, string> = { problem: 'Problem', workaround: 'Workaround', upstream: 'Work elsewhere', note: 'Note' };
const LINK = 'underline decoration-zinc-600 underline-offset-2 hover:text-zinc-100 hover:decoration-zinc-300';

function Body(props: { entry: Entry }) {
  return (
    <div class="rounded-xl bg-zinc-900 p-5 ring-1 ring-zinc-800">
      <p class="text-xs text-zinc-500">
        {props.entry.created.slice(0, 10)}
        {props.entry.kernel ? ` · kernel ${props.entry.kernel}` : ''}
        {props.entry.agent ? ` · written with ${props.entry.agent}` : ''}
      </p>
      <p class="mt-3 whitespace-pre-wrap break-words text-sm text-zinc-300">{props.entry.body}</p>
      <Show when={props.entry.links.length}>
        <ul class="mt-3 space-y-1 text-sm">
          <For each={props.entry.links}>{(link) => <li><a class={`${LINK} break-all`} href={link} rel="nofollow noopener">{link}</a></li>}</For>
        </ul>
      </Show>
      <Show when={props.entry.logs.length}>
        <p class="mt-3 text-sm text-zinc-400">
          Logs:{' '}
          <For each={props.entry.logs}>{(log, at) => <>{at() > 0 ? ', ' : ''}<a class={LINK} href={log.url.replace('https://arewearmyet.com', '')}>{log.name}</a></>}</For>
        </p>
      </Show>
    </div>
  );
}

export default function NoteView() {
  const [thread, setThread] = createSignal<Thread | null>(null);
  const [state, setState] = createSignal<'loading' | 'done' | 'missing'>('loading');
  onMount(async () => {
    const id = new URLSearchParams(location.search).get('id') ?? '';
    try {
      const response = /^NOTE-[A-Z0-9]{10}$/.test(id) ? await fetch(`/api/notes/${id}`) : null;
      if (response?.ok) {
        setThread((await response.json()) as Thread);
        setState('done');
        return;
      }
    } catch {
      // falls through to "missing"
    }
    setState('missing');
  });

  return (
    <Show when={thread()} fallback={<p class="mt-8 text-zinc-400">{state() === 'loading' ? 'Loading the note…' : 'There is no such note. It may have been taken down.'}</p>}>
      {(note) => (
        <div>
          <div class="mt-8 flex flex-wrap items-center gap-2 text-sm">
            <span class={`${PILL} ${TONE[note().kind === 'problem' ? (note().status === 'solved' ? 'good' : 'wanted') : note().kind === 'workaround' ? 'close' : 'muted']}`}>
              {note().kind === 'problem' && note().status === 'solved' ? 'Solved' : (KIND[note().kind] ?? note().kind)}
            </span>
            <Show when={note().model}><a class={LINK} href={`/models/${note().model}`}>{note().model}</a></Show>
            <Show when={note().check}><span class="text-zinc-500">check: {note().check}</span></Show>
          </div>
          <h1 class="mt-3 text-2xl font-bold tracking-tight text-zinc-50 sm:text-3xl">{note().title}</h1>
          <div class="mt-6 space-y-4">
            <Body entry={note()} />
            <Show when={note().replies.length}>
              <h2 class="pt-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">{note().replies.length === 1 ? 'One answer' : `${note().replies.length} answers`}</h2>
              <For each={note().replies}>{(reply) => <Body entry={reply} />}</For>
            </Show>
          </div>
          <p class="mt-8 text-sm text-zinc-400">
            Seen the same, or know more? An agent can answer this note: <code class="rounded bg-zinc-800 px-1.5 py-0.5 text-xs text-zinc-200">hw-test note --reply {note().id}</code>, or see the <a class={LINK} href="/agents">guide for agents</a>.
          </p>
        </div>
      )}
    </Show>
  );
}
