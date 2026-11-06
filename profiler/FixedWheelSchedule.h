#pragma once
#include <cstdint>

namespace FixedWheelSchedule {
using Tick = std::int64_t;
constexpr Tick StartDelayNs = 200'000'000;
constexpr Tick PeriodNs = 10'000'000;
constexpr Tick DurationNs = 30'000'000'000;
constexpr int Count = int(DurationNs / PeriodNs);
constexpr int DirectionSlots = 150;

struct Decision {
    enum Kind { Wait, Dispatch, Finished, Invalid } kind;
    int slot;
    int skipped;
    Tick scheduledNs;
    Tick waitNs;
    bool upward;
};

// elapsedNs is measured from the planned first-input instant. Never replay
// overdue events in a burst: retain the skipped slot count and dispatch once.
constexpr Decision decide(Tick elapsedNs, int nextSlot) {
    if (nextSlot < 0 || nextSlot > Count)
        return {Decision::Invalid, nextSlot, 0, 0, 0, false};
    if (elapsedNs >= DurationNs || nextSlot == Count)
        return {Decision::Finished, Count, Count-nextSlot, DurationNs, 0, false};
    const Tick nextNs = Tick(nextSlot) * PeriodNs;
    if (elapsedNs < nextNs)
        return {Decision::Wait, nextSlot, 0, nextNs, nextNs-elapsedNs, false};
    const int slot = int(elapsedNs / PeriodNs);
    return {Decision::Dispatch, slot, slot-nextSlot, Tick(slot)*PeriodNs, 0,
            (slot / DirectionSlots) % 2 != 0};
}

constexpr int timerDelayMs(Tick waitNs) {
    return waitNs <= 0 ? 0 : int((waitNs + 999'999) / 1'000'000);
}
}
