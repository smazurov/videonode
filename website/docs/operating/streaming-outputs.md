# Consuming streaming outputs

VideoNode exposes each stream over three protocols simultaneously. The stream ID you set in the REST API is the identifier end-to-end: it appears in the RTSP path, the SRT `streamid` parameter, the WebRTC query string, and the Prometheus metrics label.

The default ports are `:8554` for RTSP, `:6001` for SRT, and `:8090` for the HTTP/WebRTC API. Override them in `config.toml` or via environment variables (`VIDEONODE_STREAMING_RTSP_PORT`, `VIDEONODE_SRT_ADDR`, `VIDEONODE_SERVER_PORT`).

The encoder only starts when the first reader connects. Expect up to a few seconds of buffering on the initial connection.

## RTSP

To watch the stream with ffplay, run:

```bash
ffplay rtsp://localhost:8554/cam-lobby
```

Replace `cam-lobby` with your stream ID. Any RTSP 1.0 client works — VLC, GStreamer, OBS.

## SRT

To watch the stream with ffplay over SRT, run:

```bash
ffplay "srt://localhost:6001?streamid=cam-lobby"
```

The `streamid` query parameter is how the server routes the connection to the right encoder. VLC 4.x and mpv (with `--demuxer-lavf-o=streamid=cam-lobby`) also accept SRT URIs.

## WebRTC

WebRTC uses an SDP offer/answer exchange over the REST API. No WHEP endpoint exists; the browser must drive signaling via `POST /api/webrtc`.

To open a live preview in the VideoNode UI, navigate to:

```
http://localhost:8090
```

The built-in UI handles WebRTC signaling automatically. To implement your own player, post an SDP offer:

```bash
curl -X POST "http://localhost:8090/api/webrtc?stream=cam-lobby" \
  -H "Content-Type: application/sdp" \
  --data-binary @offer.sdp
```

The response body is the SDP answer (`Content-Type: application/sdp`). Set it as the remote description in your `RTCPeerConnection`.

Authentication follows the same Basic Auth required by all other API endpoints. See `config.toml` → `[auth]` for credentials.
