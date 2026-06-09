# Introduction

This page explains what VideoNode is and how its three-entity pipeline model works. For step-by-step setup, see [Installation](./installation) and [Quickstart](./quickstart).

VideoNode is a self-hosted video streaming server for Linux. It takes frames from V4L2 capture devices (webcams, HDMI capture cards) and publishes them as RTSP, SRT, and WebRTC streams. Because it runs headless and ships as a single binary plus native C++ pipeline binaries, it targets Rockchip RK3588 single-board computers (the FriendlyElec NanoPC-T6 is the reference board), and runs on any Linux host with a GPU through software fallbacks. The daemon detects connected devices automatically and builds FFmpeg pipelines from a declarative TOML config. Run the `validate-encoders` subcommand once to probe which hardware encoders work and record the result.

## Three-entity pipeline model

VideoNode organises every pipeline into three independent entities. Each has its own identity, CRUD surface, and lifecycle. Streams reference upstream entities by explicit ID. There is no monolithic block that bundles capture, composition, and encoding together.

- **Source** captures V4L2 frames from one device (or generates a test pattern when `test_mode = true`) and broadcasts raw NV12 frames to any number of consumers. A source runs as a separate `videonode-source` process per source ID.
- **Composer** reads frames from one or more sources, composites them onto a canvas using libplacebo (Vulkan, with an OpenGL fallback), and broadcasts the result. Multiple streams can share one composer: the GPU does the compositing work once, and each stream pays only its own encoder cost. A composer is optional; streams that need no compositing reference a source directly.
- **Stream** pairs an upstream reference (`"source:<id>"` or `"composer:<id>"`) with an encoder config. The encoder publishes to the daemon's local RTSP relay, and SRT and WebRTC fan out from there; the output destination is hardcoded, not user-configurable. The encoder process spawns lazily when the first viewer connects and stops after the last one disconnects.

<!-- depicts: CLAUDE.md §"Pipeline model" -->
```mermaid
flowchart TB
  accTitle: VideoNode three-entity pipeline model
  accDescr {
    A V4L2 device feeds videonode-source. The source broadcasts NV12 frames
    to videonode-composer and directly to videonode-sink instances via
    SCM_RIGHTS. The composer produces a BGRA canvas that feeds its own sink.
    Each sink pipes into ffmpeg, which publishes to RTSP, SRT, or WebRTC.
  }

  dev["V4L2 device"] --> src["videonode-source"]
  src -- "NV12" --> comp["videonode-composer"]
  src -- "NV12" --> sink1["videonode-sink"]
  comp -- "BGRA" --> sink2["videonode-sink"]
  sink1 --> ff1["ffmpeg"]
  sink2 --> ff2["ffmpeg"]
  ff1 -- "H.264/H.265" --> vn["videonode (RTSP / SRT / WebRTC fanout)"]
  ff2 -- "H.264/H.265" --> vn
  vn --> viewers["viewers"]
```

The config that drives this model uses three TOML tables: `[[sources]]`, `[[composers]]`, and `[[streams]]`. Each stream names its upstream with an `upstream` field; each composer lists its sources under `inputs[].ref`; sources carry no reference. For the full entity model and lifecycle details, see the [pipeline model reference](../reference/pipeline-model).
