// @ts-check
import { defineConfig } from 'astro/config';

import tailwindcss from '@tailwindcss/vite';

import solidJs from '@astrojs/solid-js';
import { rehypeHeadingIds } from '@astrojs/markdown-remark';
import rehypeAutolinkHeadings from 'rehype-autolink-headings';

// https://astro.build/config
export default defineConfig({
  site: 'https://arewearmyet.com',
  vite: {
    plugins: [tailwindcss()]
  },

  integrations: [solidJs()],

  // Every heading of a Markdown page ends in a link to itself, so its address can be copied
  markdown: {
    rehypePlugins: [
      rehypeHeadingIds,
      [rehypeAutolinkHeadings, { behavior: 'append', properties: { class: 'anchor', ariaLabel: 'Link to this section' }, content: { type: 'text', value: '#' } }],
    ],
  },
});