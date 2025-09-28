// This will be available after build via the vite-plugin-version-mark
declare global {
  const VIDEONODE_UI_VERSION: string | undefined;
}

export const UI_VERSION: string = typeof VIDEONODE_UI_VERSION !== 'undefined' ? VIDEONODE_UI_VERSION : 'dev';