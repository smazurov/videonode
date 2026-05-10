import { WebRTCPlayer } from './webrtc';

interface PerspectivePreviewProps {
  streamId: string;
}

export function PerspectivePreview({ streamId }: Readonly<PerspectivePreviewProps>) {
  return (
    <div className="rounded-lg overflow-hidden bg-black">
      <WebRTCPlayer streamId={streamId} className="w-full" muted />
    </div>
  );
}
