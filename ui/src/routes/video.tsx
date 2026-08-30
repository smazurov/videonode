import { useEffect, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import { WebRTCPlayer } from "../components/webrtc";
import { parseCanvasSize, parseClip } from "../components/composers/canvas-mask";

export default function VideoRoute() {
  const [searchParams, setSearchParams] = useSearchParams();
  const streamId = searchParams.get("stream");
  const muted = searchParams.get("muted") !== "false";
  const showStats = searchParams.get("stats") === "true";
  const clipParam = searchParams.get("clip");
  const canvasParam = searchParams.get("canvas");

  const maskRects = useMemo(() => parseClip(clipParam), [clipParam]);
  const maskCanvas = useMemo(() => parseCanvasSize(canvasParam), [canvasParam]);
  const masking = maskRects.length > 0 && maskCanvas !== null;

  // A masked player composites with alpha, which only reaches an OBS browser
  // source if nothing behind it paints — including the app's own background.
  useEffect(() => {
    if (!masking) return;
    const { documentElement, body } = document;
    const prevRoot = documentElement.style.background;
    const prevBody = body.style.background;
    documentElement.style.background = "transparent";
    body.style.background = "transparent";
    return () => {
      documentElement.style.background = prevRoot;
      body.style.background = prevBody;
    };
  }, [masking]);

  const toggleStats = () => {
    setSearchParams((prev) => {
      if (prev.get("stats") === "true") {
        prev.delete("stats");
      } else {
        prev.set("stats", "true");
      }
      return prev;
    });
  };

  if (!streamId) {
    return (
      <div className="w-screen h-screen bg-black flex items-center justify-center">
        <p className="text-gray-400">Missing stream parameter. Use /video?stream=&lt;id&gt;</p>
      </div>
    );
  }

  return (
    <div className="w-screen h-screen" onDoubleClick={toggleStats}>
      <WebRTCPlayer
        streamId={streamId}
        className="w-full h-full"
        muted={muted}
        showStats={showStats}
        {...(masking && maskCanvas ? { maskRects, maskCanvas } : {})}
      />
    </div>
  );
}
