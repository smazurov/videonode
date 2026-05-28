# Quickstart

This tutorial takes you from a running VideoNode installation to a live stream playing in your browser. It assumes you've completed [Installation](./installation).

## Step 1: Open the UI

Browse to `http://<your-box>:8090` (or `http://localhost:8090` if you're on the box itself). The login page appears. Sign in with your Linux username and password (or the basic-auth credentials, if you switched to that during install).

You'll land on the **Streams** page, empty on a fresh install:

![Empty streams page with a Create Stream button](/screenshots/streams-empty.png)

## Step 2: Create a source

A *source* is the upstream that produces frames: a real V4L2 device or a built-in test pattern. Click **Sources** in the nav, then **New source**.

![New source form: ID field, test-mode checkbox, video device dropdown, format / resolution / framerate selectors](/screenshots/new-source.png)

Pick a Source ID (kebab-case), select your capture device from the **Video Device** dropdown, and pick a format, resolution, and framerate. If you don't have hardware plugged in yet, tick **Test mode** to use the built-in test-pattern producer instead. Click **Create Source**.

The source appears in the table with status **Running** and consumer count `0`. It's broadcasting frames but no one is listening yet.

![Sources page showing a running test source](/screenshots/sources-list.png)

## Step 3: Create a stream

A *stream* pairs an upstream (your new source) with an encoder and a publish target. Click **Streams**, then **Create Stream**.

![New stream form: stream ID, upstream selector, encoder fields, audio block](/screenshots/new-stream.png)

Set:

- **Stream ID**: anything kebab-case; this becomes the URL path component for RTSP/SRT/WebRTC consumers.
- **Upstream**: pick your source from the dropdown.
- **Codec**: `H.264` for widest compatibility.
- **Bitrate**: `2M` is a reasonable default for 1080p.

Leave the rest at defaults and click **Create Stream**. The stream appears in the Streams table.

## Step 4: Watch the stream

The Streams page renders a live WebRTC preview as soon as you click the row. To open the stream in an external player, see [Streaming outputs](../operating/streaming-outputs) for the RTSP / SRT / WebRTC URL formats and example consumer commands.
