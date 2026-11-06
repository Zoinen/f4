#pragma once
#include "CurrentMotionProbe20261031.h"
#include <QMetaEnum>
#include <QEasingCurve>

// Separate diagnostic storage: CurrentMotionProbe's frozen ABI is unchanged.
namespace WheelCurveProbe {
struct Sample {
    unsigned motionIndex = 0;
    qint64 ns = 0;
    quintptr job = 0;
    quint64 revision = 0;
    double animationMs = 0, beforeStartMs = 0, beforeFrom = 0, destination = 0;
    double afterStartMs = 0, afterFrom = 0;
    int durationMs = 0, curveType = 0;
    bool continuous = false;
};
inline std::array<Sample, 16384> samples;
inline std::atomic_uint count{0}, joinErrors{0};

inline void reset() {
    for (auto &sample : samples) sample = {};
    count.store(0); joinErrors.store(0);
}

inline void append(quintptr receiver, Sample sample) {
    if (!CurrentMotionProbe::enabled.load(std::memory_order_relaxed)
        || receiver != CurrentMotionProbe::target.load(std::memory_order_relaxed)) return;
    const auto motionCount = CurrentMotionProbe::count.load(std::memory_order_relaxed);
    if (!motionCount || motionCount > CurrentMotionProbe::samples.size()) {
        ++joinErrors; return;
    }
    const auto index = count.load(std::memory_order_relaxed);
    const auto &motion = CurrentMotionProbe::samples[motionCount-1];
    if (motion.job != sample.job || motion.revision != sample.revision
        || motion.animationMs != sample.animationMs
        || (index && samples[index-1].motionIndex >= motionCount-1)) {
        ++joinErrors; return;
    }
    const auto slot = count.fetch_add(1, std::memory_order_relaxed);
    if (slot >= samples.size()) return;
    sample.motionIndex = motionCount-1;
    sample.ns = motion.ns;
    samples[slot] = sample;
}

inline QJsonObject snapshot() {
    QJsonArray rows;
    const auto n = count.load();
    const auto types = QMetaEnum::fromType<QEasingCurve::Type>();
    for (unsigned i=0; i<qMin(n, unsigned(samples.size())); ++i) {
        const auto &s = samples[i];
        const char *name = types.valueToKey(s.curveType);
        rows.append(QJsonObject{{"motionIndex",int(s.motionIndex)}, {"timeNs",double(s.ns)},
            {"job",double(s.job)}, {"revision",double(s.revision)},
            {"animationMs",s.animationMs}, {"beforeStartMs",s.beforeStartMs},
            {"beforeFrom",s.beforeFrom}, {"destination",s.destination},
            {"afterStartMs",s.afterStartMs}, {"afterFrom",s.afterFrom},
            {"durationMs",s.durationMs}, {"curveType",s.curveType},
            {"curve",QString::fromLatin1(name ? name : "Unknown")},
            {"continuous",s.continuous}});
    }
    return {{"count",int(n)}, {"capacity",int(samples.size())},
            {"overflow",n>samples.size()}, {"joinErrors",int(joinErrors.load())},
            {"samples",rows}};
}
}
