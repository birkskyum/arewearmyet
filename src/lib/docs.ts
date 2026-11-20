// The documentation, in the order and groups the sidebar, the docs overview and llms.txt show it.
// Addresses stay as they were when each page was first linked from elsewhere.
export type DocPage = { href: string; title: string; text: string };
export type DocGroup = { title: string; pages: DocPage[] };

export const DOC_GROUPS: DocGroup[] = [
  {
    title: 'Get started',
    pages: [
      { href: '/docs/install', title: 'Install Omarchy', text: 'Making the stick, installing, and the fixes for known snags at the first start.' },
      { href: '/docs/help', title: 'Help with your laptop', text: 'What to do with the laptop you have, whether it still runs Windows, runs another Linux, or runs Omarchy already.' },
    ],
  },
  {
    title: 'Hardware test',
    pages: [{ href: '/docs/hw-test', title: 'The hardware test', text: 'Running hw-test, what it tests and sends, handing a part to an agent, and every command.' }],
  },
  {
    title: 'Agents and API',
    pages: [
      { href: '/docs/agents', title: 'Guide for agents', text: 'How an agent measures a computer, reports, tries a fix from a boot entry of its own and opens a pull request on Omarchy.' },
      { href: '/docs/api', title: 'API reference', text: 'Every route of the API, with what it takes and what it answers. Everything the pages show can be read from it.' },
    ],
  },
];

/** Whether an address is one of the docs pages, which the top bar shows as Docs. */
export const isDocs = (path: string): boolean => path === '/docs' || DOC_GROUPS.some((group) => group.pages.some((page) => page.href === path));
