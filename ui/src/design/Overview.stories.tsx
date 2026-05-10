import type { Story, StoryDefault } from "@ladle/react";

export default {
  title: "Design System/Overview",
} satisfies StoryDefault;

export const Readme: Story = () => (
  <div className="max-w-3xl space-y-4 text-fg">
    <h1 className="text-2xl font-bold">videonode design system</h1>
    <p className="text-fg-muted">
      Primitives, tokens, and guidance — all sourced from a single artifact.
    </p>

    <h2 className="text-xl font-semibold pt-2">Sources of truth</h2>
    <table className="border-separate border-spacing-x-4">
      <thead>
        <tr className="text-left text-sm text-fg-muted">
          <th>Layer</th>
          <th>Source</th>
        </tr>
      </thead>
      <tbody className="text-sm">
        <tr>
          <td>Color tokens</td>
          <td>
            <code className="font-mono">src/design/tokens.dtcg.json</code> (W3C DTCG)
          </td>
        </tr>
        <tr>
          <td>Generated CSS</td>
          <td>
            <code className="font-mono">src/design/tokens.css</code> (via <code>pnpm tokens</code>)
          </td>
        </tr>
        <tr>
          <td>Generated TS</td>
          <td>
            <code className="font-mono">src/design/tokens.ts</code>
          </td>
        </tr>
        <tr>
          <td>Primitive APIs</td>
          <td>
            <code className="font-mono">src/components/*.tsx</code>
          </td>
        </tr>
        <tr>
          <td>Primitive docs</td>
          <td>
            <code className="font-mono">src/components/*.stories.tsx</code> (this site)
          </td>
        </tr>
        <tr>
          <td>Status maps</td>
          <td>
            <code className="font-mono">src/design/status.ts</code>
          </td>
        </tr>
      </tbody>
    </table>

    <h2 className="text-xl font-semibold pt-2">How to add a primitive</h2>
    <ol className="list-decimal pl-6 space-y-2 text-sm">
      <li>
        Create <code className="font-mono">src/components/&lt;Name&gt;.tsx</code> using semantic tokens
        (<code>bg-surface</code>, <code>text-fg</code>, <code>border-danger</code>). The lint gate rejects
        raw palette classes like <code>bg-slate-800</code>.
      </li>
      <li>
        Create <code className="font-mono">src/components/&lt;Name&gt;.stories.tsx</code> in CSF 3 format.
        Use a slash-hierarchy title like <code>"Forms/YourName"</code> so it groups in the sidebar. One export
        per meaningful variant or state.
      </li>
      <li>
        Reuse blocks in <code className="font-mono">src/design/blocks/</code>:{" "}
        <code>&lt;VariantMatrix&gt;</code>, <code>&lt;StateRow&gt;</code>, <code>&lt;TokenGrid&gt;</code>.
      </li>
    </ol>

    <h2 className="text-xl font-semibold pt-2">How to add a token</h2>
    <ol className="list-decimal pl-6 space-y-2 text-sm">
      <li>
        Edit <code className="font-mono">src/design/tokens.dtcg.json</code> — add to{" "}
        <code>semantic.color</code> (preferred) or <code>primitive.color</code>.
      </li>
      <li>
        Run <code>pnpm tokens</code> to regenerate the CSS, TS, and portable DTCG export.
      </li>
      <li>Commit the generated files alongside the source edit.</li>
    </ol>

    <h2 className="text-xl font-semibold pt-2">Accessibility</h2>
    <ul className="list-disc pl-6 space-y-1 text-sm">
      <li>Keyboard-reachable: logical tab order.</li>
      <li>
        <code>focus-visible:ring-2 focus-visible:ring-focus-ring</code> on every interactive element.
      </li>
      <li>
        Label association: <code>htmlFor</code> ↔ <code>id</code> (use <code>useId()</code>), or explicit{" "}
        <code>aria-label</code> for icon-only controls.
      </li>
      <li>
        <code>aria-invalid</code> + <code>aria-describedby</code> for errors.
      </li>
      <li>Color contrast: WCAG AA minimum (Ladle's a11y addon flags violations automatically).</li>
    </ul>
  </div>
);
