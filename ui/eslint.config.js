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
  ],
  // Feature components must not import heroicons directly; pass the icon via a
  // primitive's `icon`/`LeadingIcon` prop. Primitives (Button/IconButton/Badge)
  // and the design/ module are exempt via the block's `ignores`. See README.md.
  "no-restricted-imports": [
    "error",
    {
      patterns: [
        {
          group: ["@heroicons/react", "@heroicons/react/**"],
          message:
            "Do not import heroicons directly in feature components; pass the icon via a primitive's `icon`/`LeadingIcon` prop. See ui/src/design/README.md.",
        },
      ],
    },
  ],
};

// Files that still contain raw palette classes and are tracked as migration debt.
// Remove entries here as they are migrated. Do not grow this list.
// Empty — all component files have been migrated to semantic tokens.
// Keep this constant so future regressions can be pinned explicitly rather than re-growing allowlists.
const DESIGN_SYSTEM_DEBT = [];

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
  // Design-system enforcement: semantic tokens only in component files.
  {
    name: "videonode/design-system",
    files: ["src/components/**/*.{ts,tsx}"],
    ignores: [
      // Primitives and the design module are the source of truth for tokens.
      "src/components/Button.tsx",
      "src/components/IconButton.tsx",
      "src/components/Badge.tsx",
      "src/design/**",
      ...DESIGN_SYSTEM_DEBT,
    ],
    rules: designSystemRules,
  },
]);
