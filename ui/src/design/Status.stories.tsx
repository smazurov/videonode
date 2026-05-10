import type { Story, StoryDefault } from "@ladle/react";
import { logLevelClasses, connectionStatusClasses } from "./status";

export default {
  title: "Design System/Status utilities",
} satisfies StoryDefault;

export const LogLevelClasses: Story = () => (
  <div className="space-y-2 max-w-2xl">
    <p className="text-sm text-fg-muted">
      <code className="font-mono">logLevelClasses(level)</code> maps log-level strings to the right
      semantic foreground token.
    </p>
    <div className="space-y-1 font-mono text-sm">
      {(["error", "warn", "info", "debug"] as const).map((lvl) => (
        <div key={lvl} className={logLevelClasses(lvl)}>
          [{lvl.toUpperCase()}] example log message
        </div>
      ))}
    </div>
  </div>
);

export const ConnectionStatus: Story = () => (
  <div className="space-y-2 max-w-2xl">
    <p className="text-sm text-fg-muted">
      <code className="font-mono">connectionStatusClasses(state)</code> returns a background token plus
      any required animation.
    </p>
    <div className="flex flex-wrap items-center gap-4">
      {(["connected", "connecting", "disconnected"] as const).map((state) => (
        <div key={state} className="flex items-center gap-2 text-xs">
          <div className={`w-2 h-2 rounded-full ${connectionStatusClasses(state)}`} />
          <span className="text-fg-muted font-mono">{state}</span>
        </div>
      ))}
    </div>
  </div>
);
