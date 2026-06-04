package pipelinectl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/smazurov/videonode/internal/logging"
	pb "github.com/smazurov/videonode/internal/streams/pipelinectl/pb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// FindingParams is the daemon-native shape of a perception Finding, decoupling
// consumers (the BindingRouter, GoldenSampler) from the generated proto type.
// Geometry is normalized [0,1].
type FindingParams struct {
	SensorID   string
	ModelID    string
	TargetRef  string
	FrameIdx   uint64
	Confidence float64
	Kind       string // "bbox" | "none"
	BBox       FindingBBox
}

// FindingBBox is the normalized axis-aligned region of a "bbox" finding.
type FindingBBox struct {
	X float64
	Y float64
	W float64
	H float64
}

// SetFindingHandler registers the callback fired for every Finding received on
// any sensor's StreamFindings stream. Set once before Start; not safe to
// change while findings are flowing.
func (m *Manager) SetFindingHandler(fn func(FindingParams)) {
	m.mu.Lock()
	m.onFinding = fn
	m.mu.Unlock()
}

func findingFromProto(f *pb.Finding) FindingParams {
	p := FindingParams{
		SensorID:   f.GetSensorId(),
		ModelID:    f.GetModelId(),
		TargetRef:  f.GetTargetRef(),
		FrameIdx:   f.GetFrameIdx(),
		Confidence: float64(f.GetConfidence()),
		Kind:       "none",
	}
	if b := f.GetBbox(); b != nil {
		p.Kind = "bbox"
		p.BBox = FindingBBox{
			X: float64(b.GetX()),
			Y: float64(b.GetY()),
			W: float64(b.GetW()),
			H: float64(b.GetH()),
		}
	}
	return p
}

// RegisterSensor dials the sensor's gRPC server, calls Describe() to
// capture identity, and starts the long-running StreamFindings goroutine that
// pumps Findings into the registered handler.
func (m *Manager) RegisterSensor(ctx context.Context, sensorID, udsPath string) error {
	if m.ctx == nil {
		return fmt.Errorf("pipelinectl: manager not started")
	}
	cc, err := m.dial(udsPath)
	if err != nil {
		return err
	}
	client := pb.NewSensorClient(cc)
	info, err := client.Describe(ctx, &emptypb.Empty{})
	if err != nil {
		_ = cc.Close()
		return fmt.Errorf("pipelinectl: describe sensor %s: %w", sensorID, err)
	}
	streamCtx, cancel := context.WithCancel(m.ctx)
	c := &nativeConn{
		id:              sensorID,
		kind:            "sensor",
		udsPath:         udsPath,
		cc:              cc,
		anaClient:       client,
		pid:             info.GetNative().GetPid(),
		version:         info.GetNative().GetVersion(),
		protocolVersion: info.GetNative().GetProtocolVersion(),
		statusCancel:    cancel,
		streamDone:      make(chan struct{}),
	}

	m.mu.Lock()
	if old, ok := m.sensors[sensorID]; ok {
		m.logger.Warn("pipelinectl: evicting prior sensor", logging.KeySensorID, sensorID)
		m.mu.Unlock()
		m.closeConn(old)
		m.mu.Lock()
	}
	m.sensors[sensorID] = c
	m.mu.Unlock()

	go m.runFindingsStream(streamCtx, c)

	m.logger.Debug("pipelinectl: sensor registered",
		logging.KeySensorID, sensorID, logging.KeyPID, info.GetNative().GetPid(),
		logging.KeyVersion, info.GetNative().GetVersion(), logging.KeyUDS, udsPath)
	return nil
}

func (m *Manager) runFindingsStream(ctx context.Context, c *nativeConn) {
	defer close(c.streamDone)
	backoff := 200 * time.Millisecond
	const backoffCap = 5 * time.Second
	lastGood := time.Now()
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Since(lastGood) > StaleStreamTimeout {
			m.logger.Warn("pipelinectl: sensor unresponsive — evicting",
				logging.KeySensorID, c.id, logging.KeyStaleFor, time.Since(lastGood))
			go m.Unregister(c.id)
			return
		}
		stream, err := c.anaClient.StreamFindings(ctx, &pb.StreamFindingsRequest{})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.logger.Warn("pipelinectl: StreamFindings failed; will retry",
				logging.KeySensorID, c.id, logging.KeyError, err, logging.KeyBackoff, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < backoffCap {
				backoff *= 2
				if backoff > backoffCap {
					backoff = backoffCap
				}
			}
			continue
		}
		backoff = 200 * time.Millisecond
		for {
			msg, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if ctx.Err() == nil {
					m.logger.Warn("pipelinectl: StreamFindings recv error",
						logging.KeySensorID, c.id, logging.KeyError, err)
				}
				break
			}
			lastGood = time.Now()
			params := findingFromProto(msg)
			if params.SensorID == "" {
				params.SensorID = c.id
			}
			m.mu.RLock()
			handler := m.onFinding
			m.mu.RUnlock()
			if handler != nil {
				handler(params)
			}
		}
	}
}
