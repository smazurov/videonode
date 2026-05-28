import { defineConfig } from 'vitepress';
import { withMermaid } from 'vitepress-plugin-mermaid';

export default withMermaid(
  defineConfig({
    title: 'VideoNode',
    description: 'A hardware-accelerated video streaming daemon for Linux.',
    base: '/videonode/',
    cleanUrls: true,
    lastUpdated: true,
    ignoreDeadLinks: [/^http:\/\/localhost/],

    head: [
      ['link', { rel: 'icon', href: '/videonode/favicon.ico' }],
    ],

    themeConfig: {
      nav: [
        { text: 'Docs', link: '/getting-started/introduction' },
        { text: 'GitHub', link: 'https://github.com/smazurov/videonode' },
      ],

      sidebar: [
        {
          text: 'Getting Started',
          collapsed: false,
          items: [
            { text: 'Introduction', link: '/getting-started/introduction' },
            { text: 'Installation', link: '/getting-started/installation' },
            { text: 'Quickstart', link: '/getting-started/quickstart' },
          ],
        },
        {
          text: 'Reference',
          collapsed: false,
          items: [
            { text: 'config.toml', link: '/reference/config-toml' },
            { text: 'Pipeline model', link: '/reference/pipeline-model' },
            { text: 'REST API', link: '/reference/rest-api' },
          ],
        },
        {
          text: 'Operating',
          collapsed: false,
          items: [
            { text: 'Streaming outputs', link: '/operating/streaming-outputs' },
            { text: 'Encoders', link: '/operating/encoders' },
            { text: 'Observability', link: '/operating/observability' },
          ],
        },
        {
          text: 'Development',
          collapsed: false,
          items: [
            { text: 'Architecture', link: '/development/architecture' },
            { text: 'Building from source', link: '/development/building' },
          ],
        },
      ],

      search: {
        provider: 'local',
      },

      socialLinks: [
        { icon: 'github', link: 'https://github.com/smazurov/videonode' },
      ],

      editLink: {
        pattern: 'https://github.com/smazurov/videonode/edit/main/website/docs/:path',
        text: 'Edit this page on GitHub',
      },

      footer: {
        message: 'Released under the <a href="https://github.com/smazurov/videonode/blob/main/LICENSE">AGPL-3.0 License</a>.',
        copyright: `Copyright © ${new Date().getFullYear()} Stepan Mazurov`,
      },
    },

    mermaid: {
      // Mermaid config — see https://mermaid.js.org/config/setup/modules/mermaidAPI.html
    },
  })
);
