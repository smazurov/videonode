# Capturing audio

A stream can carry one audio track per ALSA capture device. Pick the devices in the stream form's **Audio** block (or set `audio.devices` over the [REST API](../reference/rest-api)); with several devices and a **Mix filter** such as `amix=inputs=2:duration=shortest`, they collapse into one track.

## Device names

Prefer card names over card numbers: `hw:Lyra,0` keeps pointing at the same device after it is unplugged and replugged, while `hw:4,0` follows whichever card the kernel numbers 4 next. The device picker offers numbered entries; edit the saved value to the card name (from `/proc/asound/cards`) when the card count can change.

## Codec

Opus is the default and the only codec the WebRTC output can carry. AAC works for RTSP and SRT consumers.

## Unplugs, stalls, and recovery

`videonode-sink` captures the devices itself, so audio trouble never stalls video:

- While a device is missing or stops delivering samples, its track carries silence. Players that wait on audio keep playing.
- A device that disappears is retried every 500 ms, and one that goes quiet for 2 s is reopened. Real audio returns on its own after a replug, without restarting the stream.
- Stalls and recoveries are logged once per state change (for example `audio [0] vncap:CARD=Lyra,DEV=0: missing; sending silence, retrying (No such device)`, then `live again after 12.3s missing`). Xruns, filled silence, and late audio that had to be dropped are summarized in one line at most every 30 s, only when something happened.

## Watching audio health

Only `videonode-sink` can tell real samples from the silence it substitutes; the relay sees audio packets either way. On the stream page, each audio track sits under the encoder's video input with a wire into the encoder. A missing device, or one sending no samples, turns red and becomes the status headline. Xruns or substituted silence in the last 10 s show as amber `dropouts`. A dotted device name has a tooltip with the outage reason and start time, or the xrun and silence counts. The same view is in the stream's health:

```bash
curl -s -u videonode:videonode http://localhost:8090/api/streams/<stream_id> | jq '.health.encoder.sink.audio'
```

Each entry carries the `device`, its `state` (`live`, `stalled`, `absent`, or `opening`) with `since_us`, the `error` behind an absent state (for example `No such device`), and run totals for `xruns`, `silence_ms`, and `dropped_ms`. The same data streams over the `/api/events` metrics feed and as the `videonode_sink_audio_*` Prometheus series; `videonode_sink_audio_up == 0` is the alert condition.

## Lip sync

Video passes through more stages than audio (capture, conversion, compositing), so audio can arrive slightly ahead of the picture. Set **A/V offset (ms)** (`audio.offset_ms`, ±2000) to correct it: positive values delay audio, negative values delay video. To measure, clap on camera and compare the sound with the frame where your hands meet; 33 ms is one frame at 30 fps.
