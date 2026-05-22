package streams

import (
	"context"
	"fmt"
	"time"

	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
)

// captureNativeSnapshot pulls a raw NV12 frame from videonode-source's
// broadcast loop via the Source.Snapshot gRPC RPC, then encodes it as
// JPEG using the same ffmpeg subprocess the daemon already uses
// elsewhere. Replaces the legacy SCM_RIGHTS dma-buf consumer path; the
// daemon no longer opens the data plane directly.
func captureNativeSnapshot(ctx context.Context, mgr *pipelinectl.Manager, deviceID string,
	timeout time.Duration,
) ([]byte, error) {
	if mgr == nil {
		return nil, fmt.Errorf("snapshot: nil control manager")
	}
	if deviceID == "" {
		return nil, fmt.Errorf("snapshot: empty device id")
	}
	rpcCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := mgr.Snapshot(rpcCtx, deviceID)
	if err != nil {
		return nil, err
	}
	if len(resp.GetNv12()) == 0 || resp.GetWidth() == 0 || resp.GetHeight() == 0 {
		return nil, fmt.Errorf("snapshot: empty frame from source %s", deviceID)
	}
	return ffmpeg.EncodeNV12ToJPEG(resp.GetNv12(), int(resp.GetWidth()), int(resp.GetHeight()))
}
