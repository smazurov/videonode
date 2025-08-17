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
const globals = require("globals");

// Workaround for zustand-rules plugin - manually load rules
const zustandRules = {
  rules: {
    "enforce-slices-when-large-state": require("eslint-plugin-zustand-rules/lib/rules/enforce-slices-when-large-state"),
    "use-store-selectors": require("eslint-plugin-zustand-rules/lib/rules/use-store-selectors"),
    "no-state-mutation": require("eslint-plugin-zustand-rules/lib/rules/no-state-mutation"),
    "enforce-use-setstate": require("eslint-plugin-zustand-rules/lib/rules/enforce-use-setstate"),
    "enforce-state-before-actions": require("eslint-plugin-zustand-rules/lib/rules/enforce-state-before-actions"),
    "no-multiple-stores": require("eslint-plugin-zustand-rules/lib/rules/no-multiple-stores"),
  }
};

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

module.exports = [
  {
    ignores: ["**/dist", "**/build", "**/node_modules"],
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