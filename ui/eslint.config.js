import { defineConfig } from "eslint/config";
import js from "@eslint/js";
import typescriptEslint from "@typescript-eslint/eslint-plugin";
import tsParser from "@typescript-eslint/parser";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import eslintReact from "@eslint-react/eslint-plugin";
import security from "eslint-plugin-security";
import unicorn from "eslint-plugin-unicorn";
import sonarjs from "eslint-plugin-sonarjs";
import storeSelectors from "eslint-plugin-zustand-store-selectors";
import globals from "globals";

// Common rules shared between JS and TS
const commonRules = {
  // React hooks rules
  ...reactHooks.configs.recommended.rules,

  // Security rules
  ...security.configs.recommended.rules,

  // SonarJS rules
  ...sonarjs.configs.recommended.rules,

  // React refresh rules
  "react-refresh/only-export-components": [
    "warn",
    {
      allowConstantExport: true,
    },
  ],

  // Security rules
  "security/detect-object-injection": "off",
  "security/detect-non-literal-regexp": "warn",
  "security/detect-unsafe-regex": "error",

  // Unicorn rules for code quality
  "unicorn/better-regex": "error",
  "unicorn/catch-error-name": "error",
  "unicorn/consistent-destructuring": "error",
  "unicorn/no-array-for-each": "warn",
  "unicorn/no-console-spaces": "error",
  "unicorn/no-for-loop": "error",
  "unicorn/prefer-includes": "error",
  "unicorn/prefer-string-starts-ends-with": "error",
  "unicorn/prefer-ternary": "error",
  "unicorn/no-await-expression-member": "error",
  "unicorn/no-empty-file": "error",
  "unicorn/no-abusive-eslint-disable": "error",

  // Zustand: require selector functions when reading from the store.
  // (Sole rule with an ESLint 10 published home; the other zustand-rules
  // rules had no maintained es10 package — see eslint migration notes.)
  "zustand-store-selectors/use-store-selectors": "error",

  // SonarJS specific overrides
  "sonarjs/cognitive-complexity": ["error", 15],
  "sonarjs/no-duplicate-string": "error",
  "sonarjs/no-identical-functions": "error",
  "sonarjs/prefer-immediate-return": "error",
};

// Design-system enforcement: ban raw Tailwind palette classes (e.g. bg-slate-800) and
// arbitrary-value hex colors in component files. Forces use of semantic tokens defined
// in src/design/tokens.dtcg.json (bg-surface, text-fg, border-danger, etc.).
// See ui/src/design/README.md for the allowed vocabulary.
const TAILWIND_PALETTE_PATTERN = String.raw`\b(bg|text|border|ring|from|to|via|divide|placeholder|fill|stroke|outline|decoration|accent|caret|shadow)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b`;
const HEX_COLOR_ARBITRARY_PATTERN = String.raw`\[#[0-9a-fA-F]{3,8}\]`;

// Bare color *value* literals (not class strings) — e.g. fill="#ef4444",
// stroke: "rgba(0,0,0,0.5)" passed to Konva/canvas props or inline styles.
// Anchored hex avoids matching DOM ids / arbitrary-value class strings (those
// are caught by HEX_COLOR_ARBITRARY_PATTERN).
const RAW_HEX_COLOR_PATTERN = String.raw`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`;
const FUNCTIONAL_COLOR_PATTERN = String.raw`(?:rgba?|hsla?)\(`;
const RAW_COLOR_MESSAGE =
  "Use a design-system token instead of a hardcoded color. For DOM, use a semantic Tailwind class (bg-surface, text-fg). For canvas/Konva, resolve the token at runtime via getComputedStyle(document.documentElement).getPropertyValue('--color-…') with a semanticTokens fallback — see src/components/composers/KonvaCanvasEditor.tsx (readPalette).";

const designSystemRules = {
  "no-restricted-syntax": [
    "error",
    {
      selector: `Literal[value=/${TAILWIND_PALETTE_PATTERN}/]`,
      message:
        "Use a semantic token (e.g. bg-surface, text-fg, border-danger) instead of a raw Tailwind palette class. See ui/src/design/README.md.",
    },
    {
      selector: `TemplateElement[value.raw=/${TAILWIND_PALETTE_PATTERN}/]`,
      message:
        "Use a semantic token (e.g. bg-surface, text-fg, border-danger) instead of a raw Tailwind palette class. See ui/src/design/README.md.",
    },
    {
      selector: `Literal[value=/${HEX_COLOR_ARBITRARY_PATTERN}/]`,
      message:
        "Do not use arbitrary hex colors in class strings; add the value to tokens.dtcg.json and reference it as a semantic token.",
    },
    {
      selector: `TemplateElement[value.raw=/${HEX_COLOR_ARBITRARY_PATTERN}/]`,
      message:
        "Do not use arbitrary hex colors in class strings; add the value to tokens.dtcg.json and reference it as a semantic token.",
    },
    {
      selector: `Literal[value=/${RAW_HEX_COLOR_PATTERN}/]`,
      message: RAW_COLOR_MESSAGE,
    },
    {
      selector: `TemplateElement[value.raw=/${RAW_HEX_COLOR_PATTERN}/]`,
      message: RAW_COLOR_MESSAGE,
    },
    {
      selector: `Literal[value=/${FUNCTIONAL_COLOR_PATTERN}/]`,
      message: RAW_COLOR_MESSAGE,
    },
    {
      selector: `TemplateElement[value.raw=/${FUNCTIONAL_COLOR_PATTERN}/]`,
      message: RAW_COLOR_MESSAGE,
    },
  ],
  // heroicons import restriction intentionally lifted: icons here are intrinsic
  // component vocabulary (action glyphs, chrome, status/empty states), there is
  // no central icon barrel, and routes/** import heroicons freely. Routing all
  // call sites through caller props would be awkward prop-drilling. The
  // icon-via-prop pattern stays available/encouraged for primitives but is no
  // longer lint-enforced. See ui/src/design/README.md.
  "no-restricted-imports": "off",
};

// Files that still contain raw palette/color literals and are tracked as
// migration debt. Remove entries here as they are migrated. Do not grow this list.
// Keep this constant so regressions can be pinned explicitly rather than re-growing allowlists.
const DESIGN_SYSTEM_DEBT = [];

// Typed-API enforcement: the openapi-fetch `api` client (src/lib/api.ts) is the
// only sanctioned way to talk to the HTTP API. Raw `fetch(...)` and the
// half-typed `fetchWithTimeout(...)` are banned everywhere except the
// allowlisted raw layer src/lib/api_fetch.ts.
const TYPED_API_MESSAGE =
  "Use the typed 'api' client (src/lib/api.ts). For a genuinely raw request, add a wrapper in src/lib/api_fetch.ts.";
const RAW_FETCH_SELECTORS = [
  {
    selector: "CallExpression[callee.name='fetch']",
    message: TYPED_API_MESSAGE,
  },
  {
    selector: "CallExpression[callee.type='MemberExpression'][callee.property.name='fetch']",
    message: TYPED_API_MESSAGE,
  },
  {
    selector: "CallExpression[callee.name='fetchWithTimeout']",
    message: TYPED_API_MESSAGE,
  },
];

// The allowlist file: the single place where raw requests are permitted.
const API_FETCH_ALLOWLIST = "src/lib/api_fetch.ts";

export default defineConfig([
  {
    name: "videonode/ignores",
    ignores: ["**/dist", "**/build", "**/node_modules", "**/api.generated.ts"],
  },
  // Base JavaScript recommended rules
  js.configs.recommended,
  // Modern React rules (@eslint-react) for TS/TSX, with the rules that overlap
  // eslint-plugin-react-hooks disabled so the official hooks plugin owns them.
  {
    name: "videonode/eslint-react/recommended-typescript",
    files: ["**/*.{ts,tsx}"],
    ...eslintReact.configs["recommended-typescript"],
  },
  {
    name: "videonode/eslint-react/disable-hooks-conflict",
    files: ["**/*.{ts,tsx}"],
    ...eslintReact.configs["disable-conflict-eslint-plugin-react-hooks"],
  },
  // TypeScript files configuration
  {
    name: "videonode/typescript",
    files: ["**/*.{ts,tsx}"],
    plugins: {
      "@typescript-eslint": typescriptEslint,
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
      security: security,
      unicorn: unicorn,
      sonarjs: sonarjs,
      "zustand-store-selectors": storeSelectors,
    },

    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.es2020,
      },

      parser: tsParser,
      ecmaVersion: "latest",
      sourceType: "module",

      parserOptions: {
        ecmaFeatures: {
          jsx: true,
        },
        project: "./tsconfig.json",
        tsconfigRootDir: import.meta.dirname,
      },
    },

    rules: {
      // Common rules
      ...commonRules,

      // TypeScript ESLint recommended rules
      ...typescriptEslint.configs.recommended.rules,

      // Custom TypeScript rules
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          argsIgnorePattern: "^_",
        },
      ],
      "@typescript-eslint/prefer-readonly": "error",
      "@typescript-eslint/no-unsafe-argument": "error",
      "@typescript-eslint/no-unsafe-assignment": "error",
      "@typescript-eslint/no-unsafe-call": "error",
      "@typescript-eslint/no-unsafe-member-access": "error",
      "@typescript-eslint/no-unsafe-return": "error",

      // Disable conflicting rules
      "no-undef": "off", // TypeScript handles this
      "no-unused-vars": "off", // Use @typescript-eslint/no-unused-vars instead
    },
  },
  // Typed-API enforcement: ban raw fetch / fetchWithTimeout outside the
  // allowlisted raw layer. Routes everything through the typed `api` client.
  {
    name: "videonode/no-raw-fetch",
    files: ["src/**/*.{ts,tsx}"],
    rules: {
      "no-restricted-syntax": ["error", ...RAW_FETCH_SELECTORS],
    },
  },
  // Design-system enforcement: semantic tokens only in component files. Folds
  // in the raw-fetch ban so component files keep both restrictions (a later
  // block's no-restricted-syntax fully replaces an earlier one).
  {
    name: "videonode/design-system",
    files: ["src/components/**/*.{ts,tsx}"],
    ignores: [
      // Primitives and the design module are the source of truth for tokens.
      "src/components/Button.tsx",
      "src/components/IconButton.tsx",
      "src/components/Badge.tsx",
      // Brand mark: literal hues are intrinsic to the logo, not themeable.
      "src/components/HexLogo.tsx",
      "src/design/**",
      ...DESIGN_SYSTEM_DEBT,
    ],
    rules: {
      ...designSystemRules,
      "no-restricted-syntax": [
        "error",
        ...designSystemRules["no-restricted-syntax"].slice(1),
        ...RAW_FETCH_SELECTORS,
      ],
    },
  },
  // Allowlist: api_fetch.ts is the single sanctioned home for raw requests.
  {
    name: "videonode/api-fetch-allowlist",
    files: [API_FETCH_ALLOWLIST],
    rules: {
      "no-restricted-syntax": "off",
    },
  },
]);
