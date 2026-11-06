#include "WheelCurveProbe.h"
#include <QCoreApplication>
#include <QFile>
#include <QJsonDocument>
#include <cstdio>

int main(int argc, char **argv) {
    QCoreApplication app(argc, argv);
    if (argc != 2) return 1;
    CurrentMotionProbe::reset(7);
    WheelCurveProbe::reset();
    CurrentMotionProbe::enabled.store(true);
    CurrentMotionProbe::Sample motion;
    motion.job = 11; motion.revision = 3; motion.animationMs = 25;
    CurrentMotionProbe::append(7, motion);
    WheelCurveProbe::Sample curve;
    curve.job = 11; curve.revision = 3; curve.animationMs = 25;
    curve.destination = 100; curve.durationMs = 100;
    curve.curveType = int(QEasingCurve::OutSine);
    WheelCurveProbe::append(7, curve);
    if (WheelCurveProbe::count != 1 || WheelCurveProbe::joinErrors != 0
        || WheelCurveProbe::samples[0].ns != CurrentMotionProbe::samples[0].ns
        || WheelCurveProbe::samples[0].motionIndex != 0) return 2;
    const auto validSnapshot = WheelCurveProbe::snapshot();
    if (validSnapshot["samples"].toArray()[0].toObject()["curve"] != "OutSine") return 3;
    WheelCurveProbe::append(7, curve);
    if (WheelCurveProbe::joinErrors != 1 || WheelCurveProbe::count != 1) return 4;
    WheelCurveProbe::append(8, curve);
    if (WheelCurveProbe::joinErrors != 1 || WheelCurveProbe::count != 1) return 5;
    curve.job = 12;
    WheelCurveProbe::append(7, curve);
    if (WheelCurveProbe::joinErrors != 2 || WheelCurveProbe::count != 1) return 6;
    CurrentMotionProbe::count = unsigned(CurrentMotionProbe::samples.size()+1);
    WheelCurveProbe::append(7, curve);
    if (WheelCurveProbe::joinErrors != 3 || WheelCurveProbe::count != 1) return 7;
    CurrentMotionProbe::enabled.store(false);

    QJsonArray oracle;
    for (auto type : {QEasingCurve::Linear, QEasingCurve::OutSine, QEasingCurve::OutCubic}) {
        const QEasingCurve easing(type);
        const char *name = QMetaEnum::fromType<QEasingCurve::Type>().valueToKey(int(type));
        for (double progress : {-.1, 0., .125, .25, .5, .75, .9, 1., 1.1}) {
            const double expected = 12.5+(130.-12.5)*easing.valueForProgress(
                qBound<qreal>(0, progress, 1));
            oracle.append(QJsonObject{{"curve",QString::fromLatin1(name)},
                {"afterStartMs",5.}, {"afterFrom",12.5}, {"destination",130.},
                {"durationMs",150}, {"animationMs",5.+150.*progress},
                {"qtExpected",expected}});
        }
    }
    QFile file(QString::fromLocal8Bit(argv[1]));
    if (!file.open(QIODevice::WriteOnly)) return 8;
    const auto data = QJsonDocument(QJsonObject{{"nativeProbe",validSnapshot},
        {"expectedJoinRejections",3}, {"oracle",oracle}}).toJson();
    if (file.write(data) != data.size()) return 9;
    std::puts("WheelCurveProbe: indexed joins, duplicate/mismatch/overflow rejection and Qt oracle passed");
}
