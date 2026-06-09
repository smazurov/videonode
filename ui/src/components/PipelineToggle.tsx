import { useCallback, useEffect } from "react";
import toast from "react-hot-toast";
import { useShallow } from "zustand/shallow";
import { useStreamStore } from "../hooks/useStreamStore";
import { useSSEManager } from "../hooks/useSSEManager";
import { cn } from "../utils";
import { HexLogo, type HexTone } from "./HexLogo";

// PipelineToggle is the VideoNode brand mark, doubling as the daemon-wide
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

  useEffect(() => {
    if (pipelineEnabled === null) return;
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (link) link.href = pipelineEnabled ? "/favicon.ico" : "/favicon-red.ico";
  }, [pipelineEnabled]);

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
  let tone: HexTone;
  if (pipelineEnabled === null) {
    label = "Pipeline state unknown";
    tone = "gray";
  } else if (pipelineEnabled) {
    label = "Stop pipeline";
    tone = "blue";
  } else {
    label = "Start pipeline";
    tone = "red";
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      disabled={pipelineEnabled === null || pipelineToggling}
      title={label}
      aria-label={label}
      className={cn(
        "group outline-none rounded-md",
        "focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-focus-ring focus-visible:ring-offset-surface",
        pipelineToggling ? "opacity-70 cursor-wait" : undefined,
      )}
    >
      <div className="relative w-8 h-8 transition-transform duration-200 group-hover:scale-105">
        <HexLogo
          tone={tone}
          title={label}
          className="w-8 h-8 rounded-md transition-[filter] duration-200 group-hover:brightness-110"
        />
        {pipelineToggling ? (
          <span className="absolute inset-0 flex items-center justify-center">
            <span className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
          </span>
        ) : null}
      </div>
    </button>
  );
}
