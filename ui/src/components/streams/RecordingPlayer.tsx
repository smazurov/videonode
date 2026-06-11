import { useCallback, useEffect, useRef, useState } from 'react';
import Hls from 'hls.js';
import {
  MediaController,
  MediaControlBar,
  MediaTimeRange,
  MediaTimeDisplay,
  MediaPlayButton,
  MediaLiveButton,
  MediaMuteButton,
  MediaFullscreenButton,
} from 'media-chrome/react';
import { apiUrl } from '../../lib/api_fetch';
import { cn } from '../../utils';
import { RecordingFilmstrip } from './RecordingFilmstrip';
import type { components } from '../../lib/api.generated';

type RecordingStatus = components['schemas']['RecordingStatusData'];

interface RecordingPlayerProps {
  readonly status: RecordingStatus;
  readonly className?: string;
}

// RecordingPlayer plays a recording's fMP4/HLS via hls.js inside a Media Chrome
// player (HLS over MSE → Firefox/Linux + mobile, not just Safari) with built-in
// hover thumbnails, plus the storyboard filmstrip seek surface. Used by the
// recordings detail route.
export function RecordingPlayer({ status, className }: RecordingPlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [currentTime, setCurrentTime] = useState(0);

  const live = status.active ?? false;
  const playlistUrl = apiUrl(status.playlist_url);
  const vttUrl = apiUrl(status.thumbnails_vtt_url);
  const sessionBase = playlistUrl.replace(/\/index\.m3u8$/, '');

  // Attach hls.js to the <video>. Prefer MSE (Hls.isSupported) so Firefox/
  // Android work; fall back to native HLS for Safari.
  useEffect(() => {
    const video = videoRef.current;
    if (!video || !playlistUrl) return;

    let hls: Hls | null = null;
    if (Hls.isSupported()) {
      hls = new Hls({ lowLatencyMode: false });
      hls.loadSource(playlistUrl);
      hls.attachMedia(video);
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = playlistUrl;
    }
    return () => {
      hls?.destroy();
      video.removeAttribute('src');
      video.load();
    };
  }, [playlistUrl]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const onTime = () => setCurrentTime(video.currentTime);
    video.addEventListener('timeupdate', onTime);
    video.addEventListener('seeking', onTime);
    return () => {
      video.removeEventListener('timeupdate', onTime);
      video.removeEventListener('seeking', onTime);
    };
  }, []);

  const handleSeek = useCallback((seconds: number) => {
    const video = videoRef.current;
    if (!video) return;
    video.currentTime = seconds;
    setCurrentTime(seconds);
  }, []);

  return (
    <div className={cn('space-y-2', className)}>
      <MediaController className="aspect-video w-full overflow-hidden rounded bg-black">
        <video
          ref={videoRef}
          slot="media"
          muted
          playsInline
          autoPlay={live}
          className="h-full w-full"
        >
          {vttUrl && <track default kind="metadata" label="thumbnails" src={vttUrl} />}
        </video>
        <MediaControlBar>
          <MediaPlayButton />
          {live && <MediaLiveButton />}
          <MediaTimeRange />
          <MediaTimeDisplay showDuration />
          <MediaMuteButton />
          <MediaFullscreenButton />
        </MediaControlBar>
      </MediaController>
      {vttUrl && (
        <RecordingFilmstrip
          baseUrl={sessionBase}
          vttUrl={vttUrl}
          live={live}
          currentTime={currentTime}
          onSeek={handleSeek}
        />
      )}
    </div>
  );
}
