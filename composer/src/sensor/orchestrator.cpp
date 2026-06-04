#include "src/sensor/orchestrator.hpp"

#include "src/sensor/sensor_service.hpp"
#include "src/sensor/detector_backend.hpp"
#include "src/common/log_levels.hpp"
#include "src/ipc/scm_rights_source.hpp"
#include "src/rpc/grpc_server.hpp"

#include <cerrno>
#include <cstring>
#include <ctime>
#include <deque>
#include <linux/dma-buf.h>
#include <optional>
#include <poll.h>
#include <span>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>
#include <utility>
#include <vector>

namespace sensor {

namespace {

int64_t now_monotonic_ns() {
    timespec ts{};
    ::clock_gettime(CLOCK_MONOTONIC, &ts);
    return static_cast<int64_t>(ts.tv_sec) * 1000000000LL + ts.tv_nsec;
}

void dmabuf_sync(int fd, uint64_t flags) {
    dma_buf_sync s{};
    s.flags = flags | DMA_BUF_SYNC_READ;
    (void)::ioctl(fd, DMA_BUF_IOCTL_SYNC, &s);
}

// Copies the Y plane out of a (cached) NV12 dma-buf into a tight w*h buffer.
bool extract_y(const scm_rights_source::OwnedFrameView& f, std::vector<uint8_t>& out) {
    if (f.fd.get() < 0 || f.width <= 0 || f.height <= 0)
        return false;
    const size_t w = static_cast<size_t>(f.width);
    const size_t h = static_cast<size_t>(f.height);
    const size_t pitch = f.plane0_pitch != 0 ? f.plane0_pitch : w;
    if (pitch < w)
        return false;
    const size_t map_size = f.plane0_offset + pitch * h;
    void* mapped = ::mmap(nullptr, map_size, PROT_READ, MAP_SHARED, f.fd.get(), 0);
    if (mapped == MAP_FAILED)
        return false;
    dmabuf_sync(f.fd.get(), DMA_BUF_SYNC_START);
    std::span<const uint8_t> base(static_cast<const uint8_t*>(mapped), map_size);
    std::span<const uint8_t> y = base.subspan(f.plane0_offset);
    out.resize(w * h);
    std::span<uint8_t> dst(out);
    for (size_t r = 0; r < h; ++r)
        std::memcpy(dst.subspan(r * w, w).data(), y.subspan(r * pitch, w).data(), w);
    dmabuf_sync(f.fd.get(), DMA_BUF_SYNC_END);
    ::munmap(mapped, map_size);
    return true;
}

void fill_finding(::videonode::control::Finding& out, const SensorContext& ctx, const Detection& d,
                  uint64_t frame_idx) {
    out.set_sensor_id(ctx.sensor_id);
    out.set_model_id(ctx.model_id);
    out.set_target_ref(ctx.target_ref);
    out.set_frame_idx(frame_idx);
    out.set_captured_at_ns(now_monotonic_ns());
    out.set_confidence(d.confidence);
    out.set_schema_version(ctx.schema_version);
    if (d.kind == "bbox") {
        auto* b = out.mutable_bbox();
        b->set_x(d.x);
        b->set_y(d.y);
        b->set_w(d.w);
        b->set_h(d.h);
    }
}

class SeqMap {
  public:
    void put(uint32_t seq, uint64_t frame_idx) {
        ring_.emplace_back(seq, frame_idx);
        while (ring_.size() > kCap)
            ring_.pop_front();
    }
    [[nodiscard]] std::optional<uint64_t> get(uint32_t seq) const {
        for (const auto& [s, fi] : ring_)
            if (s == seq)
                return fi;
        return std::nullopt;
    }

  private:
    static constexpr size_t kCap = 16;
    std::deque<std::pair<uint32_t, uint64_t>> ring_;
};

struct LoopState {
    bool detector_started = false;
    int frame_w = 0;
    int frame_h = 0;
    uint32_t next_seq = 1;
    uint64_t last_idx = 0;
    SeqMap seqmap;
    std::vector<uint8_t> ybuf;
};

bool ensure_detector(DetectorBackend& detector, const Args& a,
                     const scm_rights_source::OwnedFrameView& frame, LoopState& st) {
    if (st.detector_started && frame.width == st.frame_w && frame.height == st.frame_h)
        return true;
    detector.stop();
    if (!detector.start(a.detector, frame.width, frame.height)) {
        vn::log::error("sensor: detector failed to start");
        return false;
    }
    st.frame_w = frame.width;
    st.frame_h = frame.height;
    st.detector_started = true;
    vn::log::info("sensor: detector started %dx%d", st.frame_w, st.frame_h);
    return true;
}

void pump(const Args& a, scm_rights_source::ScmRightsSource& bus, DetectorBackend& detector,
          SensorService& svc, const SensorContext& ctx, LoopState& st) {
    auto frame = bus.latest_frame();
    if (frame.fd.get() < 0 || frame.frame_idx == 0 || frame.frame_idx == st.last_idx)
        return;
    if (!ensure_detector(detector, a, frame, st))
        return;
    if (!extract_y(frame, st.ybuf))
        return;
    st.last_idx = frame.frame_idx;
    if (detector.submit(st.next_seq, st.ybuf))
        st.seqmap.put(st.next_seq, frame.frame_idx);
    ++st.next_seq;
    if (auto det = detector.poll_detection(0)) {
        if (auto fi = st.seqmap.get(det->seq)) {
            ::videonode::control::Finding f;
            fill_finding(f, ctx, *det, *fi);
            svc.Publish(f);
        }
    }
    if (!detector.alive())
        st.detector_started = false;
}

void drain_notify(int nfd, int tick_ms) {
    if (nfd < 0)
        return;
    pollfd pfd{.fd = nfd, .events = POLLIN, .revents = 0};
    if (::poll(&pfd, 1, tick_ms) > 0 && (pfd.revents & POLLIN) != 0) {
        uint64_t v = 0;
        (void)::read(nfd, &v, sizeof(v));
    }
}

} // namespace

int Run(const Args& a, std::atomic<bool>& running) {
    if (a.scm_path.empty() || a.detector.empty()) {
        vn::log::fatal("sensor: --upstream-scm and --detector are required");
        return 2;
    }

    scm_rights_source::ScmRightsSource bus;
    scm_rights_source::InitParams ip;
    ip.socket_path = a.scm_path;
    ip.dial = true;
    if (!bus.init(ip) || !bus.start()) {
        vn::log::fatal("sensor: failed to dial %s", a.scm_path.c_str());
        return 1;
    }

    SensorContext ctx;
    ctx.sensor_id = a.sensor_id;
    ctx.model_id = a.model_id;
    ctx.target_ref = a.target_ref;
    ctx.running = &running;
    SensorService svc(&ctx);

    nativerpc::GrpcServer grpc;
    if (!a.grpc_listen.empty()) {
        std::vector<grpc::Service*> services{&svc};
        if (!grpc.Start(a.grpc_listen, services)) {
            vn::log::fatal("sensor: gRPC server failed on %s", a.grpc_listen.c_str());
            return 1;
        }
        vn::log::info("sensor: grpc on %s id=%s", a.grpc_listen.c_str(), a.sensor_id.c_str());
    }

    DetectorBackend detector;
    LoopState st;
    int nfd = bus.notify_fd();
    while (running.load()) {
        drain_notify(nfd, a.tick_ms);
        pump(a, bus, detector, svc, ctx, st);
    }

    svc.StopStreams();
    grpc.Shutdown();
    detector.stop();
    bus.stop();
    vn::log::info("sensor: shutdown");
    return 0;
}

} // namespace sensor
