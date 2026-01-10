// Package updater provides self-update functionality for videonode.
package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/smazurov/videonode/internal/version"
)

const (
	backupFilename     = "videonode.backup"
	backupInfoFilename = "backup.json"
)

type backupInfo struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	ExecPath  string    `json:"exec_path"`
}

type backupManager struct {
	mu        sync.RWMutex
	backupDir string
	info      *backupInfo
	logger    *slog.Logger
}

func newBackupManager(logger *slog.Logger) (*backupManager, error) {
	// Use ~/.cache/videonode/backup/ for backups
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	backupDir := filepath.Join(home, ".cache", "videonode", "backup")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	mgr := &backupManager{
		backupDir: backupDir,
		logger:    logger,
	}

	// Load existing backup info
	mgr.loadBackupInfo()

	return mgr, nil
}

func (m *backupManager) loadBackupInfo() {
	infoPath := filepath.Join(m.backupDir, backupInfoFilename)

	data, readErr := os.ReadFile(infoPath)
	if readErr != nil {
		return // No backup exists
	}

	var info backupInfo
	if err := json.Unmarshal(data, &info); err != nil {
		m.logger.Warn("Failed to parse backup info", "error", err)
		return
	}

	// Verify backup file exists
	backupPath := filepath.Join(m.backupDir, backupFilename)
	if _, statErr := os.Stat(backupPath); statErr != nil {
		m.logger.Warn("Backup file missing", "path", backupPath)
		return
	}

	m.mu.Lock()
	m.info = &info
	m.mu.Unlock()

	m.logger.Info("Loaded backup info", "version", info.Version)
}

func (m *backupManager) createBackup() error {
	execPath, err := selfupdate.ExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	backupPath := filepath.Join(m.backupDir, backupFilename)

	// Copy current executable to backup location
	src, openErr := os.Open(execPath)
	if openErr != nil {
		return fmt.Errorf("failed to open executable: %w", openErr)
	}
	defer src.Close()

	dst, createErr := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if createErr != nil {
		return fmt.Errorf("failed to create backup file: %w", createErr)
	}
	defer dst.Close()

	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		return fmt.Errorf("failed to copy executable: %w", copyErr)
	}

	// Save backup metadata
	info := backupInfo{
		Version:   version.Version,
		CreatedAt: time.Now(),
		ExecPath:  execPath,
	}

	infoPath := filepath.Join(m.backupDir, backupInfoFilename)
	infoData, marshalErr := json.Marshal(info)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal backup info: %w", marshalErr)
	}

	if err := os.WriteFile(infoPath, infoData, 0o644); err != nil {
		return fmt.Errorf("failed to write backup info: %w", err)
	}

	m.mu.Lock()
	m.info = &info
	m.mu.Unlock()

	m.logger.Info("Backup created", "version", info.Version, "path", backupPath)
	return nil
}

func (m *backupManager) restore() error {
	m.mu.RLock()
	info := m.info
	m.mu.RUnlock()

	if info == nil {
		return fmt.Errorf("no backup available")
	}

	backupPath := filepath.Join(m.backupDir, backupFilename)
	execPath := info.ExecPath

	// Copy backup back to executable location
	src, openErr := os.Open(backupPath)
	if openErr != nil {
		return fmt.Errorf("failed to open backup: %w", openErr)
	}
	defer src.Close()

	dst, createErr := os.OpenFile(execPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if createErr != nil {
		return fmt.Errorf("failed to open executable for restore: %w", createErr)
	}
	defer dst.Close()

	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		return fmt.Errorf("failed to restore backup: %w", copyErr)
	}

	m.logger.Info("Backup restored", "version", info.Version)
	return nil
}

func (m *backupManager) hasBackup() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.info != nil
}

func (m *backupManager) backupVersion() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.info == nil {
		return ""
	}
	return m.info.Version
}
