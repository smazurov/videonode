import { useSearchParams } from "react-router-dom";
import { WebRTCPlayer } from "../components/webrtc";

export default function VideoRoute() {
  const [searchParams, setSearchParams] = useSearchParams();
  const streamId = searchParams.get("stream");
  const muted = searchParams.get("muted") !== "false";
  const showStats = searchParams.get("stats") === "true";

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
      />
    </div>
  );
}
