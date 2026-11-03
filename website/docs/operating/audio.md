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
- Stalls and recoveries are logged once per state change (for example `audio [0] vncap:CARD=Lyra,DEV=0: device unavailable; filling silence and retrying`, then `live again after 12.3s absent`). Xruns, filled silence, and late audio that had to be dropped are summarized in one line at most every 30 s, only when something happened.

## Lip sync

Video passes through more stages than audio (capture, conversion, compositing), so audio can arrive slightly ahead of the picture. Set **A/V offset (ms)** (`audio.offset_ms`, ±2000) to correct it: positive values delay audio, negative values delay video. To measure, clap on camera and compare the sound with the frame where your hands meet; 33 ms is one frame at 30 fps.
