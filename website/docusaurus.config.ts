import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'VideoNode',
  tagline: 'A hardware-accelerated video streaming daemon for Linux',
  favicon: 'img/favicon.ico',

  future: {
    v4: true,
  },

  url: 'https://mazurov.dev',
  baseUrl: '/videonode/',

  organizationName: 'smazurov',
  projectName: 'videonode',

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          routeBasePath: '/',
          editUrl:
            'https://github.com/smazurov/videonode/tree/main/website/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'VideoNode',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs',
        },
        {
          href: 'https://github.com/smazurov/videonode',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Project',
          items: [
            {label: 'GitHub', href: 'https://github.com/smazurov/videonode'},
            {label: 'Releases', href: 'https://github.com/smazurov/videonode/releases'},
            {label: 'Issues', href: 'https://github.com/smazurov/videonode/issues'},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Stepan Mazurov. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['toml', 'bash', 'go', 'protobuf', 'json', 'yaml'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
