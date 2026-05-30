#include "src/source/slot_owner.hpp"

namespace source {

SlotOwner::SlotOwner(size_t slots) {
    refcount_.reserve(slots);
    generation_.reserve(slots);
    for (size_t i = 0; i < slots; ++i) {
        refcount_.push_back(std::make_unique<std::atomic<int>>(0));
        generation_.push_back(std::make_unique<std::atomic<uint64_t>>(0));
    }
}

int SlotOwner::pick_free_slot() const {
    for (size_t i = 0; i < refcount_.size(); ++i) {
        if (refcount_[i]->load(std::memory_order_acquire) == 0)
            return static_cast<int>(i);
    }
    return -1;
}

uint64_t SlotOwner::begin_write(int idx) {
    return generation_[static_cast<size_t>(idx)]->fetch_add(1, std::memory_order_acq_rel) + 1;
}

void SlotOwner::add_refs(int idx, int n) {
    if (n > 0)
        refcount_[static_cast<size_t>(idx)]->fetch_add(n, std::memory_order_acq_rel);
}

int SlotOwner::refcount(int idx) const {
    return refcount_[static_cast<size_t>(idx)]->load(std::memory_order_acquire);
}

bool SlotOwner::pin(uint64_t slot, uint64_t generation) {
    if (!in_range(slot))
        return false;
    auto& rc = *refcount_[slot];
    rc.fetch_add(1, std::memory_order_acq_rel);
    if (generation_[slot]->load(std::memory_order_acquire) != generation) {
        rc.fetch_sub(1, std::memory_order_acq_rel);
        return false;
    }
    return true;
}

void SlotOwner::release(uint64_t slot, uint64_t generation) {
    if (!in_range(slot))
        return;
    if (generation_[slot]->load(std::memory_order_acquire) != generation)
        return;
    auto& rc = *refcount_[slot];
    int cur = rc.load(std::memory_order_acquire);
    while (cur > 0) {
        if (rc.compare_exchange_weak(cur, cur - 1, std::memory_order_acq_rel))
            return;
    }
}

} // namespace source
