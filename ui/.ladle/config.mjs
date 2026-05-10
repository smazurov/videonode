/** @type {import('@ladle/react').UserConfig} */
export default {
  stories: "src/**/*.stories.{js,jsx,ts,tsx}",
  port: 61000,
  viteConfig: ".ladle/vite.config.mjs",
  defaultStory: "design-system--overview--readme",
  addons: {
    a11y: { enabled: true },
    theme: {
      enabled: true,
      defaultState: "light",
    },
    width: { enabled: false },
    rtl: { enabled: false },
    ladle: { enabled: false },
  },
};
