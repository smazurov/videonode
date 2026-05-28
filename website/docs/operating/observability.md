# Monitoring a running deployment

This page covers three operator tasks: scraping Prometheus metrics, consuming real-time SSE events, and filtering structured logs by entity ID.

## Scraping Prometheus metrics

VideoNode exposes a standard Prometheus text endpoint. To collect metrics, point your scrape config at `/metrics`:

```bash
curl -u videonode:videonode http://localhost:8090/metrics
```

All metrics use the `videonode_` prefix. The families include:

- `videonode_ffmpeg_*`: per-stream encoder stats (`fps`, `dropped_frames_total`, `duplicate_frames_total`, `processing_speed`), labeled by `stream_id`
- `videonode_producer_*`: per-source process stats (`rss_bytes`, `cpu_percent`), labeled by `source_id`
- `videonode_webrtc_*` and `videonode_srt_*`: egress byte and packet counters, labeled by `stream_id`

To see the full current set, scrape the endpoint directly; the list grows with the deployment.

## Consuming SSE events

To receive real-time lifecycle and status changes, connect to the single multiplexed SSE stream:

```bash
curl -u videonode:videonode -N http://localhost:8090/api/events
```

Each event carries a typed payload. Two events you'll see frequently:

- `pipeline-state-changed`: fires when the pipeline master switch toggles on or off
- `source-status`: fires when a source's frame delivery state changes (e.g., stalled, recovering)

The connection sends a `heartbeat` every 15 seconds to keep proxies and idle clients alive.

## Filtering logs by stream

VideoNode writes structured logs via `slog`. Each log record's attributes map to uppercase journal fields.

To see all fields on recent records:

```bash
journalctl -t videonode -o verbose --since "5 min ago"
```

To filter records for one stream:

```bash
journalctl -t videonode STREAM_ID=rtsp-lobby
```

To filter by module (e.g., the encoder pipeline):

```bash
journalctl -t videonode MODULE=encoder
```

To list every stream ID that has produced a log record:

```bash
journalctl -F STREAM_ID
```

Use `SOURCE_ID` and `COMPOSER_ID` in place of `STREAM_ID` for source and composer log records respectively.
