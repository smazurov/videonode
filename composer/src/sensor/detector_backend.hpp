#pragma once

#include "src/common/unique_fd.hpp"

#include <cstdint>
#include <mutex>
#include <optional>
#include <span>
#include <string>
#include <sys/types.h>
#include <vector>

namespace sensor {

// A normalized detection parsed from one detector-child response line.
struct Detection {
    uint32_t seq = 0;
    std::string kind = "none"; // "bbox" | "none"
    float confidence = 0.0F;
    // Normalized [0,1] bbox; valid only when kind == "bbox".
    float x = 0.0F;
    float y = 0.0F;
    float w = 0.0F;
    float h = 0.0F;
};

// DetectorBackend runs an out-of-process detector (e.g. a Python OpenCV
// script) over a full-duplex AF_UNIX socketpair: the child's stdin and
// stdout are both wired to one host fd. The host frames each gray8 Y plane
// as [u32 seq][u32 len][len bytes] and the child replies with one text line
// per frame: "<seq> <kind> <conf> [x y w h]\n". Frame submission is
// drop-on-busy (skip a tick if the prior frame hasn't drained) so a slow
// child throttles the sensor's rate instead of wedging it.
class DetectorBackend {
  public:
    DetectorBackend() = default;
    ~DetectorBackend();
    DetectorBackend(const DetectorBackend&) = delete;
    DetectorBackend& operator=(const DetectorBackend&) = delete;

    // shell_cmd runs under `/bin/sh -c`. width/height are exported to the
    // child as VN_WIDTH/VN_HEIGHT so it can reshape the Y plane.
    [[nodiscard]] bool start(const std::string& shell_cmd, int width, int height);

    // Non-blocking submit of one gray8 Y plane. Returns false if the prior
    // frame hasn't drained (caller drops this tick) or on a write error.
    [[nodiscard]] bool submit(uint32_t seq, std::span<const uint8_t> y_plane);

    // Blocks up to timeout for the next detection line. Returns nullopt on
    // timeout; the backend marks itself dead on EOF/error (see alive()).
    [[nodiscard]] std::optional<Detection> poll_detection(int timeout_ms);

    [[nodiscard]] bool alive() const;

    void stop();

  private:
    [[nodiscard]] bool spawn_(const std::string& shell_cmd, int width, int height);

    vn::base::unique_fd sock_;
    pid_t pid_ = -1;
    std::mutex write_mu_;
    std::vector<uint8_t> rx_; // accumulates partial child lines
};

} // namespace sensor
