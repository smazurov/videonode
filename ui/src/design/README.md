# videonode design system

Single source of truth: [`tokens.dtcg.json`](./tokens.dtcg.json) (W3C DTCG 2025.10).
Everything else — `tokens.css`, `tokens.ts`, the `dist-tokens/` portable export — is **generated**.

## Extend or edit a token

1. Edit `tokens.dtcg.json`.
2. Run `pnpm tokens` in `ui/`. This regenerates:
   - `src/design/tokens.css` — CSS custom properties consumed by Tailwind v4 `@theme`.
   - `src/design/tokens.ts` — TS map for non-class consumers (canvas, charts, `/design` swatches).
   - `dist-tokens/videonode-tokens.dtcg.json` — portable DTCG export for downstream tools.
   - `dist-tokens/videonode-tokens.tailwind.js` — convenience Tailwind preset.
3. Commit all generated files together with the source edit.

`pnpm tokens` runs automatically before `pnpm build` (`prebuild` hook).

The current pipeline is a ~80-line zero-dep Node script. Drop-in upgrade path: install `style-dictionary@^4`, replace `scripts/build-tokens.mjs` with a Style Dictionary config, keep `tokens.dtcg.json` unchanged.

## Semantic vocabulary

Use semantic names. Palette names (`slate-800`, `red-500`) are banned by ESLint in `src/components/**`.

### Surfaces (`bg-*`)
- `surface` — default page/panel background
- `surface-muted` — de-emphasized block, skeleton, menu hover
- `surface-raised` — card, menu, dialog panel
- `surface-overlay` — modal scrim

### Foreground (`text-*`)
- `fg` — primary text
- `fg-muted` — secondary/descriptive
- `fg-subtle` — placeholder/tertiary
- `fg-inverse` — text on inverse backgrounds

### Borders / focus
- `border` — default card/divider border
- `border-strong` — higher-contrast input border
- `focus-ring` — focus-visible outline color

### Intent
Each comes as a solid + soft pair. Solid for backgrounds on interactive surfaces; soft for pills/banners.
- `accent` / `accent-hover` / `accent-active` / `accent-fg` / `accent-soft` / `accent-soft-fg`
- `danger` / `danger-hover` / `danger-fg` / `danger-soft` / `danger-soft-fg`
- `warning` / `warning-soft` / `warning-soft-fg`
- `success`, `info`

### Feature / protocol accents
Used for pill badges identifying feature areas:
- `canvas-soft` + `canvas-soft-fg` — canvas composite
- `webrtc-soft` + `webrtc-soft-fg`
- `rtsp-soft` + `rtsp-soft-fg`
- `srt-soft` + `srt-soft-fg`
- `rtmp-soft` + `rtmp-soft-fg`

### Log levels
- `log-error`, `log-warn`, `log-info`, `log-debug` (foreground only)

All tokens resolve per mode via `.dark` selector override — do **not** add `dark:` variants when using semantic tokens.

## Primitives

| When you need…                | Reach for…                                           |
|-------------------------------|------------------------------------------------------|
| Text button, any theme/size   | `Button` (`primary` / `danger` / `light` / `blank`)  |
| Icon-only button              | `IconButton` (same themes, requires `label`)         |
| Pill / status chip            | `Badge` (`tone`, `size`)                             |
| Text input                    | `InputField` (label, error, hint, a11y wired)        |
| Native select                 | `Select` (peer of InputField)                        |
| Multi-value picker            | `MultiSelect` (Headless UI Listbox wrapper)          |
| Checkbox + label + desc/error | `Checkbox` (`useId`-wired label, aria-describedby)   |
| Loading indicator             | `Spinner` (`size`, `tone`, a11y `role="status"`)     |
| Card container                | `Card` + `Card.Header` / `Content` / `Footer`        |
| Bottom sheet / modal          | `BottomSheet` (`title`, `onClose`, `maxWidth`, `headerExtra`) |
| Menu row (inside HL `Menu`)   | `MenuRow` (`icon`, `label`, `onClick`, `variant`)    |

Prefer extending a primitive (new variant, new size) over duplicating JSX.

## Accessibility checklist for new components

- Keyboard-reachable: `tabindex`, reachable focus, logical tab order.
- `focus-visible:ring-2 focus-visible:ring-focus-ring` on every interactive element.
- Label association: `htmlFor` ↔ `id` (or `useId()`), or explicit `aria-label` for icon-only controls.
- `aria-invalid` + `aria-describedby` for error messages.
- Color contrast: verify via axe DevTools or the `/design` review page. Target WCAG AA minimum (4.5:1 body, 3:1 large).
- Semantic tokens only — do not reach past them to raw palette values.

## Review / preview

- **Ladle**: `pnpm ladle` launches the preview at http://localhost:61000. Sidebar lists every primitive (grouped by `title:` slash-hierarchy in its `*.stories.tsx`), plus MDX pages for Overview, Tokens, and Status utilities. Dark-mode toggle lives in the Ladle toolbar and flips the same `.dark` class the app uses.
- **Static export**: `pnpm ladle:build` produces a hostable `ui/build/` directory — the portable artifact for external consumers.
- **External tokens**: drop `dist-tokens/videonode-tokens.dtcg.json` into Penpot or Figma Tokens Studio for a designer-facing view of the same source of truth. (Penpot can't render the React primitives; it's a DTCG editor, not a code-component previewer.)

## Enforcement

ESLint rules in `eslint.config.cjs`:
- `no-restricted-syntax` — fails on raw Tailwind palette classes (`bg-slate-800`, `text-red-500`) in `src/components/**`.
- Same rule bans arbitrary hex colors (`bg-[#abcdef]`).
- `no-restricted-imports` — feature components cannot import heroicons directly; pass the icon via `icon`/`LeadingIcon` props.

Primitives (`Button`, `IconButton`, `Badge`) and the `design/` module are exempt — that's where tokens and icons live.

An explicit **debt allowlist** at the bottom of `eslint.config.cjs` (`DESIGN_SYSTEM_DEBT`) exempts files not yet migrated. **Shrink** this list as files are converted; do not add to it.
