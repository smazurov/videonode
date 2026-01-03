package systemd

import (
	"context"

	"github.com/coreos/go-systemd/v22/dbus"
)

// Manager handles systemd service lifecycle operations via D-Bus.
type Manager struct {
	conn *dbus.Conn
}

// NewManager creates a new systemd manager with a user-level D-Bus connection.
func NewManager(ctx context.Context) (*Manager, error) {
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	return &Manager{conn: conn}, nil
}

// GetServiceStatus retrieves the ActiveState property of a systemd service.
func (m *Manager) GetServiceStatus(ctx context.Context, serviceName string) (string, error) {
	prop, err := m.conn.GetUnitPropertyContext(ctx, serviceName, "ActiveState")
	if err != nil {
		return "", err
	}
	return prop.Value.String(), nil
}

// RestartService restarts a systemd service using the replace mode.
func (m *Manager) RestartService(ctx context.Context, serviceName string) error {
	_, err := m.conn.RestartUnitContext(ctx, serviceName, "replace", nil)
	return err
}

// StopService stops a systemd service using the replace mode.
func (m *Manager) StopService(ctx context.Context, serviceName string) error {
	_, err := m.conn.StopUnitContext(ctx, serviceName, "replace", nil)
	return err
}

// StartService starts a systemd service using the replace mode.
func (m *Manager) StartService(ctx context.Context, serviceName string) error {
	_, err := m.conn.StartUnitContext(ctx, serviceName, "replace", nil)
	return err
}

// Close cleanly closes the D-Bus connection.
func (m *Manager) Close() {
	if m.conn != nil {
		m.conn.Close()
	}
}
