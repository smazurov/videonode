import { useCallback, useEffect } from "react";
import toast from "react-hot-toast";
import { PlayIcon, StopIcon } from "@heroicons/react/24/outline";
import { useShallow } from "zustand/shallow";
import { useStreamStore } from "../hooks/useStreamStore";
import { useSSEManager } from "../hooks/useSSEManager";
import { Button } from "./Button";

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
    try {
      await (pipelineEnabled ? stopPipeline() : startPipeline());
    } catch (error) {
      const message = error instanceof Error ? error.message : "Pipeline toggle failed";
      toast.error(message);
    }
  }, [pipelineEnabled, startPipeline, stopPipeline]);

  if (pipelineEnabled === null) {
    return null;
  }

  return (
    <Button
      text={pipelineEnabled ? "Stop pipeline" : "Start pipeline"}
      theme={pipelineEnabled ? "danger" : "primary"}
      size="SM"
      LeadingIcon={pipelineEnabled ? StopIcon : PlayIcon}
      loading={pipelineToggling}
      disabled={pipelineToggling}
      onClick={handleClick}
    />
  );
}
