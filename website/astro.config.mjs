import { defineConfig } from 'astro/config';
import node from '@astrojs/node';
import rehypeSlug from 'rehype-slug';
import rehypeAutolinkHeadings from 'rehype-autolink-headings';

export default defineConfig({
  site: 'https://bootimus.com',
  output: 'server',
  adapter: node({ mode: 'standalone' }),

  i18n: {
    defaultLocale: 'en',
    locales: ['en', 'de', 'fr', 'es', 'ru'],
    routing: {
      prefixDefaultLocale: false,
      redirectToDefaultLocale: false,
    },
  },

  server: {
    host: '0.0.0.0',
    port: Number(process.env.PORT) || 3000,
  },

  compressHTML: true,

  build: {
    inlineStylesheets: 'auto',
  },

  markdown: {
    shikiConfig: {
      themes: { light: 'vitesse-light', dark: 'vitesse-dark' },
      defaultColor: false,
      wrap: true,
    },
    rehypePlugins: [
      rehypeSlug,
      [
        rehypeAutolinkHeadings,
        {
          behavior: 'append',
          properties: { className: ['heading-anchor'], 'aria-label': 'Permalink' },
          content: { type: 'text', value: '#' },
        },
      ],
    ],
  },
});
