#!/usr/bin/env bash
# smoke.sh — fully automated, quick end-to-end smoke for the composer pipeline.
#
# Exercises every binary (videonode-source / videonode-sink / videonode-composer)
# and reports PASS / FAIL / SKIP per scenario with one final summary.
# Designed for ≤60s total wall time.
#
# Scenarios:
#   R-prefix: no daemon — exercises raw videonode-source/sink binaries.
#     R1 hdmiin-source-sink         source(/dev/video0) → SCM → sink → Y4M → ffprobe
#     R4 consumer-reconnect         5× sink connect/disconnect; source survives, fds stable
#     R6 mjpeg-uvc                  only if a UVC MJPEG device is plugged in, else SKIP
#
#   I-prefix: daemon-driven — smoke owns its own ephemeral videonode
#     instance (custom port + sockets + streams.toml). Validates the
#     REST → pipelinectl → composer IPC contract end-to-end.
#     I1 ipc-canvas-perspective     engage canvas, PATCH perspective, assert no composer restart,
#                                   ffprobe codec=h264, fps ≥70% target
#     I2 ipc-resource-usage         10s RSS+%CPU sampling of daemon-supervised source + composer
#
# Usage:
#   smoke.sh                      # both targets if available
#   smoke.sh --target rig
#   smoke.sh --target host
#   smoke.sh --scenarios H1,R1,R4
#   smoke.sh --keep-artifacts     # don't delete /tmp/smoke-* on exit
#   smoke.sh --duration 4         # per-scenario capture seconds (default 4)
#   smoke.sh --warmup 2           # sampler skips first N seconds (default 1)
#   smoke.sh --steady-state       # convenience: --warmup 3 + --duration ≥8
#
# Exit code: 0 if no FAILs (SKIPs are OK), 1 otherwise.

set -uo pipefail
shopt -s lastpipe

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
# The Go module lives one level up from composer/ (composer/ is the C++ side).
GO_MODULE_ROOT="$(cd "${REPO_ROOT}/.." && pwd)"

TARGET="both"
SCENARIOS=""
KEEP=0
DURATION=4
# Warmup window (seconds) during which the resource sampler ignores CPU
# accumulation. Set higher for steady-state-only reporting; 0 to include
# startup transients.
WARMUP=1
RIG="${RIG:-orangepi}"   # ssh alias from ~/.ssh/config; full host works too
# Override RIG_SSH to use a specific key or auth method:
#   RIG_SSH="ssh -i ~/.ssh/myrig_key" smoke.sh ...
RIG_SSH="${RIG_SSH:-ssh}"
RIG_BUILD="${RIG_BUILD:-/home/orangepi/composer/build}"
HOST_BUILD="${HOST_BUILD:-/tmp/composer-build}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/smoke-composer}"
RIG_ARTIFACTS_DIR="/tmp/smoke-composer-rig"

while [ $# -gt 0 ]; do
    case "$1" in
        --target) TARGET="$2"; shift 2 ;;
        --scenarios) SCENARIOS="$2"; shift 2 ;;
        --keep-artifacts) KEEP=1; shift ;;
        --duration) DURATION="$2"; shift 2 ;;
        --warmup) WARMUP="$2"; shift 2 ;;
        --steady-state)
            # Convenience preset: 3s warmup, 8s duration → average over 5s
            # of steady-state work. Skip transient init costs.
            WARMUP=3
            [ "$DURATION" -lt 8 ] && DURATION=8
            shift ;;
        -h|--help)
            sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *) echo "unknown arg: $1" >&2; exit 2 ;;
    esac
done

# --------------------------- pretty output -----------------------------------
C_RESET=$'\033[0m'; C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'; C_DIM=$'\033[2m'
[[ -t 1 ]] || { C_RESET=; C_GREEN=; C_RED=; C_YELLOW=; C_DIM=; }

declare -a RESULTS_NAME RESULTS_STATUS RESULTS_DETAIL
record() {
    local name="$1" status="$2" detail="$3"
    RESULTS_NAME+=("$name"); RESULTS_STATUS+=("$status"); RESULTS_DETAIL+=("$detail")
    local color
    case "$status" in
        PASS) color="$C_GREEN" ;;
        FAIL) color="$C_RED" ;;
        SKIP) color="$C_YELLOW" ;;
        *)    color="" ;;
    esac
    printf '  %s%-4s%s  %-26s  %s\n' "$color" "$status" "$C_RESET" "$name" "$detail"
}

scenario() { printf '\n%s──%s %s%s%s\n' "$C_DIM" "$C_RESET" "$1" "$C_DIM" "──$C_RESET"; }

mkdir -p "$ARTIFACTS_DIR"
cleanup() {
    if [ "$KEEP" -eq 0 ]; then
        rm -rf "$ARTIFACTS_DIR" 2>/dev/null || true
        $RIG_SSH -o ConnectTimeout=5 "$RIG" "rm -rf $RIG_ARTIFACTS_DIR" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# --------------------------- helpers -----------------------------------------

scenario_selected() {
    local name="$1"
    [ -z "$SCENARIOS" ] && return 0
    [[ ",$SCENARIOS," == *",$name,"* ]]
}

# count_frames_y4m FILE → echoes integer frame count
count_frames_y4m() {
    # FRAME\n marker prefixes every frame in YUV4MPEG2
    grep -ac '^FRAME$' "$1" 2>/dev/null || echo 0
}

# observed_fps FILE_BYTES BYTES_PER_FRAME DURATION → integer fps
observed_fps_bgra() {
    local bytes="$1" per_frame="$2" duration="$3"
    [ "$per_frame" -eq 0 ] && { echo 0; return; }
    echo $(( bytes / per_frame / duration ))
}

# assert_fps OBS_FPS TARGET_FPS NAME → record PASS/FAIL
assert_fps() {
    local obs="$1" target="$2" name="$3" extra="${4:-}"
    local floor=$(( target * 7 / 10 ))   # 70% of target
    if [ "$obs" -ge "$floor" ]; then
        record "$name" PASS "fps=${obs}/${target} ${extra}"
    else
        record "$name" FAIL "fps=${obs}/${target} (floor ${floor}) ${extra}"
    fi
}

# ----- host scenarios --------------------------------------------------------


# ----- rig scenarios ---------------------------------------------------------

# rig_exec CMD — run a single bash heredoc on the rig; stderr surfaced to log.
rig_exec() {
    local label="$1" script="$2"
    local log="$ARTIFACTS_DIR/${label}.log"
    $RIG_SSH -o ConnectTimeout=5 "$RIG" "bash -s" <<<"$script" >"$log" 2>&1
    local rc=$?
    echo "$log:$rc"
}

# Shared bash helpers prepended to every rig scenario script. Provides:
#   start_sampler <name> <pid>   — poll RSS+CPU for that process until it exits
#   stop_samplers                  — wait for all samplers to finish writing
# Samplers append "SAMPLE name=... rss_kb=... cpu_pct=..." lines into a
# side file (one per scenario) so they don't collide with the RESULT line.
# CLK_TCK is 100 on aarch64 Linux; CPU% is computed from utime+stime delta
# vs wall-clock delta in centiseconds. A 100%-of-one-core process prints 100;
# a process using 2 cores prints ~200.
RIG_HELPERS_TMPL='
declare -A SAMPLER_PIDS=()
SAMPLER_WARMUP_S=__WARMUP_S__
start_sampler() {
    local name="$1" pid="$2"
    [ -d "/proc/$pid" ] || return 0
    (
        peak_rss=0
        utime_anchor=$(awk "{print \$14+\$15}" /proc/$pid/stat 2>/dev/null || echo 0)
        utime_last=$utime_anchor
        spawn_ns=$(date +%s%N)
        steady_start_ns=$((spawn_ns + SAMPLER_WARMUP_S * 1000000000))
        steady_anchor_set=0
        end_ns=$spawn_ns
        while [ -d "/proc/$pid" ]; do
            r=$(awk "/^VmRSS:/{print \$2}" /proc/$pid/status 2>/dev/null || echo 0)
            [ -n "$r" ] && [ "$r" -gt "$peak_rss" ] && peak_rss=$r
            u=$(awk "{print \$14+\$15}" /proc/$pid/stat 2>/dev/null || echo "")
            [ -n "$u" ] && utime_last=$u
            now_ns=$(date +%s%N)
            end_ns=$now_ns
            # Re-anchor utime at end of warmup so CPU% reflects steady state.
            if [ "$steady_anchor_set" -eq 0 ] && [ "$now_ns" -ge "$steady_start_ns" ]; then
                utime_anchor=$utime_last
                spawn_ns=$now_ns
                steady_anchor_set=1
            fi
            sleep 0.25
        done
        wall_cs=$(( (end_ns - spawn_ns) / 10000000 ))
        tick_delta=$(( utime_last - utime_anchor ))
        # Report 10ths of percent so light load is still visible. 1 tick =
        # 10ms = 1% over 1s wall time. cpu_tenths=12 → 1.2%.
        cpu_tenths=0
        [ "$wall_cs" -gt 0 ] && cpu_tenths=$(( 1000 * tick_delta / wall_cs ))
        echo "SAMPLE name=$name rss_kb=$peak_rss cpu_tenths=$cpu_tenths wall_cs=$wall_cs ticks=$tick_delta warmed=$steady_anchor_set"
    ) >>__SAMPLE_OUT__ 2>&1 &
    SAMPLER_PIDS["$name"]=$!
}
stop_samplers() {
    for n in "${!SAMPLER_PIDS[@]}"; do
        wait "${SAMPLER_PIDS[$n]}" 2>/dev/null || true
    done
}
'
# Returns the helper block with the per-scenario sample-out path + the
# WARMUP value substituted.
rig_helpers_for() {
    local label="$1"
    local out="$RIG_ARTIFACTS_DIR/${label}.samples"
    local body="${RIG_HELPERS_TMPL//__SAMPLE_OUT__/$out}"
    body="${body//__WARMUP_S__/$WARMUP}"
    echo "rm -f $out"
    echo "$body"
}

# Fetches SAMPLE lines from the rig's per-scenario samples file and formats
# them as "src=NMB/X% comp=NMB/Y%" for the test record.
fetch_samples() {
    local label="$1"
    $RIG_SSH -o ConnectTimeout=5 "$RIG" "cat $RIG_ARTIFACTS_DIR/${label}.samples 2>/dev/null"
}
format_samples() {
    local label="$1" out="" any_warmed=""
    while IFS= read -r line; do
        local name rss tenths mb whole frac warmed
        name=$(echo "$line" | grep -oP 'name=\K\S+')
        rss=$(echo "$line" | grep -oP 'rss_kb=\K[0-9]+')
        tenths=$(echo "$line" | grep -oP 'cpu_tenths=\K[0-9]+')
        warmed=$(echo "$line" | grep -oP 'warmed=\K[0-1]')
        [ -z "$name" ] && continue
        mb=$(( (${rss:-0} + 512) / 1024 ))
        tenths=${tenths:-0}
        whole=$(( tenths / 10 ))
        frac=$(( tenths % 10 ))
        out="$out $name=${mb}MB/${whole}.${frac}%"
        [ "${warmed:-0}" = "1" ] && any_warmed=1
    done < <(fetch_samples "$label")
    if [ -n "$any_warmed" ]; then
        echo "${out# } (steady, ≥${WARMUP}s warmup)"
    else
        echo "${out# }"
    fi
}

R_HDMI_LOCKED=0
R_setup() {
    # Pre-stage artifacts dir + sanity-check binaries on the rig.
    $RIG_SSH -o ConnectTimeout=5 "$RIG" "mkdir -p $RIG_ARTIFACTS_DIR && \
        for b in videonode-source videonode-sink videonode-composer; do \
            [ -x $RIG_BUILD/src/\$b ] || [ -x $RIG_BUILD/src/bin/\$b ] || { echo MISSING \$b; exit 1; }; \
        done" >/dev/null 2>&1 || return 1

    # Probe HDMI signal stability: 3 successive locks across 600ms = stable.
    # The output drives R2's run/skip decision and R1/R3's fps thresholds.
    local stable
    stable=$($RIG_SSH -o ConnectTimeout=3 "$RIG" '
        ok=0
        for i in 1 2 3; do
            if v4l2-ctl -d /dev/video0 --query-dv-timings 2>/dev/null | grep -q "Active width: [1-9]"; then
                ok=$((ok+1))
            fi
            sleep 0.2
        done
        echo $ok' 2>/dev/null)
    if [ "${stable:-0}" -ge 3 ]; then
        R_HDMI_LOCKED=1
        printf '  %srig HDMI: locked (signal stable)%s\n' "$C_DIM" "$C_RESET"
    else
        printf '  %srig HDMI: %s/3 locks across 600ms (unstable or no signal — placeholder paths still tested)%s\n' "$C_DIM" "${stable:-0}" "$C_RESET"
    fi
    return 0
}

R_rig_alive() {
    $RIG_SSH -o ConnectTimeout=3 -o BatchMode=yes "$RIG" 'true' >/dev/null 2>&1
}

R1_hdmiin_source_sink() {
    scenario_selected R1 || return
    scenario "R1 hdmiin-source-sink (rig)"
    local sock="/tmp/vn-smoke-r1.sock"
    local y4m="$RIG_ARTIFACTS_DIR/R1.y4m"
    local helpers
    helpers=$(rig_helpers_for R1)
    local script
    script=$(cat <<EOF
set -uo pipefail
$helpers
rm -f $sock $y4m
SRC=\$(ls $RIG_BUILD/src/videonode-source $RIG_BUILD/src/bin/videonode-source 2>/dev/null | head -1)
SINK=\$(ls $RIG_BUILD/src/videonode-sink $RIG_BUILD/src/bin/videonode-sink 2>/dev/null | head -1)
"\$SRC" --device /dev/video0 --out-socket $sock --seconds $((DURATION+2)) \
    >$RIG_ARTIFACTS_DIR/R1.src.log 2>&1 &
SRC_PID=\$!
start_sampler source \$SRC_PID
for i in {1..40}; do [ -S $sock ] && break; sleep 0.1; done
[ -S $sock ] || { echo "FAIL: socket never appeared"; kill \$SRC_PID 2>/dev/null; exit 1; }
timeout $((DURATION+1)) "\$SINK" --socket $sock --first-frame-timeout 10 \
    >$y4m 2>$RIG_ARTIFACTS_DIR/R1.sink.log &
SINK_PID=\$!
start_sampler sink \$SINK_PID
wait \$SINK_PID 2>/dev/null
SINK_RC=\$?
# Tell source to flush its shutdown log line (real=N placeholder=M).
kill -TERM \$SRC_PID 2>/dev/null; wait \$SRC_PID 2>/dev/null
stop_samplers
# Authoritative frame count: source's shutdown line "real=N placeholder=M".
# The y4m file is unreliable here because the sink may emit a second header
# when source dimensions change (placeholder 1920x1080 → live 3840x2160 etc).
LINE=\$(grep -oE 'real=[0-9]+ placeholder=[0-9]+' $RIG_ARTIFACTS_DIR/R1.src.log | tail -1)
REAL=\$(echo "\$LINE" | grep -oP 'real=\K[0-9]+'); REAL=\${REAL:-0}
PLAC=\$(echo "\$LINE" | grep -oP 'placeholder=\K[0-9]+'); PLAC=\${PLAC:-0}
TOTAL=\$(( REAL + PLAC ))
SZ=\$(stat -c%s $y4m 2>/dev/null || echo 0)
echo "RESULT total=\$TOTAL real=\$REAL placeholder=\$PLAC bytes=\$SZ sink_rc=\$SINK_RC"
EOF
)
    local result_line
    result_line=$(rig_exec R1 "DURATION=$DURATION
$script")
    local log="${result_line%:*}"
    local total real placeholder
    total=$(grep -oP 'total=\K[0-9]+' "$log" 2>/dev/null | head -1)
    real=$(grep -oP 'real=\K[0-9]+' "$log" 2>/dev/null | head -1)
    placeholder=$(grep -oP 'placeholder=\K[0-9]+' "$log" 2>/dev/null | head -1)
    total=${total:-0}; real=${real:-0}; placeholder=${placeholder:-0}
    if [ "$total" -eq 0 ]; then
        record R1 FAIL "no frames broadcast; see $log"; return
    fi
    # Source's --broadcast-fps default is 60. When HDMI is stable we expect
    # the full 60fps; when transitioning, state-machine overhead drops it,
    # so use 30fps as the threshold (the placeholder path's worst-case rate).
    local target_fps=60
    [ "$R_HDMI_LOCKED" -eq 1 ] || target_fps=30
    local obs=$(( total / DURATION ))
    local res
    res=$(format_samples R1)
    assert_fps "$obs" "$target_fps" R1 "real=$real placeholder=$placeholder ${res:+| $res}"
}



R4_consumer_reconnect() {
    scenario_selected R4 || return
    scenario "R4 consumer-reconnect (rig)"
    local sock="/tmp/vn-smoke-r4.sock"
    local script
    script=$(cat <<'EOF'
set -uo pipefail
sock="/tmp/vn-smoke-r4.sock"
rm -f $sock
SRC=$(ls __RIG_BUILD__/src/videonode-source __RIG_BUILD__/src/bin/videonode-source 2>/dev/null | head -1)
SINK=$(ls __RIG_BUILD__/src/videonode-sink __RIG_BUILD__/src/bin/videonode-sink 2>/dev/null | head -1)
"$SRC" --device /dev/video0 --out-socket $sock --seconds 30 \
    >__ART__/R4.src.log 2>&1 &
SRC_PID=$!
for i in {1..40}; do [ -S $sock ] && break; sleep 0.1; done
[ -S $sock ] || { echo "FAIL: socket"; kill $SRC_PID 2>/dev/null; exit 1; }
# Warm-up: 3 cycles + 1.5s settle. Forces all lazy allocations (e.g.
# /dev/rga opens the first time a LIVE-mode frame needs CSC; just one
# warmup cycle isn't enough if HDMI is still in placeholder state).
for i in 1 2 3; do
    timeout 1 "$SINK" --socket $sock --first-frame-timeout 5 \
        >/dev/null 2>>__ART__/R4.sinks.log || true
    sleep 0.2
done
sleep 1.5
# Capture the fd SET (paths, not just count) so we can diff exactly which
# fds appeared between baseline and post-cycles.
list_fds() {
    for f in $(ls /proc/$SRC_PID/fd 2>/dev/null | sort -n); do
        echo "$f $(readlink /proc/$SRC_PID/fd/$f 2>/dev/null)"
    done
}
list_fds >__ART__/R4.fd0.txt
FD0=$(wc -l <__ART__/R4.fd0.txt)
CYCLES=5
for i in $(seq 1 $CYCLES); do
    timeout 1 "$SINK" --socket $sock --first-frame-timeout 5 \
        >/dev/null 2>>__ART__/R4.sinks.log || true
    sleep 0.2
done
# Generous settle window so any async cleanup completes before we sample.
# If fds are still growing 2s after the last sink died, it's not a race —
# it's a real leak in the source's consumer-eviction path.
sleep 2
list_fds >__ART__/R4.fd1.txt
FD1=$(wc -l <__ART__/R4.fd1.txt)
ALIVE=0; kill -0 $SRC_PID 2>/dev/null && ALIVE=1
kill $SRC_PID 2>/dev/null; wait $SRC_PID 2>/dev/null
# Anything in fd1 that wasn't in fd0, IGNORING the fd number (which may
# differ even for the same resource), comparing only the target paths.
diff_targets=$(comm -13 \
    <(awk '{print $2}' __ART__/R4.fd0.txt | sort -u) \
    <(awk '{print $2}' __ART__/R4.fd1.txt | sort -u) | tr '\n' '|')
echo "RESULT fd0=$FD0 fd1=$FD1 alive=$ALIVE cycles=$CYCLES new_targets=$diff_targets"
EOF
)
    script="${script//__RIG_BUILD__/$RIG_BUILD}"
    script="${script//__ART__/$RIG_ARTIFACTS_DIR}"
    local result_line
    result_line=$(rig_exec R4 "$script")
    local log="${result_line%:*}"
    local fd0 fd1 alive new_targets
    fd0=$(grep -oP 'fd0=\K[0-9]+' "$log" 2>/dev/null | head -1)
    fd1=$(grep -oP 'fd1=\K[0-9]+' "$log" 2>/dev/null | head -1)
    alive=$(grep -oP 'alive=\K[0-9]+' "$log" 2>/dev/null | head -1)
    new_targets=$(grep -oP 'new_targets=\K[^ ]*' "$log" 2>/dev/null | head -1)
    fd0=${fd0:-0}; fd1=${fd1:-0}; alive=${alive:-0}; new_targets=${new_targets:-}
    if [ "$alive" -ne 1 ]; then
        record R4 FAIL "source died during reconnect cycles; see $log"; return
    fi
    local growth=$(( fd1 - fd0 ))
    # Real leak detector: anything in fd1 that isn't in fd0, after settle.
    # Counts can differ by lazy resources (e.g. /dev/rga opens the first
    # time a LIVE frame is broadcast); only count NEW socket-like fds as
    # a leak. Strip benign device opens before reporting.
    local leaky_targets
    leaky_targets=$(echo "$new_targets" | tr '|' '\n' | grep -E 'socket:|/dmabuf:' | head -5 | tr '\n' '|')
    if [ -n "$leaky_targets" ]; then
        record R4 FAIL "fd leak fd0=$fd0 → fd1=$fd1 (new: $leaky_targets, real bug — see scm_rights_producer eviction)"; return
    fi
    record R4 PASS "fd0=$fd0 fd1=$fd1 (growth=$growth, alive=yes, 5 cycles; non-leaky new fds: ${new_targets:-none})"
}


R6_mjpeg_uvc() {
    scenario_selected R6 || return
    scenario "R6 mjpeg-uvc (rig)"
    # Detect a UVC MJPEG device on the rig.
    local dev
    dev=$($RIG_SSH -o ConnectTimeout=3 "$RIG" '
        for v in /dev/video*; do
            v4l2-ctl -d "$v" --list-formats 2>/dev/null | grep -qE "MJPG|MJPEG" && { echo "$v"; exit 0; }
        done
        exit 0' 2>/dev/null)
    if [ -z "$dev" ]; then
        record R6 SKIP "no UVC MJPEG device on rig (plug in a USB camera)"; return
    fi
    local sock="/tmp/vn-smoke-r6.sock"
    local y4m="$RIG_ARTIFACTS_DIR/R6.y4m"
    local script
    script=$(cat <<EOF
set -uo pipefail
rm -f $sock $y4m
SRC=\$(ls $RIG_BUILD/src/videonode-source $RIG_BUILD/src/bin/videonode-source 2>/dev/null | head -1)
SINK=\$(ls $RIG_BUILD/src/videonode-sink $RIG_BUILD/src/bin/videonode-sink 2>/dev/null | head -1)
"\$SRC" --device $dev --in-format mjpeg --out-socket $sock --seconds $((DURATION+2)) \
    >$RIG_ARTIFACTS_DIR/R6.src.log 2>&1 &
SRC_PID=\$!
for i in {1..40}; do [ -S $sock ] && break; sleep 0.1; done
[ -S $sock ] || { echo "FAIL: socket"; kill \$SRC_PID 2>/dev/null; exit 1; }
timeout $((DURATION+1)) "\$SINK" --socket $sock --first-frame-timeout 10 \
    >$y4m 2>$RIG_ARTIFACTS_DIR/R6.sink.log
kill \$SRC_PID 2>/dev/null; wait \$SRC_PID 2>/dev/null
FRAMES=\$(grep -ac '^FRAME\$' $y4m 2>/dev/null || echo 0)
echo "RESULT dev=$dev frames=\$FRAMES"
EOF
)
    local result_line
    result_line=$(rig_exec R6 "DURATION=$DURATION; $script")
    local log="${result_line%:*}"
    local frames
    frames=$(grep -oP 'frames=\K[0-9]+' "$log" 2>/dev/null | head -1)
    frames=${frames:-0}
    if [ "$frames" -eq 0 ]; then
        record R6 FAIL "no MJPEG frames from $dev; see $log"; return
    fi
    record R6 PASS "dev=$dev frames=$frames over ${DURATION}s"
}


I1_ipc_canvas_perspective() {
    scenario_selected I1 || return
    scenario "I1 ipc-canvas-perspective (rig, daemon-driven composer + live re-warp)"

    # Smoke owns its videonode daemon: a fresh build of the working
    # tree at a smoke-only path with smoke-only config + ports + sockets.
    # Cross-compile + ship every run so the smoke always tests the
    # current code.
    local rig_bin="/tmp/smoke-vn/videonode"
    local local_bin="/tmp/videonode-arm64-smoke"
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go -C "$GO_MODULE_ROOT" build -o "$local_bin" . 2>"$ARTIFACTS_DIR/I1.build.log"
    local rc=$?
    if [ "$rc" -ne 0 ]; then
        record I1 FAIL "go build (arm64) failed; see $ARTIFACTS_DIR/I1.build.log"
        return
    fi
    # Ensure the rig scratch dir exists — rsync into a missing parent
    # was failing silently before this guard.
    $RIG_SSH -o ConnectTimeout=5 "$RIG" "mkdir -p /tmp/smoke-vn/data" >/dev/null 2>&1 || true
    rsync -az "$local_bin" "$RIG:$rig_bin" 2>"$ARTIFACTS_DIR/I1.rsync.log" \
        || { record I1 FAIL "rsync videonode to rig failed; see $ARTIFACTS_DIR/I1.rsync.log"; return; }

    # Write the rig-side script to a file and scp it over — using
    # `bash -s` with a heredoc-as-stdin breaks complex scripts (here-
    # strings + arithmetic + heredoc nesting) in transit.
    local script_file="$ARTIFACTS_DIR/I1.rig.sh"
    cat >"$script_file" <<'EOF'
#!/bin/bash
set -uo pipefail
RIG_BIN=/tmp/smoke-vn/videonode
WORKDIR=/tmp/smoke-vn
mkdir -p "$WORKDIR/data"
chmod +x "$RIG_BIN"

# Stop any prior smoke daemon + cascaded children.
for p in videonode-composer videonode-source videonode-sink; do pkill -9 -f "$p" 2>/dev/null; done
pkill -9 -f /tmp/smoke-vn/videonode 2>/dev/null
sleep 1

# Fresh streams.toml so the test starts from zero.
printf 'version = 1\n' > "$WORKDIR/streams.toml"
cat > "$WORKDIR/config.toml" <<'TOML'
[server]
port = ":8190"
[streams]
config_file = "/tmp/smoke-vn/streams.toml"
[streaming]
rtsp_port = ":8654"
[recording]
data_dir = "/tmp/smoke-vn/data"
[auth]
type = "basic"
username = "smoke"
password = "smoke"
[update]
enabled = false
[logging]
level = "info"
format = "text"
TOML

cd "$WORKDIR"
setsid "$RIG_BIN" \
    -c "$WORKDIR/config.toml" \
    --port :8190 \
    --streaming-rtsp-port :8654 \
    --srt-addr :6101 \
    --streams-config-file "$WORKDIR/streams.toml" \
    --recording-data-dir "$WORKDIR/data" \
    --native-composer /home/orangepi/composer/build/src/bin/videonode-composer \
    --native-v4-l2-source /home/orangepi/composer/build/src/bin/videonode-source \
    --native-vn-sink /home/orangepi/composer/build/src/bin/videonode-sink \
    </dev/null >"$WORKDIR/daemon.log" 2>&1 &
disown
sleep 4

API=http://127.0.0.1:8190
AUTH=smoke:smoke

curl -fsS -u "$AUTH" "$API/api/health" >/dev/null \
    || { echo "STAGE=health FAIL"; tail -10 "$WORKDIR/daemon.log"; exit 1; }

curl -fsS -u "$AUTH" -X POST "$API/api/streams" -H "Content-Type: application/json" \
  -d '{"stream_id":"hdmi","device_id":"platform-fdee0000.hdmirx-controller-video-index0","codec":"h264","input_format":"nv12","width":3840,"height":2160,"framerate":60}' \
  >/dev/null || { echo "STAGE=create_source FAIL"; exit 1; }
curl -fsS -u "$AUTH" -X POST "$API/api/streams" -H "Content-Type: application/json" \
  -d '{"stream_id":"canvas","codec":"h264","canvas":{"width":1920,"height":1080,"fps":"30","source_streams":["hdmi"]}}' \
  >/dev/null || { echo "STAGE=create_canvas FAIL"; exit 1; }
curl -sS -u "$AUTH" -X POST "$API/api/streams/canvas/canvas/engage" >/dev/null
sleep 6

# Composer must have spawned + identified, and the daemon must have
# pushed the initial config over IPC.
COMP_PID_BEFORE=$(pgrep -f "videonode-composer --drm-device" | sort -n | tail -1)
[ -z "$COMP_PID_BEFORE" ] && { echo "STAGE=composer_spawn FAIL"; exit 1; }
# The gRPC migration replaced the JSON-RPC `client identified` log line
# with `composer registered` (the daemon now dials the composer's UDS
# and calls Describe() to seed identity).
grep -q 'pipelinectl: composer registered' "$WORKDIR/daemon.log" \
    || { echo "STAGE=identify FAIL"; exit 1; }
grep -q 'composer initial config pushed' "$WORKDIR/daemon.log" \
    || { echo "STAGE=initial_push FAIL"; exit 1; }

LOG_OFFSET=$(stat -c%s "$WORKDIR/daemon.log")
curl -fsS -u "$AUTH" -X PATCH "$API/api/streams/hdmi" -H "Content-Type: application/json" \
  -d '{"perspective":{"corners":[[0,180],[1919,0],[1919,1079],[0,899]],"snapshot_width":1920,"snapshot_height":1080}}' \
  >/dev/null || { echo "STAGE=patch FAIL"; exit 1; }
sleep 2

COMP_PID_AFTER=$(pgrep -f "videonode-composer --drm-device" | sort -n | tail -1)
if [ "$COMP_PID_BEFORE" != "$COMP_PID_AFTER" ]; then
    echo "STAGE=no_restart FAIL (before=$COMP_PID_BEFORE after=$COMP_PID_AFTER)"
    exit 1
fi
tail -c +$((LOG_OFFSET + 1)) "$WORKDIR/daemon.log" \
    | grep -q "perspective updated via IPC; canvas not restarted" \
    || { echo "STAGE=live_push FAIL"; exit 1; }

# RTSP frame check — composer + ffmpeg are encoding real h264.
# Capture 60 BGRA frames over 2s; 80% of expected = pass.
RTSP_SECS=2
RTSP_FRAMES=$((30 * RTSP_SECS))
EXPECT=$((1920 * 1080 * 4 * RTSP_FRAMES))
ffmpeg -hide_banner -loglevel error -rtsp_transport tcp -i rtsp://127.0.0.1:8654/canvas \
    -frames:v $RTSP_FRAMES -pix_fmt bgra -f rawvideo "$WORKDIR/sample.bgra" -y 2>"$WORKDIR/rtsp.log"
SZ=$(stat -c%s "$WORKDIR/sample.bgra" 2>/dev/null || echo 0)
if [ "$SZ" -lt $((EXPECT * 80 / 100)) ]; then
    echo "STAGE=rtsp_frames FAIL (got $SZ / expected $EXPECT bytes)"
    exit 1
fi

# Codec assertion via ffprobe — confirms ffmpeg/h264_rkmpp wired to RTSP
# correctly. ffprobe peeks one packet from the live stream.
CODEC=$(ffprobe -hide_banner -v error -rtsp_transport tcp -timeout 5000000 \
    -select_streams v:0 -show_entries stream=codec_name \
    -of default=noprint_wrappers=1:nokey=1 rtsp://127.0.0.1:8654/canvas 2>"$WORKDIR/ffprobe.log" | head -1)
if [ "$CODEC" != "h264" ]; then
    echo "STAGE=codec FAIL (got codec='$CODEC')"
    exit 1
fi

# Frame-rate floor: bytes captured / pixel-size / seconds. We're at 30fps
# canvas; accept ≥70% to absorb startup transients in a 2s window.
OBS_FPS=$(( SZ / (1920*1080*4) / RTSP_SECS ))
FLOOR=$(( 30 * 7 / 10 ))
if [ "$OBS_FPS" -lt $FLOOR ]; then
    echo "STAGE=fps FAIL (got ${OBS_FPS} < floor ${FLOOR})"
    exit 1
fi

echo "RESULT comp_pid=$COMP_PID_AFTER bytes=$SZ obs_fps=$OBS_FPS codec=$CODEC"
EOF
    chmod +x "$script_file"
    local rig_script="/tmp/smoke-vn/I1.rig.sh"
    $RIG_SSH "$RIG" "mkdir -p /tmp/smoke-vn"
    scp -q "$script_file" "$RIG:$rig_script" || {
        record I1 FAIL "scp I1.rig.sh to rig failed"; return; }
    local log="$ARTIFACTS_DIR/I1.log"
    $RIG_SSH "$RIG" "bash $rig_script" >"$log" 2>&1
    if grep -q '^STAGE=.*FAIL' "$log" 2>/dev/null; then
        local stage
        stage=$(grep -oP 'STAGE=\K\S+' "$log" | head -1)
        record I1 FAIL "stage=$stage (see $log)"
        return
    fi
    local comp_pid bytes obs_fps codec
    comp_pid=$(grep -oP 'comp_pid=\K[0-9]+' "$log" 2>/dev/null | head -1)
    bytes=$(grep -oP 'bytes=\K[0-9]+' "$log" 2>/dev/null | head -1)
    obs_fps=$(grep -oP 'obs_fps=\K[0-9]+' "$log" 2>/dev/null | head -1)
    codec=$(grep -oP 'codec=\K\S+' "$log" 2>/dev/null | head -1)
    if [ -z "$comp_pid" ]; then
        record I1 FAIL "no RESULT line; see $log"
        return
    fi
    record I1 PASS "no-restart pid=$comp_pid; codec=${codec:-?}; fps=${obs_fps:-?}/30; rtsp_bytes=${bytes:-0}"
}

I2_ipc_resource_usage() {
    scenario_selected I2 || return
    scenario "I2 ipc-resource-usage (rig, 10s daemon-driven pipeline sampling)"
    # Mirrors R5 but exercises the daemon-driven path: a smoke daemon
    # supervises videonode-source + videonode-composer + ffmpeg. We
    # sample RSS+CPU for source and composer, then assert pipe survival
    # + soft RSS bounds. Composer here is the one I1 spawned (we reuse
    # the running daemon when present).
    local helpers
    helpers=$(rig_helpers_for I2)
    local SECS=10
    local script
    script=$(cat <<EOF
set -uo pipefail
$helpers
API=http://127.0.0.1:8190
AUTH=smoke:smoke
# Reuse the smoke daemon if I1 left it running; otherwise relaunch.
if ! curl -fsS -u "\$AUTH" "\$API/api/health" >/dev/null 2>&1; then
    echo "FAIL: smoke daemon not running (run I1 first or share start.sh)"
    exit 1
fi
SRC_PID=\$(pgrep -f "videonode-source --device /dev/v4l/by-path" | sort -n | tail -1)
COMP_PID=\$(pgrep -f "videonode-composer --drm-device" | sort -n | tail -1)
if [ -z "\$SRC_PID" ] || [ -z "\$COMP_PID" ]; then
    echo "FAIL: source/composer not running (src=\$SRC_PID comp=\$COMP_PID)"
    exit 1
fi
START=\$(date +%s)
start_sampler source \$SRC_PID
start_sampler composer \$COMP_PID
SAMPLE_END=\$((START + $SECS))
while [ \$(date +%s) -lt \$SAMPLE_END ]; do
    kill -0 \$SRC_PID 2>/dev/null || { echo "FAIL: source died at t=\$((\$(date +%s) - START))s"; exit 1; }
    kill -0 \$COMP_PID 2>/dev/null || { echo "FAIL: composer died at t=\$((\$(date +%s) - START))s"; exit 1; }
    sleep 0.5
done
END=\$(date +%s)
RAN_SECS=\$((END - START))
# Samplers stop when the target PID exits. We didn't kill anything, so
# pkill them to flush the sample lines.
kill \$SRC_PID 2>/dev/null
kill \$COMP_PID 2>/dev/null
stop_samplers
echo "RESULT ran_secs=\$RAN_SECS"
EOF
)
    local result_line
    result_line=$(rig_exec I2 "$script")
    local log="${result_line%:*}"
    if grep -q "^FAIL" "$log" 2>/dev/null; then
        local why
        why=$(grep "^FAIL" "$log" | head -1)
        record I2 FAIL "${why#FAIL: }"; return
    fi
    local res
    res=$(format_samples I2)
    if [ -z "$res" ]; then
        record I2 FAIL "no samples collected; see $log"; return
    fi
    local src_rss comp_rss
    src_rss=$(fetch_samples I2 | grep 'name=source' | grep -oP 'rss_kb=\K[0-9]+' | head -1)
    comp_rss=$(fetch_samples I2 | grep 'name=composer' | grep -oP 'rss_kb=\K[0-9]+' | head -1)
    src_rss=${src_rss:-0}; comp_rss=${comp_rss:-0}
    # Soft bounds (same as R5's): source <200 MB, composer <400 MB.
    if [ "$src_rss" -gt 204800 ] || [ "$comp_rss" -gt 409600 ]; then
        record I2 FAIL "RSS over bound | $res"
    else
        record I2 PASS "$res"
    fi
}


# --------------------------- main --------------------------------------------

printf 'smoke.sh — composer pipeline smoke test\n'
printf '  target=%s  duration=%ss  artifacts=%s\n' "$TARGET" "$DURATION" "$ARTIFACTS_DIR"

# Host target — no daemon, no composer; the host has no HDMI source so
# none of the rig scenarios apply. Reserved for future host-only checks.
if [ "$TARGET" = "host" ] || [ "$TARGET" = "both" ]; then
    :
fi

if [ "$TARGET" = "rig" ] || [ "$TARGET" = "both" ]; then
    if R_rig_alive; then
        if R_setup; then
            # R-prefix: no daemon. Raw binary smoke (source / sink only).
            R1_hdmiin_source_sink
            R4_consumer_reconnect
            R6_mjpeg_uvc
            # I-prefix: daemon-driven IPC validation. The smoke owns its
            # own ephemeral videonode instance (custom port, custom
            # sockets, custom streams.toml).
            I1_ipc_canvas_perspective
            I2_ipc_resource_usage
        else
            scenario "rig setup"
            record rig-setup FAIL "binaries missing on $RIG ($RIG_BUILD); run scripts/sync-to-rig.sh && scripts/build-on-rig.sh"
        fi
    else
        scenario "rig reachability"
        record rig-ssh SKIP "$RIG unreachable"
    fi
fi

# --------------------------- summary -----------------------------------------
fail_count=0; pass_count=0; skip_count=0
for s in "${RESULTS_STATUS[@]:-}"; do
    case "$s" in PASS) ((pass_count++)) ;; FAIL) ((fail_count++)) ;; SKIP) ((skip_count++)) ;; esac
done

printf '\n%s──── summary ────%s\n' "$C_DIM" "$C_RESET"
printf '  %s%d PASS%s  %s%d FAIL%s  %s%d SKIP%s\n' \
    "$C_GREEN" "$pass_count" "$C_RESET" \
    "$C_RED"   "$fail_count" "$C_RESET" \
    "$C_YELLOW" "$skip_count" "$C_RESET"

if [ "$fail_count" -gt 0 ]; then
    printf '\nartifacts kept in %s for inspection\n' "$ARTIFACTS_DIR"
    KEEP=1
    exit 1
fi
exit 0
