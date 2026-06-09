# Monitoring a running deployment

This page covers three operator tasks: scraping Prometheus metrics, consuming real-time SSE events, and filtering structured logs by entity ID.

## Scraping Prometheus metrics

VideoNode exposes two metrics endpoints. Use `/metrics` for Prometheus scraping. It serves the standard text exposition format and, unlike the rest of the API, is unauthenticated: it is registered on the raw mux ahead of the auth middleware, so point your scrape config straight at it.

```bash
curl http://localhost:8090/metrics
```

The same series are also available as JSON at `/api/metrics`, which does require auth. Use it for ad-hoc queries with `jq`:

```bash
curl -u videonode:videonode http://localhost:8090/api/metrics | jq '.[].name'
```

All metrics use the `videonode_` prefix. The families include:

- `videonode_ffmpeg_*`: per-stream encoder stats (`fps`, `dropped_frames_total`, `duplicate_frames_total`, `processing_speed`), labeled by `stream_id`
- `videonode_producer_*`: per-source process stats (`rss_bytes`, `cpu_percent`), labeled by `source_id`
- `videonode_webrtc_*` and `videonode_srt_*`: per-stream egress counters plus per-peer and per-consumer feedback (see below)
- `videonode_mpp_*`: Rockchip MPP device load and utilization (see below)

To see the full current set, scrape the endpoint directly; the list grows with the deployment.

### WebRTC per-peer metrics

Each connected WebRTC peer contributes a row of RTCP-feedback series, labeled by `stream_id` and `peer_id`. A peer's rows are removed when it disconnects.

| Metric | Type | What |
|---|---|---|
| `videonode_webrtc_active_peers` | gauge | Active peers per stream (labeled by `stream_id` only). |
| `videonode_webrtc_peer_rtcp_packets_total` | counter | RTCP packets received from the peer. |
| `videonode_webrtc_peer_nacks_total` | counter | NACK requests (packet-loss indicator). |
| `videonode_webrtc_peer_plis_total` | counter | PLI requests (decoder asked for a keyframe). |
| `videonode_webrtc_peer_firs_total` | counter | FIR requests (full intra request). |
| `videonode_webrtc_peer_jitter` | gauge | Interarrival jitter in RTP timestamp units. Divide by 90000 for seconds. |

### SRT per-consumer metrics

Each SRT consumer contributes a row labeled by `stream_id` and `consumer_id`, removed on disconnect.

| Metric | Type | What |
|---|---|---|
| `videonode_srt_active_consumers` | gauge | Active consumers per stream (labeled by `stream_id` only). |
| `videonode_srt_frames_written_total` | counter | Frames written per stream, also labeled by `codec`. |
| `videonode_srt_consumer_rtt_ms` | gauge | Round-trip time in milliseconds. |
| `videonode_srt_consumer_bandwidth_mbps` | gauge | Send bandwidth in Mbps. |
| `videonode_srt_consumer_packet_loss_rate` | gauge | Packet-loss rate. |
| `videonode_srt_consumer_retransmits_total` | counter | Retransmitted packets. |
| `videonode_srt_consumer_dropped_total` | counter | Dropped packets. |

### Rockchip MPP device load

On Rockchip hardware, a collector samples `/proc/mpp_service/load` every five seconds and publishes two gauges per hardware codec device, labeled by `device`:

- `videonode_mpp_device_load`: device load percentage.
- `videonode_mpp_device_utilization`: device utilization percentage.

The collector logs a warning and skips the sample when the proc file is absent, so these series are simply missing on hosts without the Rockchip MPP service.

## Inspecting supervised processes

VideoNode supervises one OS process per pipeline stage (source, composer, encoder). To list them with live state, query `/api/processes`:

```bash
curl -u videonode:videonode http://localhost:8090/api/processes | jq '.processes'
```

Each row carries the pool `id` (`source:<id>`, `composer:<id>`, or `encoder:<stream-id>`), the `kind` (`source`, `composer`, `encoder`, or `daemon`), `state`, `pid`, `restart_count`, `rss_bytes`, and `cpu_percent`. A `self` row of kind `daemon` reports the videonode process itself. Source rows also carry `refcount` and the `consumers` holding the device.

The `state` field is one of five pool states:

- `idle`: not running.
- `starting`: being started.
- `running`: active.
- `stopping`: being stopped.
- `error`: failed to start or crashed (see `last_error`).

To bounce one stage, post to `/api/processes/{id}/restart` with the pool id:

```bash
curl -u videonode:videonode -X POST \
  http://localhost:8090/api/processes/source:hdmi0/restart
```

Sources and composers are re-applied (the control plane reconnects); a running encoder is bounced, while an idle one with no reader attached is left down.

For the full schema, see [`/openapi.json`](http://localhost:8090/openapi.json).

## Consuming SSE events

To receive real-time lifecycle and status changes, connect to the single multiplexed SSE stream:

```bash
curl -u videonode:videonode -N http://localhost:8090/api/events
```

Each event carries a typed payload. The two you'll see most:

- `entity`: a uniform envelope for every per-entity update, discriminated by a `type` tag of the form `<entity>.<action>` (for example `source.status`), where `<entity>` is `source`, `composer`, or `stream` and `<action>` is `created`, `updated`, `deleted`, `status`, `metrics`, or `consumers`
- `pipeline-state-changed`: fires when the pipeline master switch toggles on or off

The connection sends a `heartbeat` every 15 seconds to keep proxies and idle clients alive. For the full event model, see [Events and SSE](../development/events-and-sse).

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
