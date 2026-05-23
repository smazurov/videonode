import { useCallback, useEffect } from "react";
import toast from "react-hot-toast";
import { useShallow } from "zustand/shallow";
import { useStreamStore } from "../hooks/useStreamStore";
import { useSSEManager } from "../hooks/useSSEManager";
import { cn } from "../utils";

// PipelineToggle is the "VN" brand mark, doubling as the daemon-wide
// start/stop button. Blue = running, red = stopped, gray = unknown.
export function PipelineToggle() {
  const { pipelineEnabled, pipelineToggling, fetchPipelineState, startPipeline, stopPipeline, setPipelineEnabled } =
    useStreamStore(
      useShallow((state) => ({
        pipelineEnabled: state.pipelineEnabled,
        pipelineToggling: state.pipelineToggling,
        fetchPipelineState: state.fetchPipelineState,
        startPipeline: state.startPipeline,
        stopPipeline: state.stopPipeline,
        setPipelineEnabled: state.setPipelineEnabled,
      })),
    );

  useEffect(() => {
    if (pipelineEnabled === null) {
      fetchPipelineState();
    }
  }, [pipelineEnabled, fetchPipelineState]);

  const handlePipelineEvent = useCallback(
    (event: { enabled: boolean }) => {
      setPipelineEnabled(event.enabled);
    },
    [setPipelineEnabled],
  );

  useSSEManager({ onPipelineStateEvent: handlePipelineEvent });

  const handleClick = useCallback(async () => {
    if (pipelineEnabled === null) return;
    try {
      await (pipelineEnabled ? stopPipeline() : startPipeline());
    } catch (error) {
      const message = error instanceof Error ? error.message : "Pipeline toggle failed";
      toast.error(message);
    }
  }, [pipelineEnabled, startPipeline, stopPipeline]);

  let label: string;
  let bg: string;
  if (pipelineEnabled === null) {
    label = "Pipeline state unknown";
    bg = "bg-surface-muted";
  } else if (pipelineEnabled) {
    label = "Stop pipeline";
    bg = "bg-accent group-hover:bg-accent-hover";
  } else {
    label = "Start pipeline";
    bg = "bg-danger group-hover:bg-danger-hover";
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      disabled={pipelineEnabled === null || pipelineToggling}
      title={label}
      aria-label={label}
      className={cn(
        "group outline-none rounded-sm",
        "focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-focus-ring focus-visible:ring-offset-surface",
        pipelineToggling ? "opacity-70 cursor-wait" : undefined,
      )}
    >
      <div
        className={cn(
          "w-8 h-8 rounded-sm flex items-center justify-center transition-colors duration-200",
          bg,
        )}
      >
        {pipelineToggling ? (
          <div className="w-4 h-4 border-2 border-accent-fg border-t-transparent rounded-full animate-spin" />
        ) : (
          <span className="text-accent-fg font-bold text-sm">VN</span>
        )}
      </div>
    </button>
  );
}
