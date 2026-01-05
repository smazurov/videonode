import { useSearchParams } from "react-router-dom";
import { WebRTCPlayer } from "../components/webrtc";

export default function VideoRoute() {
  const [searchParams] = useSearchParams();
  const streamId = searchParams.get("stream");
  const muted = searchParams.get("muted") !== "false";
  const showStats = searchParams.get("stats") === "true";

  if (!streamId) {
    return (
      <div className="w-screen h-screen bg-black flex items-center justify-center">
        <p className="text-gray-400">Missing stream parameter. Use /video?stream=&lt;id&gt;</p>
      </div>
    );
  }

  return (
    <WebRTCPlayer
      streamId={streamId}
      className="w-screen h-screen"
      muted={muted}
      showStats={showStats}
    />
  );
}
