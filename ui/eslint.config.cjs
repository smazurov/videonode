const js = require("@eslint/js");
const typescriptEslint = require("@typescript-eslint/eslint-plugin");
const tsParser = require("@typescript-eslint/parser");
const reactHooks = require("eslint-plugin-react-hooks");
const reactRefresh = require("eslint-plugin-react-refresh");
const react = require("eslint-plugin-react");
const importPlugin = require("eslint-plugin-import");
const security = require("eslint-plugin-security");
const unicorn = require("eslint-plugin-unicorn").default;
const sonarjs = require("eslint-plugin-sonarjs");
const zustandRules = require("eslint-plugin-zustand-rules");
const globals = require("globals");

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
  
  // React rules
  "react/react-in-jsx-scope": "off", // Not needed in React 17+
  "react/jsx-uses-react": "off", // Not needed in React 17+
  
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
  
  // Zustand rules
  "zustand-rules/enforce-slices-when-large-state": ["warn", { maxProperties: 10 }],
  "zustand-rules/use-store-selectors": "error",
  "zustand-rules/no-state-mutation": "error",
  "zustand-rules/enforce-use-setstate": "error",
  // "zustand-rules/enforce-state-before-actions": "error", // Disabled: plugin has bugs
  "zustand-rules/no-multiple-stores": "error",
  
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
};

// Files that still contain raw palette classes and are tracked as migration debt.
// Remove entries here as they are migrated. Do not grow this list.
// Empty — all component files have been migrated to semantic tokens.
// Keep this constant so future regressions can be pinned explicitly rather than re-growing allowlists.
const DESIGN_SYSTEM_DEBT = [];

module.exports = [
  {
    ignores: ["**/dist", "**/build", "**/node_modules", "**/api.generated.ts"],
  },
  // Base JavaScript recommended rules
  js.configs.recommended,
  // TypeScript files configuration
  {
    files: ["**/*.{ts,tsx}"],
    plugins: {
      "@typescript-eslint": typescriptEslint,
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
      react: react,
      import: importPlugin,
      security: security,
      unicorn: unicorn,
      sonarjs: sonarjs,
      "zustand-rules": zustandRules,
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
        tsconfigRootDir: __dirname,
      },
    },

    settings: {
      react: {
        version: "detect",
      },
      "import/resolver": {
        typescript: {
          alwaysTryTypes: true,
          project: "./tsconfig.json",
        },
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
  // JavaScript files configuration
  {
    files: ["**/*.{js,jsx}"],
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
      react: react,
      import: importPlugin,
      security: security,
      unicorn: unicorn,
      sonarjs: sonarjs,
      "zustand-rules": zustandRules,
    },

    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.es2020,
      },

      ecmaVersion: "latest",
      sourceType: "module",

      parserOptions: {
        ecmaFeatures: {
          jsx: true,
        },
      },
    },

    settings: {
      react: {
        version: "detect",
      },
    },

    rules: {
      // Common rules
      ...commonRules,
    },
  },
];