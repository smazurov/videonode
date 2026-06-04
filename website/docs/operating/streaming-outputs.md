# Consuming streaming outputs

VideoNode exposes each stream over three protocols simultaneously. The stream ID you set in the REST API is the identifier end-to-end: it appears in the RTSP path, the SRT `streamid` parameter, the WebRTC query string, and the Prometheus metrics label.

The default ports are `:8554` for RTSP, `:6001` for SRT, and `:8090` for the HTTP/WebRTC API. Override them in `config.toml` or via environment variables (`VIDEONODE_STREAMING_RTSP_PORT`, `VIDEONODE_SRT_ADDR`, `VIDEONODE_SERVER_PORT`).

The encoder only starts when the first reader connects. Expect up to a few seconds of buffering on the initial connection.

## RTSP

To watch the stream with ffplay, run:

```bash
ffplay rtsp://localhost:8554/cam-lobby
```

Replace `cam-lobby` with your stream ID. Any RTSP 1.0 client works: VLC, GStreamer, OBS.

## SRT

To watch the stream with ffplay over SRT, run:

```bash
ffplay "srt://localhost:6001?streamid=cam-lobby"
```

The `streamid` query parameter is how the server routes the connection to the right encoder. VLC 4.x and mpv (with `--demuxer-lavf-o=streamid=cam-lobby`) also accept SRT URIs.

## WebRTC

The simplest way to watch a stream over WebRTC is the built-in UI: log in to `http://<host>:8090/streams` and click the stream. The browser opens a peer connection for you.

### Embed a single stream

Each stream also has a chromeless URL designed to drop into an iframe or open in a kiosk:

```
http://<host>:8090/video?stream=<stream_id>
```

Optional query parameters:

| Param | Default | Effect |
|---|---|---|
| `muted` | `true` | Set to `false` to start with audio. |
| `stats` | `false` | Show the debug overlay on load. Toggles on double-click. |

Example:

```
http://videonode.local:8090/video?stream=cam-lobby&muted=false&stats=true
```

The view renders only the player at full viewport on a black background. No nav, no chrome.

### Debug stats

The built-in stats overlay calls `RTCPeerConnection.getStats()` once per second and shows resolution, FPS, frames decoded and dropped, codecs, current bitrate, jitter buffer delay, A/V sync offset, packet loss, and RTT. Open any stream in the UI to see it, or pass `?stats=true` on the embed URL, or double-click the player to toggle it. This is purely browser-side instrumentation; no extra endpoint is involved.

For server-side metrics across all consumers, scrape Prometheus at `/api/metrics`:

```bash
curl -u videonode:videonode http://localhost:8090/api/metrics \
  | jq '[.[] | select(.name | startswith("videonode_webrtc"))]'
```

Relevant series:

| Metric | Labels | What |
|---|---|---|
| `videonode_webrtc_active_peers` | `stream_id` | Connected peers per stream. |
| `videonode_webrtc_peer_nacks_total` | `stream_id`, `peer_id` | NACK count (packet loss indicator). |
| `videonode_webrtc_peer_plis_total` | `stream_id`, `peer_id` | PLI count (decoder requested a keyframe). |
| `videonode_webrtc_peer_jitter` | `stream_id`, `peer_id` | Interarrival jitter in RTP units. Divide by 90000 for seconds. |
| `videonode_webrtc_stream_bytes_total` | `stream_id` | Bytes sent per stream. |

See [observability](./observability) for the full metric inventory.

### Custom player

For a self-driven player, drive signaling against the daemon's WHEP (WebRTC-HTTP Egress Protocol) endpoint:

```
POST http://<host>:8090/whep/<stream_id>
Content-Type: application/sdp
Body: your SDP offer
```

The `201 Created` response carries your SDP answer in the body (`Content-Type: application/sdp`) and a `Location` header pointing at the session resource (`/whep/<stream_id>/<session_id>`). Set the answer as the remote description on your `RTCPeerConnection`. To tear the session down, send `DELETE` to that `Location`:

```
DELETE http://<host>:8090/whep/<stream_id>/<session_id>
```

Authentication follows the daemon's `auth.type` setting, using the same Linux-account or basic credentials as the rest of the API. See [config.toml reference](../reference/config-toml#auth).
