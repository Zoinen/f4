#include "../qt_quick_order_scratch.h"
#undef NDEBUG
#include <atomic>
#include <cassert>
#include <cstdlib>
#include <iostream>
#include <limits>
#include <new>
#include <random>
#include <thread>
#include <unordered_map>

static thread_local bool countAllocations = false;
static thread_local std::size_t allocations = 0;
void *operator new(std::size_t size) {
    if (countAllocations) ++allocations;
    if (void *p = std::malloc(size ? size : 1)) return p;
    throw std::bad_alloc();
}
void *operator new[](std::size_t size) { return ::operator new(size); }
void operator delete(void *p) noexcept { std::free(p); }
void operator delete[](void *p) noexcept { std::free(p); }
void operator delete(void *p, std::size_t) noexcept { std::free(p); }
void operator delete[](void *p, std::size_t) noexcept { std::free(p); }

int main() {
    using namespace F4OrderScratch;
    using L = Lease<int *>;
    std::array<int, maximumElements + 1> keys{};
    std::mt19937 random(61031);
    std::size_t comparisons = 0;
    for (int trial = 0; trial < 256; ++trial) {
        const int count = int(random() % 2048);
        auto lease = L::acquire(count);
        assert(lease);
        std::unordered_map<int *, int> reference;
        std::vector<int *> ordered;
        for (int i = 0; i < count; ++i) {
            int *key = &keys[random() % 4096];
            reference[key] = int(ordered.size());
            ordered.push_back(key);
            lease->append(key);
        }
        assert(lease->elements == ordered);
        for (int i = 0; i < 4096; ++i) {
            auto found = reference.find(&keys[i]);
            assert(lease->position(&keys[i]) == (found == reference.end() ? -1 : found->second));
            ++comparisons;
        }
    }
    {
        std::array<L, maximumLeases> live;
        for (std::size_t i = 0; i < live.size(); ++i) {
            live[i] = L::acquire(16);
            live[i]->append(&keys[i]);
        }
        assert(!L::acquire(16));
        auto moved = std::move(live[0]);
        assert(!live[0] && moved->position(&keys[0]) == 0);
        live[1] = std::move(moved);
        assert(!moved && live[1]->position(&keys[0]) == 0);
        live[1].release();
        live[0] = L::acquire(16);
        assert(live[0]->position(&keys[0]) == -1);
        live[0]->generation = std::numeric_limits<std::uint32_t>::max();
    }
    {
        auto lease = L::acquire(0);
        assert(lease && lease->generation == 1 && lease->position(&keys[0]) == -1);
    }
    assert(!L::acquire(maximumElements + 1));
    for (int pass = 0; pass < 2; ++pass) {
        std::array<L, maximumLeases> live;
        for (auto &lease : live) {
            lease = L::acquire(maximumElements);
            for (std::size_t i = 0; i < maximumElements; ++i) lease->append(&keys[i]);
        }
    }
    allocations = 0;
    countAllocations = true;
    for (int pass = 0; pass < 1000; ++pass) {
        std::array<L, maximumLeases> live;
        for (auto &lease : live) {
            lease = L::acquire(maximumElements);
            for (std::size_t i = 0; i < maximumElements; ++i) lease->append(&keys[i]);
            assert(lease->position(&keys[maximumElements - 1]) == int(maximumElements - 1));
        }
    }
    countAllocations = false;
    assert(allocations == 0);
    std::size_t bytes = 0;
    for (const auto &slot : poolSlots<int *>)
        bytes += slot.storage.elements.capacity() * sizeof(int *)
            + slot.storage.entries.capacity() * sizeof(Storage<int *>::Entry);
    assert(bytes <= maximumLeases * maximumElements * (sizeof(int *) + 2 * sizeof(Storage<int *>::Entry)));
    std::atomic_bool otherSucceeded = false;
    auto mainLease = L::acquire(1);
    mainLease->append(&keys[0]);
    std::thread worker([&] {
        auto lease = L::acquire(1);
        assert(lease->position(&keys[0]) == -1);
        lease->append(&keys[1]);
        otherSucceeded = lease->position(&keys[1]) == 0;
    });
    worker.join();
    assert(otherSucceeded && mainLease->position(&keys[0]) == 0 && mainLease->position(&keys[1]) == -1);
    std::cout << "{\"lookupComparisons\":" << comparisons
              << ",\"steadyStateSnapshots\":4000,\"steadyStateAllocations\":" << allocations
              << ",\"retainedBytes\":" << bytes << "}\n";
}
