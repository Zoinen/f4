#include "FixedWheelSchedule.h"
#include <cstdio>

using namespace FixedWheelSchedule;
static_assert(Count == 3000);
static_assert(decide(-StartDelayNs, 0).kind == Decision::Wait);
static_assert(decide(-StartDelayNs, 0).waitNs == StartDelayNs);
static_assert(decide(0, 0).kind == Decision::Dispatch);
static_assert(decide(PeriodNs-1, 1).waitNs == 1);
static_assert(decide(PeriodNs, 1).slot == 1);
static_assert(decide(3*PeriodNs+5, 1).slot == 3);
static_assert(decide(3*PeriodNs+5, 1).skipped == 2);
static_assert(!decide(149*PeriodNs, 149).upward);
static_assert(decide(150*PeriodNs, 150).upward);
static_assert(!decide(300*PeriodNs, 300).upward);
static_assert(decide(DurationNs-1, Count-1).slot == Count-1);
static_assert(decide(DurationNs, Count-1).kind == Decision::Finished);
static_assert(decide(DurationNs, Count-1).skipped == 1);
static_assert(decide(DurationNs+PeriodNs, Count-3).skipped == 3);
static_assert(decide(0, -1).kind == Decision::Invalid);
static_assert(decide(0, Count+1).kind == Decision::Invalid);
static_assert(timerDelayMs(1) == 1 && timerDelayMs(1'000'000) == 1);
static_assert(timerDelayMs(1'000'001) == 2 && timerDelayMs(0) == 0);

int main() {
    int delivered = 0, missing = 0, next = 0;
    for (Tick elapsed = 0; elapsed < DurationNs; elapsed += 3*PeriodNs) {
        const auto decision = decide(elapsed, next);
        if (decision.kind != Decision::Dispatch || decision.slot < next) return 1;
        delivered++;
        missing += decision.skipped;
        next = decision.slot+1;
    }
    const auto end = decide(DurationNs, next);
    missing += end.skipped;
    if (delivered != 1000 || missing != 2000 || delivered+missing != Count) return 2;
    std::puts("FixedWheelSchedule: boundary, direction, late-slot and conservation checks passed");
}
