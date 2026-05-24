# Migrating `streams.toml` from v1 to v2

VideoNode v2 splits the old monolithic `[[streams]]` block into three
top-level entities: `[[sources]]`, `[[composers]]`, and `[[streams]]`.
The daemon detects v1 files on load and rewrites them in place. This
document describes what the rewrite does, what to expect in the result,
and how to revert.

## When migration runs

On daemon startup, the TOML loader inspects the file:

- If the top-level key `version = 2` is present, the file is already v2
  and is loaded as-is.
- Otherwise, if any `[[streams.inputs]]` table is present, the file is
  treated as v1 and migrated.
- The migrated file is written back to the same path on disk before the
  daemon continues. The original is **not** preserved as a `.bak` — rely
  on git (see "Reverting" below).

You can also trigger migration explicitly without starting the daemon:

```bash
videonode migrate-config path/to/streams.toml
```

## What the migration does

The migration is mechanical and deterministic. For each v1 `[[streams]]`
entry the loader:

1. **Synthesizes one `[[sources]]` per unique `device`.** Two v1 streams
   that pointed at `device = "usb-1-2"` collapse into a single source
   shared by both v2 streams. Source ids are derived from the `inputs[].id`
   when unique, otherwise from the device string (kebab-cased).

2. **Migrates `test_mode` down to the source.** v1 had a stream-level
   `test_mode` flag; v2 has a source-level one. A v1 stream with
   `test_mode = true` and no real device becomes a `test_mode = true`
   source. A v1 stream that had both a device AND `test_mode = true` is
   migrated as a real-device source — the flag at the stream level was
   already a no-op in v1 (see the README note from pipeline-rip).

3. **Synthesizes a `[[composers]]` when needed.** A composer is created
   when **any** of the following are true for a v1 stream:
   - `len(inputs) > 1` (multi-source canvas)
   - one or more inputs has an entry in `[[streams.effects.<input_id>]]`
   - `force_composer = true`

   The composer's `canvas` dims, `inputs`, and `layout` are copied from
   the v1 stream's `inputs`/`layout`/`effects` tables, with layout slots
   rewritten from positional `slot = N` to named `input = "source:<id>"`.

4. **Rewrites the stream as the v2 shape.** Drops `inputs`, `layout`,
   `effects`, `force_composer`, and the stream-level `test_mode`. Adds a
   single `upstream = "composer:<id>"` (when a composer was synthesized)
   or `upstream = "source:<id>"` (when the stream is a direct passthrough).

5. **Stamps the file with `version = 2`** so subsequent loads skip the
   migration pass.

## Worked example

### Before (v1)

```toml
[[streams]]
id = "split-show"
name = "Side-by-side cameras"

  [[streams.inputs]]
  id = "left"
  device = "usb-1-2"

  [[streams.inputs]]
  id = "right"
  device = "usb-1-3"

  [[streams.layout]]
  slot = 0
  x = 0; y = 0; w = 1920; h = 1080

  [[streams.layout]]
  slot = 1
  x = 1920; y = 0; w = 1920; h = 1080

  [streams.encoder]
  codec = "h265"
  bitrate = "12M"
  gop = 120

  [streams.audio]
  devices = ["hw:CARD=USB,DEV=0"]
  codec = "aac"
  bitrate = "192k"

  [[streams.publish]]
  type = "rtsp"
  url = "rtsp://localhost:8554/split-show"
```

### After (v2)

```toml
version = 2

[[sources]]
id = "left"
device = "usb-1-2"

[[sources]]
id = "right"
device = "usb-1-3"

[[composers]]
id = "split-show"
canvas = { w = 3840, h = 1080 }

  [[composers.split-show.inputs]]
  ref = "source:left"

  [[composers.split-show.inputs]]
  ref = "source:right"

  [[composers.split-show.layout]]
  input = "source:left"
  x = 0; y = 0; w = 1920; h = 1080

  [[composers.split-show.layout]]
  input = "source:right"
  x = 1920; y = 0; w = 1920; h = 1080

[[streams]]
id = "split-show"
name = "Side-by-side cameras"
upstream = "composer:split-show"

  [streams.split-show.encoder]
  codec = "h265"
  bitrate = "12M"
  gop = 120

  [streams.split-show.audio]
  devices = ["hw:CARD=USB,DEV=0"]
  codec = "aac"
  bitrate = "192k"

  [[streams.split-show.publish]]
  type = "rtsp"
  url = "rtsp://localhost:8554/split-show"
```

The composer id matches the v1 stream id because the canvas was implicit
in the stream itself. If two v1 streams happened to share an identical
input set + layout + effects, the migrator does **not** dedupe them into
a shared composer — each v1 stream becomes its own composer. You can
manually merge them afterwards (point both `upstream`s at one composer,
delete the duplicate composer entry) to get the multi-encode optimization
described in the README.

## Reverting

The migration writes the file in place with no backup. Since the file
should be checked into git, revert with:

```bash
git checkout HEAD~1 -- streams.toml
```

Then either pin the daemon to a pre-v2 release, or hand-add
`version = 2` to the top of your file once you have finished editing in
v1 shape (the daemon will then treat the v1 shape underneath as a
malformed v2 file and refuse to load — which is the right failure mode).

If you simply want to inspect the diff before committing the migrated
file:

```bash
git diff streams.toml
```

## Fields removed in v2

The migrator silently drops these v1 fields (they have no v2 equivalent):

- Stream-level `test_mode` — moved to source.
- Stream-level `force_composer` — replaced by "composer exists in
  `[[composers]]` or it doesn't."
- Stream-level `inputs`, `layout`, `effects` — moved into composer.
- Positional `slot = N` inside `[[streams.layout]]` — replaced by named
  `input = "source:<id>"` inside `[[composers.<id>.layout]]`.

If your v1 file uses any of these, the migrator handles them. If you
were relying on them via the API, see the v2 API surface
(`/api/sources`, `/api/composers`, `/api/streams`) for the replacements.
