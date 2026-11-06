#pragma once
#include <array>
#include <cstddef>
#include <cstdint>
#include <type_traits>
#include <utility>
#include <vector>

// Pointer identities are never dereferenced. A lease belongs to one render-list
// rebuild on its acquiring thread; nested rebuilds cannot reuse a live lease.
namespace F4OrderScratch {
constexpr std::size_t maximumElements = 16384;
constexpr std::size_t maximumLeases = 4;

template<class Key> struct Storage {
    static_assert(std::is_pointer_v<Key>);
    struct Entry { Key key = nullptr; int position = -1; std::uint32_t generation = 0; };
    std::vector<Key> elements;
    std::vector<Entry> entries;
    std::uint32_t generation = 0;

    void begin(std::size_t expected) {
        std::size_t capacity = 1;
        while (capacity < expected) capacity *= 2;
        if (elements.capacity() < capacity) elements.reserve(capacity);
        std::size_t buckets = 2;
        while (buckets < expected * 2) buckets *= 2;
        if (entries.size() < buckets) entries.resize(buckets);
        if (++generation == 0) {
            for (auto &entry : entries) entry.generation = 0;
            generation = 1;
        }
        elements.clear();
    }
    static std::size_t hash(Key key) {
        std::uintptr_t bits = reinterpret_cast<std::uintptr_t>(key);
        bits ^= bits >> 17;
        bits *= std::uintptr_t(0x9e3779b1U);
        bits ^= bits >> 13;
        return std::size_t(bits);
    }
    void append(Key key) {
        const auto mask = entries.size() - 1;
        std::size_t bucket = hash(key) & mask;
        while (entries[bucket].generation == generation && entries[bucket].key != key)
            bucket = (bucket + 1) & mask;
        entries[bucket] = {key, int(elements.size()), generation};
        elements.push_back(key);
    }
    int position(Key key) const {
        if (!generation) return -1;
        const auto mask = entries.size() - 1;
        std::size_t bucket = hash(key) & mask;
        while (entries[bucket].generation == generation) {
            if (entries[bucket].key == key) return entries[bucket].position;
            bucket = (bucket + 1) & mask;
        }
        return -1;
    }
};

template<class Key> struct Slot { Storage<Key> storage; bool leased = false; };
template<class Key> inline thread_local std::array<Slot<Key>, maximumLeases> poolSlots;

template<class Key> class Lease {
    Slot<Key> *slot = nullptr;
    explicit Lease(Slot<Key> *value): slot(value) {}
public:
    Lease() = default;
    Lease(const Lease &) = delete;
    Lease &operator=(const Lease &) = delete;
    Lease(Lease &&other) noexcept: slot(std::exchange(other.slot, nullptr)) {}
    Lease &operator=(Lease &&other) noexcept {
        if (this != &other) { release(); slot = std::exchange(other.slot, nullptr); }
        return *this;
    }
    ~Lease() { release(); }
    void release() {
        if (!slot) return;
        slot->storage.elements.clear();
        slot->leased = false;
        slot = nullptr;
    }
    explicit operator bool() const { return slot != nullptr; }
    Storage<Key> *operator->() { return &slot->storage; }
    const Storage<Key> *operator->() const { return &slot->storage; }
    static Lease acquire(std::size_t expected) {
        if (expected > maximumElements) return {};
        for (auto &candidate : poolSlots<Key>) {
            if (candidate.leased) continue;
            candidate.storage.begin(expected);
            candidate.leased = true;
            return Lease(&candidate);
        }
        // Preserve ordinary allocation when the bounded pool cannot serve it.
        return {};
    }
};
}
