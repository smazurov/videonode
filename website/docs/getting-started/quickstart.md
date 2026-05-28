# Quickstart

This tutorial takes you from a running VideoNode installation to a live stream in five steps. It assumes you have already [installed VideoNode](installation).

## Step 1: Start the service

```bash
sudo systemctl start videonode
```

You'll see no output — that's expected. The daemon logs to the system journal.

## Step 2: Confirm the API is up

```bash
curl http://localhost:8090/api/health
```

You should see:

```json
{"status":"ok","message":"API is healthy"}
```

The health endpoint requires no authentication.

## Step 3: Open the UI

Navigate to `http://localhost:8090` in a browser. A login prompt appears — enter username `admin` and password `password` (the defaults from `config.toml`; change them before exposing VideoNode to a network). You'll land on the stream dashboard.

## Step 4: Add a source

We'll use a test-pattern source so you don't need a capture device plugged in yet.

```bash
curl -u admin:password \
  -X POST http://localhost:8090/api/sources \
  -H "Content-Type: application/json" \
  -d '{"id":"test-src","test_mode":true}'
```

You should see a response containing `"id":"test-src"` and `"status":"idle"`.

Next, create a stream that encodes from that source and publishes over RTSP:

```bash
curl -u admin:password \
  -X POST http://localhost:8090/api/streams \
  -H "Content-Type: application/json" \
  -d '{
    "stream_id": "test-stream",
    "upstream": "source:test-src",
    "encoder": {"codec":"h264","bitrate":"2M"},
    "publish": [{"type":"rtsp","url":"rtsp://localhost:8554/test-stream"}]
  }'
```

Enable the pipeline to start the process:

```bash
curl -u admin:password \
  -X POST http://localhost:8090/api/pipeline/start
```

Refresh the UI — you'll see `test-stream` listed with a live WebRTC preview.

## Step 5: Consume the stream

Click the stream thumbnail in the UI for in-browser WebRTC playback. To pull from an external player, use the RTSP URL you configured above:

```bash
ffplay -fflags nobuffer -flags low_delay -framedrop \
  -rtsp_transport tcp rtsp://localhost:8554/test-stream
```

For SRT and other output options, see [Streaming outputs](../operating/streaming-outputs).
