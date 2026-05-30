// SlotOwner — per-slot in-flight refcount + generation for the source
// NV12 output ring. Gates slot reuse on consumer read-completion: a slot
// is only rewritten when its refcount is zero, so the producer can never
// lap a consumer that is still reading. Implements vn::snapshot::SlotPinner
// so the snapshot holder can pin the slot it reads.
#pragma once

#include "src/snapshot/snapshot.hpp"

#include <atomic>
#include <cstdint>
#include <memory>
#include <vector>

namespace source {

class SlotOwner : public vn::snapshot::SlotPinner {
  public:
    explicit SlotOwner(size_t slots);

    // Lowest-index slot with refcount 0, or -1 if all are in flight.
    [[nodiscard]] int pick_free_slot() const;

    // Bump the slot's generation for a fresh write; returns the new value.
    // Precondition: refcount[idx] == 0.
    uint64_t begin_write(int idx);

    // Add `n` outstanding readers to the slot (the broadcast sent count).
    void add_refs(int idx, int n);

    [[nodiscard]] int refcount(int idx) const;
    [[nodiscard]] size_t size() const { return refcount_.size(); }

    // SlotPinner — `slot`/`generation` may come off the wire; out-of-range
    // and kNoSlot are ignored. release drops one ref iff `generation`
    // matches (guards stale/duplicate credits and never underflows).
    [[nodiscard]] bool pin(uint64_t slot, uint64_t generation) override;
    void release(uint64_t slot, uint64_t generation) override;

  private:
    [[nodiscard]] bool in_range(uint64_t slot) const { return slot < refcount_.size(); }

    std::vector<std::unique_ptr<std::atomic<int>>> refcount_;
    std::vector<std::unique_ptr<std::atomic<uint64_t>>> generation_;
};

} // namespace source
