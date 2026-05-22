package streams

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
)

// snapshotPollInterval is how often captureNativeSnapshot retries when
// the source has been spawned but hasn't yet produced its first frame
// (Source.Snapshot returns codes.Unavailable). Matches the legacy
// SCM-blocking-read behavior which would wait for the next broadcast
// tick (~33ms at 30fps).
const snapshotPollInterval = 50 * time.Millisecond

// captureNativeSnapshot pulls a raw NV12 frame from videonode-source's
// broadcast loop via the Source.Snapshot gRPC RPC, then encodes it as
// JPEG using the same ffmpeg subprocess the daemon already uses
// elsewhere. Replaces the legacy SCM_RIGHTS dma-buf consumer path; the
// daemon no longer opens the data plane directly.
//
// On a fresh-spawn source whose orchestrator hasn't yet published its
// first LatestFrame, Source.Snapshot returns codes.Unavailable. We poll
// up to `timeout` so the very-first snapshot of a stream returns the
// real first frame instead of a 503.
func captureNativeSnapshot(ctx context.Context, mgr *pipelinectl.Manager, deviceID string,
	timeout time.Duration,
) ([]byte, error) {
	if mgr == nil {
		return nil, fmt.Errorf("snapshot: nil control manager")
	}
	if deviceID == "" {
		return nil, fmt.Errorf("snapshot: empty device id")
	}
	deadline := time.Now().Add(timeout)
	rpcCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		resp, err := mgr.Snapshot(rpcCtx, deviceID)
		if err == nil {
			if len(resp.GetNv12()) == 0 || resp.GetWidth() == 0 || resp.GetHeight() == 0 {
				return nil, fmt.Errorf("snapshot: empty frame from source %s", deviceID)
			}
			return ffmpeg.EncodeNV12ToJPEG(resp.GetNv12(), int(resp.GetWidth()), int(resp.GetHeight()))
		}
		// Manager.Snapshot wraps codes.Unavailable into a "has no frame
		// yet" error string but doesn't preserve the gRPC code. We can
		// still detect transient-pre-first-frame via the message or by
		// re-querying status.FromError on a re-call. Simpler: a context
		// deadline + a simple poll regardless of error kind. If a hard
		// error (NotFound for unknown device, Internal, etc.) keeps
		// recurring we fall through the timeout the same way.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		// If the error is anything other than UNAVAILABLE / not-yet-ready,
		// surface immediately — retrying e.g. PermissionDenied is futile.
		if st, ok := status.FromError(err); ok && st.Code() != codes.Unavailable {
			// Manager.Snapshot wraps the original error; the wrapped
			// status.Code() unfolds correctly here.
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(snapshotPollInterval):
		}
	}
}
