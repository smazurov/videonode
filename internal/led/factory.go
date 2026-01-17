package led

import (
	"os"
	"strings"

	"github.com/smazurov/videonode/internal/logging"
)

const deviceTreeModelPath = "/proc/device-tree/model"

// New creates a new LED controller based on board detection
// Falls back to no-op controller if LEDs are not available.
func New(logger logging.Logger) Controller {
	boardModel := detectBoard()

	if logger != nil {
		logger.Info("Detecting board for LED control", "board_model", boardModel)
	}

	// Detect board type and return appropriate controller
	switch {
	case strings.Contains(boardModel, "NanoPC-T6"):
		if logger != nil {
			logger.Info("Detected NanoPC-T6, using sysfs LED controller")
		}
		return newSysfs(map[string]string{
			"user":   "usr_led",
			"system": "sys_led",
		})

	case strings.Contains(boardModel, "Orange Pi"):
		if logger != nil {
			logger.Info("Detected Orange Pi, using sysfs LED controller")
		}
		return newSysfs(map[string]string{
			"blue":  "blue_led",
			"green": "green_led",
		})

	case strings.Contains(boardModel, "Raspberry Pi"):
		if logger != nil {
			logger.Info("Detected Raspberry Pi, using sysfs LED controller")
		}
		return newSysfs(map[string]string{
			"act": "ACT",
		})

	default:
		if logger != nil {
			logger.Info("No LED support detected, using no-op controller", "board_model", boardModel)
		}
		return newNoop(logger)
	}
}

// detectBoard reads the device tree model to identify the board.
func detectBoard() string {
	data, err := os.ReadFile(deviceTreeModelPath)
	if err != nil {
		return "unknown"
	}

	// Device tree model contains null bytes, trim them
	model := strings.TrimRight(string(data), "\x00")
	return model
}
