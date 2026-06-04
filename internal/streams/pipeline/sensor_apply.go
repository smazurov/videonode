package pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// ApplySensor spawns or updates a per-sensor `videonode-sensor` process
// and registers it on the control plane (mirrors ApplyComposer). Called by the
// sensor reconciler when an auto_crop effect needs a tap. The analysis
// composer it dials must already exist (the Provisioner ensures it first).
func (p *Pipeline) ApplySensor(stage *SensorStage) error {
	if stage == nil || stage.SensorID == "" {
		return errors.New("pipeline: sensor stage with SensorID is required")
	}
	mu := p.entityLock("sensor:" + stage.SensorID)
	mu.Lock()
	defer mu.Unlock()

	if err := p.ensureUdsDir(); err != nil {
		return fmt.Errorf("pipeline: mkdir uds dir: %w", err)
	}
	if stage.BinaryPath == "" {
		stage.BinaryPath = p.cfg.VNSensorBin
	}
	if stage.GrpcUds == "" {
		stage.GrpcUds = GrpcSocketPathFor("sensor", stage.SensorID)
	}

	p.replaceStage(stage)
	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(stage.SensorID)
	}
	if err := p.restartStage(stage); err != nil {
		return err
	}
	if p.cfg.ControlServer != nil {
		go p.registerSensor(stage.SensorID, stage.GrpcUds)
	}
	return nil
}

func (p *Pipeline) registerSensor(sensorID, udsPath string) {
	const dialDeadline = 30 * time.Second
	deadline := time.Now().Add(dialDeadline)
	for {
		if time.Now().After(deadline) {
			p.logger.Warn("registerSensor: register never succeeded",
				logging.KeySensorID, sensorID, logging.KeyUDS, udsPath)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		err := p.cfg.ControlServer.RegisterSensor(ctx, sensorID, udsPath)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// SetSensorReconciler injects the auto_crop reconciler after construction
// (it depends on the pipeline, so it can't be passed in Config at NewPipeline
// time). Call once at startup before serving.
func (p *Pipeline) SetSensorReconciler(r SensorReconciler) {
	p.mu.Lock()
	p.cfg.SensorReconciler = r
	p.mu.Unlock()
}

// RemoveSensor stops the sensor process and tears down its stage +
// control-plane registration. No-op for unknown ids.
func (p *Pipeline) RemoveSensor(id string) error {
	if id == "" {
		return nil
	}
	mu := p.entityLock("sensor:" + id)
	mu.Lock()
	defer mu.Unlock()

	if p.cfg.ControlServer != nil {
		p.cfg.ControlServer.Unregister(id)
	}
	poolID := SensorPoolKey(id)
	if err := p.pool.Stop(poolID); err != nil {
		p.logger.Warn("RemoveSensor: pool.Stop failed", logging.KeyPoolID, poolID, logging.KeyError, err)
	}
	p.mu.Lock()
	delete(p.stages, poolID)
	p.mu.Unlock()
	return nil
}
