#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QImage>
#include <QPainter>
#include <QRegularExpression>
#include <QSaveFile>
#include <QSvgRenderer>
#include <QTextStream>

#include <algorithm>

// Offline asset normalization; never run by the application or release build.
int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    if (app.arguments().size() != 2)
        return 2;
    const QDir directory(app.arguments().at(1));
    const QStringList names = directory.entryList({"*.svg"}, QDir::Files);
    if (names.isEmpty())
        return 3;
    const QRegularExpression viewBox(R"(viewBox="[^"]+")");
    constexpr int extent = 4096;
    for (const QString &name : names) {
        const QString path = directory.filePath(name);
        QFile input(path);
        if (!input.open(QIODevice::ReadOnly))
            return 4;
        QString source = QString::fromUtf8(input.readAll());
        input.close();
        QSvgRenderer original(source.toUtf8());
        if (!original.isValid() || !viewBox.match(source).hasMatch())
            return 5;
        const QRectF box = original.viewBoxF();
        const qreal span = std::max(box.width(), box.height()) * 2;
        const QPointF origin = box.center() - QPointF(span / 2, span / 2);
        QString probeSource = source;
        probeSource.replace(viewBox, QString("viewBox=\"%1 %2 %3 %3\"")
            .arg(origin.x(), 0, 'g', 12).arg(origin.y(), 0, 'g', 12)
            .arg(span, 0, 'g', 12));
        QSvgRenderer renderer(probeSource.toUtf8());
        QImage image(extent, extent, QImage::Format_ARGB32_Premultiplied);
        image.fill(Qt::transparent);
        QPainter painter(&image);
        painter.setRenderHint(QPainter::Antialiasing);
        renderer.render(&painter);
        painter.end();
        int left = extent, top = extent, right = -1, bottom = -1;
        for (int y = 0; y < extent; ++y) {
            const auto *row = reinterpret_cast<const QRgb *>(image.constScanLine(y));
            for (int x = 0; x < extent; ++x) {
                if (qAlpha(row[x]) == 0)
                    continue;
                left = std::min(left, x);
                top = std::min(top, y);
                right = std::max(right, x);
                bottom = std::max(bottom, y);
            }
        }
        if (right < left || left == 0 || top == 0
                || right == extent - 1 || bottom == extent - 1)
            return 6;
        const qreal unit = span / extent;
        const qreal width = (right - left + 1) * unit;
        const qreal height = (bottom - top + 1) * unit;
        source.replace(viewBox, QString("viewBox=\"%1 %2 %3 %4\"")
            .arg(origin.x() + left * unit, 0, 'g', 12)
            .arg(origin.y() + top * unit, 0, 'g', 12)
            .arg(width, 0, 'g', 12).arg(height, 0, 'g', 12));
        // Intrinsic dimensions must agree with viewBox; otherwise SVG loaders
        // can retain the old square aspect even with tightly cropped artwork.
        source.replace(QRegularExpression(R"(\bwidth="[^"]+")"),
                       QString("width=\"%1\"").arg(width, 0, 'g', 12));
        source.replace(QRegularExpression(R"(\bheight="[^"]+")"),
                       QString("height=\"%1\"").arg(height, 0, 'g', 12));
        QSaveFile output(path);
        const QByteArray bytes = source.toUtf8();
        if (!output.open(QIODevice::WriteOnly)
                || output.write(bytes) != bytes.size() || !output.commit())
            return 7;
        QTextStream(stdout) << "[FIX:zoin-padding] " << name << " artwork=" << (right - left + 1) * unit
            << 'x' << (bottom - top + 1) * unit << '\n';
    }
    return 0;
}
