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

// Offline maintenance tool, never executed by the application or a release
// build. Commit its SVG outputs so cross-compilation needs no host Qt tools.
int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    if (app.arguments().size() != 2)
        return 2;
    const QDir icons(app.arguments().at(1));
    QFile manifest(icons.filePath("files/names.txt"));
    if (!manifest.open(QIODevice::ReadOnly))
        return 3;
    const auto names = QString::fromUtf8(manifest.readAll()).split(u'\n', Qt::SkipEmptyParts);
    const QRegularExpression stroke(R"(stroke-width="[0-9.]+")");
    const QRegularExpression viewBox(R"(viewBox="[^"]+")");
    const QRegularExpression width(R"((?<![-\w])width="[^"]+")");
    const QRegularExpression height(R"(\bheight="[^"]+")");
    constexpr int extent = 4096;
    constexpr qreal canvasOrigin = -4;
    constexpr qreal canvasSpan = 32;
    for (const QString &name : names) {
        QFile original(icons.filePath("lucide/" + name + ".svg"));
        if (!original.open(QIODevice::ReadOnly))
            return 4;
        const QString source = QString::fromUtf8(original.readAll());
        for (int step = 1; step <= 8; ++step) {
            QString svg = source;
            svg.replace(stroke, QString("stroke-width=\"%1\"").arg(step / 2.0));
            QString probeSvg = svg;
            probeSvg.replace(viewBox, "viewBox=\"-4 -4 32 32\"");
            QSvgRenderer renderer(probeSvg.toUtf8());
            if (!renderer.isValid())
                return 5;
            QImage probe(extent, extent, QImage::Format_ARGB32_Premultiplied);
            probe.fill(Qt::transparent);
            QPainter painter(&probe);
            painter.setRenderHint(QPainter::Antialiasing);
            renderer.render(&painter, QRectF(0, 0, extent, extent));
            painter.end();
            int left = extent, top = extent, right = -1, bottom = -1;
            for (int y = 0; y < extent; ++y) {
                const auto *row = reinterpret_cast<const QRgb *>(probe.constScanLine(y));
                for (int x = 0; x < extent; ++x) {
                    if (qAlpha(row[x]) == 0)
                        continue;
                    left = std::min(left, x);
                    top = std::min(top, y);
                    right = std::max(right, x);
                    bottom = std::max(bottom, y);
                }
            }
            if (right < left || left == 0 || top == 0 || right == extent - 1 || bottom == extent - 1)
                return 6;
            const qreal unit = canvasSpan / extent;
            const qreal w = (right - left + 1) * unit;
            const qreal h = (bottom - top + 1) * unit;
            svg.replace(viewBox, QString("viewBox=\"%1 %2 %3 %4\"")
                .arg(canvasOrigin + left * unit, 0, 'g', 12)
                .arg(canvasOrigin + top * unit, 0, 'g', 12)
                .arg(w, 0, 'g', 12).arg(h, 0, 'g', 12));
            const int headerEnd = svg.indexOf(u'>') + 1;
            QString header = svg.left(headerEnd);
            header.replace(width, QString("width=\"%1\"").arg(w, 0, 'g', 12));
            header.replace(height, QString("height=\"%1\"").arg(h, 0, 'g', 12));
            svg = header + svg.mid(headerEnd);
            const QString directory = icons.filePath("files/" + QString::number(step));
            if (!QDir().mkpath(directory))
                return 7;
            QSaveFile output(directory + u'/' + name + ".svg");
            const QByteArray bytes = svg.toUtf8();
            if (!output.open(QIODevice::WriteOnly) || output.write(bytes) != bytes.size() || !output.commit())
                return 8;
        }
        QTextStream(stdout) << name << '\n';
    }
    return 0;
}
