package hostmetrics

import "github.com/smazurov/videonode/internal/logging"

// HostLoad is a single device-global hardware-utilization snapshot for the
// daemon (host) row. Any field is nil/empty when its hardware is absent.
type HostLoad struct {
	RKMPP []RKMPPCore  `json:"rkmpp,omitempty"`
	GPU   *DevfreqLoad `json:"gpu,omitempty"`
	NPU   *DevfreqLoad `json:"npu,omitempty"`
}

// Sample reads the current device-global hardware utilization. Presence-gated:
// each source independently yields nil/empty when its node is missing, so the
// result is simply empty on non-Rockchip / non-Mali hosts.
func Sample() HostLoad {
	cores, err := readRKMPPCores()
	if err != nil {
		logging.GetLogger("hostmetrics").Debug("read rkmpp load", logging.KeyError, err)
	}
	return HostLoad{
		RKMPP: cores,
		GPU:   readDevfreqLoad("gpu"),
		NPU:   readDevfreqLoad("npu"),
	}
}
