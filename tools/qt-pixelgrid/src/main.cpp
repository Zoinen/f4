#include "QtQuickPixelGrid.h"

#include <QtCore/QDir>
#include <QtCore/QFileInfo>
#include <QtCore/QStandardPaths>
#include <QtCore/QTimer>
#include <QtCore/QEventLoop>
#include <QtCore/QElapsedTimer>
#include <QtCore/QMetaObject>
#include <QtGui/QGuiApplication>
#include <QtGui/QImage>
#include <QtGui/QFont>
#include <QtGui/QPainter>
#include <QtGui/QScreen>
#include <QtQuick/QQuickImageProvider>
#include <QtQuick/QQuickItem>
#include <QtQuick/QQuickView>
#include <QtQuick/QQuickWindow>
#include <QtQuick/QSGRendererInterface>
#include <QtQuickControls2/QQuickStyle>
#include <QtQml/QQmlContext>
#include <QtQml/QQmlComponent>
#include <QtTest/QTest>

#include <cmath>
#include <algorithm>
#include <atomic>
#include <cstdio>
#include <iomanip>
#include <iostream>
#include <mutex>
#include <memory>
#include <vector>

class DemoState final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qreal motion READ motion WRITE setMotion NOTIFY motionChanged)
    Q_PROPERTY(bool animating READ animating WRITE setAnimating NOTIFY animatingChanged)
    Q_PROPERTY(int widthDelta READ widthDelta WRITE setWidthDelta NOTIFY widthDeltaChanged)
    Q_PROPERTY(QString content READ content NOTIFY contentChanged)
    Q_PROPERTY(int captionPixelSize READ captionPixelSize NOTIFY captionPixelSizeChanged)
    Q_PROPERTY(QString imageSource READ imageSource NOTIFY imageSourceChanged)
    Q_PROPERTY(int unsupportedCount READ unsupportedCount NOTIFY unsupportedCountChanged)
    Q_PROPERTY(bool viewerShaderSnap READ viewerShaderSnap WRITE setViewerShaderSnap NOTIFY viewerShaderSnapChanged)

public:
    qreal motion() const { return m_motion; }
    bool animating() const { return m_animating; }
    int widthDelta() const { return m_widthDelta; }
    QString content() const { return m_content; }
    int captionPixelSize() const { return m_captionPixelSize; }
    QString imageSource() const { return m_imageSource; }
    int unsupportedCount() const { return m_unsupportedCount; }
    bool viewerShaderSnap() const { return m_viewerShaderSnap; }

    void setViewerShaderSnap(bool enabled)
    {
        if (m_viewerShaderSnap == enabled)
            return;
        m_viewerShaderSnap = enabled;
        emit viewerShaderSnapChanged();
    }

    void setMotion(qreal value)
    {
        if (qFuzzyCompare(m_motion, value))
            return;
        m_motion = value;
        emit motionChanged();
    }

    void setAnimating(bool value)
    {
        if (m_animating == value)
            return;
        m_animating = value;
        emit animatingChanged();
    }

    void setWidthDelta(int value)
    {
        if (m_widthDelta == value)
            return;
        m_widthDelta = value;
        emit widthDeltaChanged();
    }

    void setUnsupportedCount(int value)
    {
        if (m_unsupportedCount == value)
            return;
        m_unsupportedCount = value;
        emit unsupportedCountChanged();
    }

    Q_INVOKABLE void move(qreal amount) { setMotion(m_motion + amount); }
    Q_INVOKABLE void resizeBy(int amount) { setWidthDelta(m_widthDelta + amount); }
    Q_INVOKABLE void changeContent()
    {
        m_content = m_content == QStringLiteral("Pixel aligned text")
            ? QStringLiteral("Subpixel 175% check")
            : QStringLiteral("Pixel aligned text");
        m_captionPixelSize = m_captionPixelSize == 9 ? 11 : 9;
        m_imageSource = m_imageSource == QStringLiteral("image://pixel/icon")
            ? QStringLiteral("image://pixel/alternate")
            : QStringLiteral("image://pixel/icon");
        emit contentChanged();
        emit captionPixelSizeChanged();
        emit imageSourceChanged();
    }

    Q_INVOKABLE void saveCapture(QQuickWindow *window, bool aligned)
    {
        if (!window)
            return;
        const QImage capture = window->grabWindow();
        if (capture.isNull())
            return;
        const QString dir = QDir(QCoreApplication::applicationDirPath()).filePath("captures");
        QDir().mkpath(dir);
        const QString name = QStringLiteral("%1-dpr-%2.png")
            .arg(aligned ? QStringLiteral("pixel-aligned") : QStringLiteral("original"))
            .arg(window->effectiveDevicePixelRatio(), 0, 'f', 3);
        const QString path = QDir(dir).filePath(name);
        if (capture.save(path))
            qInfo().noquote() << "Saved" << path << capture.size();
    }

signals:
    void motionChanged();
    void animatingChanged();
    void widthDeltaChanged();
    void contentChanged();
    void captionPixelSizeChanged();
    void imageSourceChanged();
    void unsupportedCountChanged();
    void viewerShaderSnapChanged();

private:
    qreal m_motion = 0;
    bool m_animating = false;
    int m_widthDelta = 0;
    QString m_content = QStringLiteral("Pixel aligned text");
    int m_captionPixelSize = 9;
    QString m_imageSource = QStringLiteral("image://pixel/icon");
    int m_unsupportedCount = -1;
    bool m_viewerShaderSnap = true;
};

class PixelIconProvider final : public QQuickImageProvider
{
public:
    PixelIconProvider() : QQuickImageProvider(QQuickImageProvider::Image) {}

    QImage requestImage(const QString &id, QSize *size, const QSize &) override
    {
        if (id.startsWith(QStringLiteral("border/"))) {
            bool ok = false;
            const int pixels = id.sliced(7).toInt(&ok);
            if (!ok || pixels < 3 || pixels > 512)
                return {};
            // The URL specifies physical texels. Ignore requestedSize: Qt scales
            // that logical, integer size by DPR, which can introduce rounding.
            QImage image(pixels, pixels, QImage::Format_ARGB32_Premultiplied);
            image.fill(Qt::white);
            QPainter painter(&image);
            const QColor border("#00bfff");
            // Filled integer rectangles avoid a pen centered on a half texel.
            painter.fillRect(0, 0, pixels, 1, border);
            painter.fillRect(0, pixels - 1, pixels, 1, border);
            painter.fillRect(0, 1, 1, pixels - 2, border);
            painter.fillRect(pixels - 1, 1, 1, pixels - 2, border);
            if (size)
                *size = image.size();
            return image;
        }
        const bool alternate = id.contains(QStringLiteral("alternate"));
        QImage image(16, 16, QImage::Format_ARGB32_Premultiplied);
        image.fill(Qt::transparent);
        QPainter painter(&image);
        painter.setRenderHint(QPainter::Antialiasing, false);
        painter.fillRect(QRect(1, 1, 14, 14), alternate ? QColor("#81d4a7") : QColor("#f3d15b"));
        painter.fillRect(QRect(3, 3, 4, 4), alternate ? QColor("#274e38") : QColor("#315875"));
        painter.fillRect(QRect(9, 3, 4, 4), alternate ? QColor("#274e38") : QColor("#315875"));
        painter.fillRect(QRect(5, 8, 6, 3), alternate ? QColor("#704aa0") : QColor("#9d4e45"));
        painter.fillRect(QRect(3, 12, 10, 2), QColor("#315875"));
        if (size)
            *size = image.size();
        return image;
    }
};

static QQuickView *createView(DemoState *state, bool aligned, const QRect &geometry,
                              bool showWindow)
{
    auto *view = new QQuickView;
    view->setScreen(QGuiApplication::primaryScreen());
    view->engine()->setImportPathList({ QStringLiteral("qrc:/qt/qml"),
                                       QStringLiteral("qrc:/qt-project.org/imports") });
    view->engine()->addImageProvider(QStringLiteral("pixel"), new PixelIconProvider);
    view->rootContext()->setContextProperty(QStringLiteral("demoState"), state);
    view->rootContext()->setContextProperty(QStringLiteral("pixelAligned"), aligned);
    view->rootContext()->setContextProperty(QStringLiteral("shaderRenderingAvailable"),
        view->rendererInterface()->graphicsApi() != QSGRendererInterface::Software);
    view->setResizeMode(QQuickView::SizeRootObjectToView);
    view->setSource(QUrl(QStringLiteral("qrc:/qt/qml/QtPixelGrid/Proof/Main.qml")));
    view->setTitle(aligned ? QStringLiteral("Qt pixel-grid proof — aligned")
                           : QStringLiteral("Qt pixel-grid proof — original"));
    view->setGeometry(geometry);
    if (view->status() == QQuickView::Error) {
        for (const QQmlError &error : view->errors())
            qCritical().noquote() << error.toString();
    }
    if (showWindow)
        view->show();
#ifdef QT_PIXELGRID_PATCHED_QT
    QtQuickPixelGrid::setEnabled(view, aligned);
    if (aligned) {
        state->setUnsupportedCount(QtQuickPixelGrid::unsupportedItems(view).size());
        for (const QString &name : QtQuickPixelGrid::unsupportedItems(view))
            qWarning().noquote() << "Unsupported pixel-grid subtree:" << name;
    }
#else
    Q_UNUSED(aligned);
#endif
    return view;
}

static QQuickItem *namedItem(QQuickView *view, const char *name)
{
    if (!view || !view->rootObject())
        return nullptr;
    const QString objectName = QString::fromLatin1(name);
    if (view->rootObject()->objectName() == objectName)
        return view->rootObject();
    return view->rootObject()->findChild<QQuickItem *>(objectName);
}

static QPointF mapOrigin(QQuickItem *item, QQuickWindow *window)
{
    return item->mapToItem(window->contentItem(), QPointF(0, 0));
}

static QString checkRasterIcons(QQuickView *view, const QImage &capture,
                                bool expectAligned, const char *stage)
{
    const struct {
        const char *name;
        int pixels;
    } fixtures[] = {
        { "borderIconDepth0Leaf", 48 }, { "borderIconDepth1Leaf", 56 },
        { "borderIconDepth2Leaf", 64 }, { "borderIconDepth4Leaf", 72 }
    };
    const qreal dpr = view->effectiveDevicePixelRatio();
    const QRgb border = qRgb(0, 191, 255);
    const QRgb white = qRgb(255, 255, 255);
    const QRgb background = qRgb(18, 23, 27);
    auto matches = [](QRgb actual, QRgb expected) {
        return std::abs(qRed(actual) - qRed(expected)) <= 1
            && std::abs(qGreen(actual) - qGreen(expected)) <= 1
            && std::abs(qBlue(actual) - qBlue(expected)) <= 1
            && qAlpha(actual) == qAlpha(expected);
    };
    int blurredIcons = 0;
    PixelIconProvider provider;
    for (const auto &fixture : fixtures) {
        QQuickItem *item = namedItem(view, fixture.name);
        if (!item || item->property("status").toInt() != 1)
            return QStringLiteral("Raster fixture %1 did not load").arg(fixture.name);
        const int n = fixture.pixels;
        if (std::abs(item->width() * dpr - n) > 0.001
            || std::abs(item->height() * dpr - n) > 0.001)
            return QStringLiteral("Raster fixture %1 is not displayed at native texel size")
                .arg(fixture.name);
        QSize reportedSize;
        const QImage source = provider.requestImage(QStringLiteral("border/%1").arg(n),
                                                   &reportedSize, QSize(13, 17));
        if (source.size() != QSize(n, n) || reportedSize != source.size())
            return QStringLiteral("Provider changed physical raster dimensions for %1")
                .arg(fixture.name);

        const QPointF origin = mapOrigin(item, view) * dpr;
        const QPoint topLeft(qRound(origin.x()), qRound(origin.y()));
        if (!capture.isNull()
            && !capture.rect().contains(QRect(topLeft - QPoint(1, 1), QSize(n + 2, n + 2))))
            return QStringLiteral("Raster fixture %1 is outside the capture").arg(fixture.name);
        int differences = 0;
        for (int y = -1; y <= n; ++y) {
            for (int x = -1; x <= n; ++x) {
                const bool inside = x >= 0 && y >= 0 && x < n && y < n;
                const QRgb expected = !inside ? background
                    : (x == 0 || y == 0 || x == n - 1 || y == n - 1) ? border : white;
                if (inside && source.pixel(x, y) != expected)
                    return QStringLiteral("Provider border is not exactly one texel: %1 at (%2,%3)")
                        .arg(fixture.name).arg(x).arg(y);
                if (!capture.isNull()
                    && !matches(capture.pixel(topLeft.x() + x, topLeft.y() + y), expected))
                    ++differences;
            }
        }
        std::cout << "RASTER " << stage << ' ' << fixture.name << " physical=("
                  << origin.x() << ',' << origin.y() << ") size=" << n
                  << " capture-different-pixels=" << differences << '\n';
        if (expectAligned && differences != 0)
            return QStringLiteral("Raster %1: %2 filtered/border/spill pixels in %3 capture")
                .arg(fixture.name).arg(differences).arg(QString::fromLatin1(stage));
        blurredIcons += differences != 0;
    }
    if (!expectAligned && !capture.isNull() && blurredIcons == 0) {
        if (view->rendererInterface()->graphicsApi() != QSGRendererInterface::Software)
            return QStringLiteral("Original raster fixtures did not reproduce fractional sampling");
        std::cout << "CHECK software renderer already rounds the original raster draws; "
                     "fractional logical origins are still recorded above.\n";
    }
    return {};
}

#ifdef QT_PIXELGRID_PATCHED_QT
struct RectangleFrame
{
    QRect pixels;
    int left = 0;
    int top = 0;
    int right = 0;
    int bottom = 0;
};

static RectangleFrame rectangleFrame(QQuickView *view, const QImage &capture,
                                     QQuickItem *item, QRgb border)
{
    const qreal dpr = view->effectiveDevicePixelRatio();
    const QPointF origin = mapOrigin(item, view) * dpr;
    const QRectF raw(origin, item->size() * dpr);
    const QRect probe = raw.adjusted(-3, -3, 3, 3).toAlignedRect().intersected(capture.rect());
    const auto matches = [&](int x, int y) {
        const QRgb pixel = capture.pixel(x, y);
        return std::abs(qRed(pixel) - qRed(border)) <= 1
            && std::abs(qGreen(pixel) - qGreen(border)) <= 1
            && std::abs(qBlue(pixel) - qBlue(border)) <= 1;
    };
    const int cx = qRound(raw.center().x());
    const int cy = qRound(raw.center().y());
    int l = probe.left(), r = probe.right(), t = probe.top(), b = probe.bottom();
    while (l <= r && !matches(l, cy)) ++l;
    while (r >= l && !matches(r, cy)) --r;
    while (t <= b && !matches(cx, t)) ++t;
    while (b >= t && !matches(cx, b)) --b;
    RectangleFrame result;
    if (l > r || t > b)
        return result;
    result.pixels = QRect(QPoint(l, t), QPoint(r, b));
    for (int x = l; x <= r && matches(x, cy); ++x) ++result.left;
    for (int x = r; x >= l && matches(x, cy); --x) ++result.right;
    for (int y = t; y <= b && matches(cx, y); ++y) ++result.top;
    for (int y = b; y >= t && matches(cx, y); --y) ++result.bottom;
    return result;
}

static QString checkRectangleFrames(QQuickView *view, const QImage &capture, const char *stage)
{
    const qreal dpr = view->effectiveDevicePixelRatio();
    const int expectedPen = qRound(dpr); // Qt's existing border.pixelAligned rule for border.width: 1.
    const bool aligned = QtQuickPixelGrid::isEnabled(view);
    for (const char *name : {"moveLeftBackgroundLeaf", "moveRightBackgroundLeaf", "clipViewport"}) {
        QQuickItem *item = namedItem(view, name);
        const QRgb color = QByteArray(name) == "clipViewport" ? qRgb(105, 114, 124) : qRgb(140, 154, 167);
        const RectangleFrame frame = rectangleFrame(view, capture, item, color);
        std::cout << "FRAME " << stage << ' ' << name << " top=" << frame.pixels.top()
                  << " bottom-exclusive=" << frame.pixels.bottom() + 1
                  << " thickness-LTRB=" << frame.left << ',' << frame.top << ','
                  << frame.right << ',' << frame.bottom << '\n';
        if (frame.pixels.isEmpty())
            return QStringLiteral("Rectangle border was not rendered: %1").arg(name);
        if (aligned) {
            if (frame.left != expectedPen || frame.top != expectedPen
                || frame.right != expectedPen || frame.bottom != expectedPen)
                return QStringLiteral("Unequal %1 border at %2: L/T/R/B=%3/%4/%5/%6, expected %7 physical pixels")
                    .arg(name).arg(stage).arg(frame.left).arg(frame.top).arg(frame.right).arg(frame.bottom).arg(expectedPen);
            const QRectF bounds = QtQuickPixelGrid::presentationBounds(item);
            const QRect expected(qRound(bounds.x() * dpr), qRound(bounds.y() * dpr),
                                 qRound(bounds.width() * dpr), qRound(bounds.height() * dpr));
            if (frame.pixels != expected)
                return QStringLiteral("Rendered %1 frame disagrees with presentation bounds at %2: pixels=(%3,%4 %5x%6) bounds=(%7,%8 %9x%10)")
                    .arg(name).arg(stage).arg(frame.pixels.x()).arg(frame.pixels.y()).arg(frame.pixels.width()).arg(frame.pixels.height())
                    .arg(expected.x()).arg(expected.y()).arg(expected.width()).arg(expected.height());
        }
    }
    return {};
}

static QString checkQtButtonFootprint(QQuickView *original, QQuickView *aligned,
                                     const QImage &originalCapture, const QImage &alignedCapture)
{
    // Compatibility is measured against the actual unmodified rendering path,
    // including the backend's pixel-center convention, not another round().
    for (const char *name : {"moveLeftBackgroundLeaf", "moveRightBackgroundLeaf"}) {
        const QRgb border = qRgb(140, 154, 167);
        const auto before = rectangleFrame(original, originalCapture, namedItem(original, name), border);
        const auto after = rectangleFrame(aligned, alignedCapture, namedItem(aligned, name), border);
        if (before.pixels != after.pixels)
            return QStringLiteral("Qt rectangle footprint changed on %1: original=(%2,%3 %4x%5), aligned=(%6,%7 %8x%9)")
                .arg(name).arg(before.pixels.x()).arg(before.pixels.y()).arg(before.pixels.width()).arg(before.pixels.height())
                .arg(after.pixels.x()).arg(after.pixels.y()).arg(after.pixels.width()).arg(after.pixels.height());
    }
    std::cout << "CHECK both Move button rectangle footprints match original Qt\n";
    return {};
}

static QString checkShaderDiagnostics(QQuickView *view, const QImage &capture,
                                      bool viewerSnap, const char *stage)
{
    const bool software = view->rendererInterface()->graphicsApi() == QSGRendererInterface::Software;
    const QStringList excluded = QtQuickPixelGrid::unsupportedItems(view);
    const qreal dpr = view->effectiveDevicePixelRatio();
    for (const char *name : { "galleryShaderLeaf", "viewerShaderLeaf" }) {
        QQuickItem *item = namedItem(view, name);
        if (software) {
            if (item)
                return QStringLiteral("GPU shader was instantiated in the software renderer: %1").arg(name);
            continue;
        }
        if (!item || !item->isVisible())
            return QStringLiteral("Missing visible shader diagnostic: %1").arg(name);
        const bool viewer = QByteArray(name) == "viewerShaderLeaf";
        const bool windowAligned = !viewer && QtQuickPixelGrid::isEnabled(view);
        if (!viewer && item->property("status").toInt() == 2)
            return QStringLiteral("Gallery shader failed: %1").arg(item->property("log").toString());
        if (viewer && item->property("pixelAligned").toBool() != viewerSnap)
            return QStringLiteral("F4 viewer shader snap binding did not update");
        const QPointF logicalOrigin = mapOrigin(item, view);
        const QPointF origin = logicalOrigin * dpr;
        if (windowAligned && (std::abs(origin.x() - std::round(origin.x())) > 0.001
            || std::abs(origin.y() - std::round(origin.y())) > 0.001))
            return QStringLiteral("ShaderEffect physical origin is fractional at %1: (%2,%3) px")
                .arg(QString::fromLatin1(stage)).arg(origin.x(), 0, 'f', 6).arg(origin.y(), 0, 'f', 6);
        if ((viewer && !excluded.contains(QString::fromLatin1(name)))
            || (windowAligned && excluded.contains(QString::fromLatin1(name))))
            return QStringLiteral("Incorrect shader coverage classification: %1").arg(name);
        if (windowAligned) {
            const QRectF bounds = QtQuickPixelGrid::presentationBounds(item);
            for (qreal edge : {bounds.left(), bounds.top(), bounds.right(), bounds.bottom()}) {
                if (std::abs(edge * dpr - std::round(edge * dpr)) > 0.001)
                    return QStringLiteral("ShaderEffect presentation edge is fractional: %1 px").arg(edge * dpr);
            }
        }
        const QPointF ux = item->mapToItem(view->contentItem(), QPointF(1, 0)) - logicalOrigin;
        const QPointF uy = item->mapToItem(view->contentItem(), QPointF(0, 1)) - logicalOrigin;
        if (std::abs(ux.x() - 1) > 0.001 || std::abs(ux.y()) > 0.001
            || std::abs(uy.y() - 1) > 0.001 || std::abs(uy.x()) > 0.001
            || std::abs(item->width() * dpr - 192) > 0.001
            || std::abs(item->height() * dpr - 96) > 0.001)
            return QStringLiteral("Shader fixture has unexpected physical size or transform: %1").arg(name);
        const QPoint topLeft(qRound(origin.x()), qRound(origin.y()));
        int differences = 0;
        int fieldPixels = 0;
        int inkPixels = 0;
        if (!capture.isNull()) {
            if (!capture.rect().contains(QRect(topLeft, QSize(192, 96))))
                return QStringLiteral("Shader fixture outside capture: %1").arg(name);
            // An independent integer-pixel reference, not a second shader.
            for (int y = 0; y < 96; ++y) {
                for (int x = 0; x < 192; ++x) {
                    const bool frame = x >= 4 && x < 188 && y >= 4 && y < 92
                        && (x == 4 || x == 187 || y == 4 || y == 91);
                    const bool vertical = x >= 12 && x <= 84 && (x - 12) % 4 == 0
                        && y >= 16 && y < 80;
                    const bool horizontal = x >= 104 && x < 180
                        && y >= 16 && y <= 76 && (y - 16) % 4 == 0;
                    const QRgb expected = frame ? qRgb(0, 191, 255)
                        : (vertical || horizontal) ? qRgb(255, 255, 255) : qRgb(32, 40, 48);
                    const QRgb actual = capture.pixel(topLeft.x() + x, topLeft.y() + y);
                    fieldPixels += std::abs(qRed(actual) - 32) <= 1
                        && std::abs(qGreen(actual) - 40) <= 1 && std::abs(qBlue(actual) - 48) <= 1;
                    inkPixels += qRed(actual) > 60 && qGreen(actual) > 60 && qBlue(actual) > 60;
                    differences += std::abs(qRed(actual) - qRed(expected)) > 1
                        || std::abs(qGreen(actual) - qGreen(expected)) > 1
                        || std::abs(qBlue(actual) - qBlue(expected)) > 1;
                }
            }
            if (fieldPixels < 9000 || inkPixels < 1000)
                return QStringLiteral("Shader pattern was not rendered: %1, field=%2 ink=%3")
                    .arg(name).arg(fieldPixels).arg(inkPixels);
            if ((windowAligned || (viewer && viewerSnap)) && differences != 0)
                return QStringLiteral("Aligned shader %1: %2 incorrect pixels at %3")
                    .arg(name).arg(differences).arg(QString::fromLatin1(stage));
            if (!windowAligned && !(viewer && viewerSnap) && differences == 0)
                return QStringLiteral("Unsnapped shader did not demonstrate a pixel-phase error: %1").arg(name);
        }
        std::cout << "SHADER " << stage << ' ' << name << " mapped-physical=("
                  << origin.x() << ',' << origin.y() << ") window-policy-covered=" << !viewer
                  << " own-vertex-snap=" << (viewer && viewerSnap)
                  << " reference-different-pixels=" << differences
                  << (capture.isNull() ? " (geometry-only)" : "") << '\n';
    }
    if (software)
        std::cout << "SKIP GPU shader rendering: software backend, no GPU shader coverage claimed.\n";
    return {};
}

static QString checkHalfPixelEdges(QQuickView *view)
{
    // Explicit expected pixels, including negative ties. Test accumulated
    // ancestry and shared edges without calling the engine's rounding helper.
    struct Case { qreal physical; int hardware; int software; };
    const Case cases[] = {
        {-2.51, -3, -3}, {-2.5, -3, -3}, {-2.49, -2, -2},
        {-0.5, -1, -1}, {0, 0, 0}, {0.5, 0, 1},
        {3.499, 3, 3}, {3.5, 3, 4}, {3.501, 4, 4},
        {60.5, 60, 61}, {115.5, 115, 116}
    };
    const qreal dpr = view->effectiveDevicePixelRatio();
    const bool software = view->rendererInterface()->graphicsApi() == QSGRendererInterface::Software;
    QQuickItem parent(view->contentItem());
    QQuickItem first(&parent);
    QQuickItem second(&parent);
    parent.setPosition(QPointF(7.25 / dpr, 11.25 / dpr));
    first.setSize(QSizeF(3 / dpr, 3 / dpr));
    second.setSize(first.size());
    for (const Case &test : cases) {
        const QPointF logical(test.physical / dpr - parent.x(),
                              test.physical / dpr - parent.y());
        first.setPosition(logical);
        second.setPosition(logical + QPointF(first.width(), 0));
        const QPointF origin = mapOrigin(&first, view) * dpr;
        const int expected = software ? test.software : test.hardware;
        if (std::abs(origin.x() - expected) > 0.001 || std::abs(origin.y() - expected) > 0.001
            || first.position() != logical)
            return QStringLiteral("Half-pixel edge %1 mapped to (%2,%3), expected %4")
                .arg(test.physical).arg(origin.x()).arg(origin.y()).arg(expected);
        const QRectF a = QtQuickPixelGrid::presentationBounds(&first);
        const QRectF b = QtQuickPixelGrid::presentationBounds(&second);
        if (std::abs((a.right() - b.left()) * dpr) > 0.001)
            return QStringLiteral("Shared edge split at %1 physical pixels").arg(test.physical);
        const QPointF ux = first.mapToItem(view->contentItem(), QPointF(1, 0)) - origin / dpr;
        const QPointF uy = first.mapToItem(view->contentItem(), QPointF(0, 1)) - origin / dpr;
        if (std::abs(ux.x() - 1) > 0.001 || std::abs(ux.y()) > 0.001
            || std::abs(uy.y() - 1) > 0.001 || std::abs(uy.x()) > 0.001)
            return QStringLiteral("Half-pixel correction introduced scale or rotation");
    }
    std::cout << "CHECK half-pixel ties, negative coordinates, nested shared edges and unit vectors passed\n";
    return {};
}

static QString checkCacheLifecycle(QQuickView *view, bool geometryOnly, bool shader = false)
{
    QQuickItem first(view->contentItem());
    QQuickItem second(view->contentItem());
    QQuickItem nested(&first);
    first.setZ(1000);
    second.setZ(1000);
    first.setY(360.37);
    second.setY(390.19);
    nested.setPosition(QPointF(1.19, 1.41));

    QQmlComponent component(view->engine());
    component.setData(shader ? R"(
        import QtQuick
        ShaderEffect {
            objectName: "shaderLifecycleLeaf"
            x: 0.33; y: 0.27
            width: 12.41; height: 12.19
            fragmentShader: "qrc:/ZoinGallery/resources/pixelgrid_solid.frag.qsb"
            property int clicks: 0
            MouseArea {
                objectName: "shaderLifecycleHitTarget"
                anchors.fill: parent
                onClicked: parent.clicks++
            }
        }
    )" : R"(
        import QtQuick
        Rectangle {
            objectName: "cacheLifecycleLeaf"
            x: 0.33; y: 0.27
            width: 8; height: 12
            color: "#00ff00"
        }
    )", QUrl());
    std::unique_ptr<QObject> object;
    QQmlComponent meshComponent(view->engine());
    std::unique_ptr<QObject> customMesh;
    if (shader) {
        meshComponent.setData("import QtQuick; GridMesh { resolution: Qt.size(2, 2) }", QUrl());
        customMesh.reset(meshComponent.create());
        if (!customMesh)
            return QStringLiteral("Could not create custom mesh fixture: %1").arg(meshComponent.errorString());
    }
    QQuickItem *leaf = nullptr;
    const qreal dpr = view->effectiveDevicePixelRatio();
    const auto snap = [dpr](qreal value) { return std::round(value * dpr) / dpr; };

    for (int step = 0; step < 16; ++step) {
        // Destroy/recreate, reparent, move only ancestors, resize and toggle.
        // Check mapping before a frame as well as pixels after synchronization.
        if (step % 4 == 0) {
            object.reset(component.create());
            leaf = qobject_cast<QQuickItem *>(object.get());
            if (!leaf)
                return QStringLiteral("Could not create cache lifecycle fixture: %1")
                    .arg(component.errorString());
            leaf->setParentItem(&nested);
        }
        first.setX(10.17 + step * 0.21);
        second.setX(50.39 - step * 0.17);
        if (step % 4 == 2)
            nested.setParentItem(nested.parentItem() == &first ? &second : &first);
        QQuickItem *parent = nested.parentItem();
        if (step % 4 == 3)
            leaf->setWidth(shader ? 14.63 : 12);
        const bool excludedShader = shader && (step == 5 || step == 9);
        if (shader) {
            if (step == 5 || step == 6)
                leaf->setProperty("vertexShader", step == 5
                    ? QUrl("qrc:/ZoinGallery/resources/pixelgrid_passthrough.vert.qsb") : QUrl());
            if (step == 9)
                leaf->setProperty("mesh", QVariant::fromValue(customMesh.get()));
            if (step == 10)
                leaf->setProperty("mesh", QSize(3, 2));
            first.setSize(QSizeF(10.19, 11.23));
            second.setSize(first.size());
            // Keep clipping enabled across successive ancestor moves so a
            // stale clip width cannot hide behind a clip-property update.
            first.setClip(step < 15);
            second.setClip(first.clip());
            const QStringList excluded = QtQuickPixelGrid::unsupportedItems(view);
            if (excluded.contains("shaderLifecycleLeaf") != excludedShader
                || excluded.contains("shaderLifecycleHitTarget") != excludedShader)
                return QStringLiteral("Shader vertex/mesh coverage transition failed at step %1").arg(step);
        }
        if (step % 8 == 6)
            QtQuickPixelGrid::setEnabled(view, false);
        else if (step % 8 == 7)
            QtQuickPixelGrid::setEnabled(view, true);
        const bool enabled = QtQuickPixelGrid::isEnabled(view);
        const QPointF raw = parent->position() + nested.position() + leaf->position();
        const QPointF expected = enabled && !excludedShader ? QPointF(snap(raw.x()), snap(raw.y()))
            : enabled ? mapOrigin(&nested, view) + leaf->position() : raw;
        const auto checkOrigin = [&]() {
            const QPointF delta = mapOrigin(leaf, view) - expected;
            return std::abs(delta.x() * dpr) < 0.001 && std::abs(delta.y() * dpr) < 0.001;
        };
        if (!checkOrigin())
            return QStringLiteral("Stale mapping before synchronization at lifecycle step %1").arg(step);

        if (!geometryOnly) {
            const QImage capture = view->grabWindow();
            if (capture.isNull())
                return QStringLiteral("Empty lifecycle capture at step %1").arg(step);
            if (enabled && !excludedShader) {
                const int left = qRound(raw.x() * dpr);
                const int top = qRound(raw.y() * dpr);
                const int right = qRound((raw.x() + leaf->width()) * dpr);
                const int bottom = qRound((raw.y() + leaf->height()) * dpr);
                QRect expectedPixels(left, top, right - left, bottom - top);
                if (parent->clip()) {
                    const int clipLeft = qRound(parent->x() * dpr);
                    const int clipTop = qRound(parent->y() * dpr);
                    const int clipRight = qRound((parent->x() + parent->width()) * dpr);
                    const int clipBottom = qRound((parent->y() + parent->height()) * dpr);
                    expectedPixels &= QRect(clipLeft, clipTop, clipRight - clipLeft, clipBottom - clipTop);
                }
                const QRect probe = expectedPixels.adjusted(-3, -3, 3, 3);
                if (!capture.rect().contains(probe))
                    return QStringLiteral("Lifecycle fixture is outside capture");
                for (int y = probe.top(); y <= probe.bottom(); ++y) {
                    for (int x = probe.left(); x <= probe.right(); ++x) {
                        const QColor pixel = capture.pixelColor(x, y);
                        const bool green = pixel.green() >= 254 && pixel.red() <= 1 && pixel.blue() <= 1;
                        if (green != expectedPixels.contains(x, y))
                            return QStringLiteral("Stale %1 bounds at lifecycle step %2, pixel (%3,%4)")
                                .arg(shader ? "shader" : "rectangle").arg(step).arg(x).arg(y);
                    }
                }
            }
        } else {
            QCoreApplication::processEvents(QEventLoop::AllEvents, 20);
        }
        if (!checkOrigin())
            return QStringLiteral("Stale mapping after synchronization at lifecycle step %1").arg(step);
        if (shader && enabled && !excludedShader) {
            const QRectF bounds = QtQuickPixelGrid::presentationBounds(leaf);
            const qreal visibleWidth = (std::round((raw.x() + leaf->width()) * dpr)
                                       - std::round(raw.x() * dpr)) / dpr;
            const qreal visibleHeight = (std::round((raw.y() + leaf->height()) * dpr)
                                        - std::round(raw.y() * dpr)) / dpr;
            if (std::abs(bounds.width() - visibleWidth) > 0.001
                || std::abs(bounds.height() - visibleHeight) > 0.001
                || !leaf->contains(QPointF(visibleWidth - 0.01 / dpr, visibleHeight / 2))
                || leaf->contains(QPointF(visibleWidth + 0.01 / dpr, visibleHeight / 2)))
                return QStringLiteral("Shader presentation containment disagrees with visible edge at step %1").arg(step);
        }
        if (shader && !geometryOnly && leaf->property("status").toInt() == 2)
            return QStringLiteral("Lifecycle shader failed: %1").arg(leaf->property("log").toString());
        if (shader)
            std::cout << "SHADER lifecycle step=" << step << " enabled=" << enabled
                      << " supported=" << !excludedShader << " clipped=" << parent->clip()
                      << " physical=(" << expected.x() * dpr << ',' << expected.y() * dpr << ")\n";
    }
    QtQuickPixelGrid::setEnabled(view, true);
    if (shader && !geometryOnly) {
        const QPoint inside = leaf->mapToItem(view->contentItem(), QPointF(2, 2)).toPoint();
        QTest::mouseClick(view, Qt::LeftButton, Qt::NoModifier, inside);
        if (leaf->property("clicks").toInt() != 1)
            return QStringLiteral("Click inside aligned ShaderEffect did not reach its MouseArea");
    }
    std::cout << "CHECK " << (shader ? "shader" : "rectangle")
              << " cache lifecycle: creation, destruction, reparenting, immediate mapping, "
                 "ancestor movement, resize, policy toggles"
              << (geometryOnly ? " passed\n" : " and exact rendered rectangle pixels passed\n");
    return {};
}
#endif

static int runChecks(QQuickView *original, QQuickView *aligned, DemoState *state,
                     bool geometryOnly)
{
    const QByteArray required[] = {
        "demoRoot", "rootColumnLayout", "headerRowLayout", "viewTitleLeaf",
        "dprLeaf", "statusLeaf", "toolbarRowLayout", "moveLeftControl",
        "moveLeftTextLeaf", "moveLeftBackgroundLeaf", "moveRightControl",
        "moveRightTextLeaf", "moveRightBackgroundLeaf", "resizeControl",
        "resizeTextLeaf", "resizeBackgroundLeaf", "contentControl",
        "contentTextLeaf", "contentBackgroundLeaf", "actionsRowLayout",
        "animationControl", "animationTextLeaf", "animationIndicatorLeaf",
        "captureControl", "captureTextLeaf", "captureBackgroundLeaf",
        "clipViewport", "nestedFractionalParent", "sampleRectangle",
        "smallCaptionLeaf", "editableTextLeaf", "rasterIconLeaf", "demoButton",
        "buttonTextLeaf", "buttonBackgroundLeaf", "demoCheckBox",
        "checkboxTextLeaf", "checkboxIndicatorLeaf", "oddCenterContainer",
        "oddCenteredLeaf", "clipInnerContainer", "clipEdgeLeaf",
        "rasterDiagnosticGroup", "rasterBorderTitleLeaf",
        "borderIconDepth0Leaf", "borderIconDepth1Leaf", "borderIconDepth2Leaf", "borderIconDepth4Leaf",
        "borderIconDepth0LabelLeaf", "borderIconDepth1LabelLeaf",
        "borderIconDepth2LabelLeaf", "borderIconDepth4LabelLeaf",
        "shaderDiagnosticGroup", "galleryShaderTitleLeaf", "viewerShaderTitleLeaf",
        "shaderCoverageNoteLeaf", "viewerShaderSnapControl",
        "viewerShaderSnapTextLeaf", "viewerShaderSnapIndicatorLeaf"
    };

    auto fail = [](const QString &message) {
        qCritical().noquote() << "FAIL:" << message;
        return 1;
    };

    const qreal dpr = aligned->effectiveDevicePixelRatio();
    const bool softwareRenderer = aligned->rendererInterface()->graphicsApi() == QSGRendererInterface::Software;
    std::cout << "Observed window DPR: " << dpr
              << " (no QT_SCALE_FACTOR is applied by native launch)\n";
    std::cout << "Observed scene graph API: " << int(aligned->rendererInterface()->graphicsApi())
              << (softwareRenderer ? " (software)\n" : " (hardware)\n");
    if (qEnvironmentVariable("QT_QUICK_BACKEND") == QStringLiteral("software") && !softwareRenderer)
        return fail(QStringLiteral("Requested software renderer is not active"));
    bool expectedDprOk = false;
    const qreal expectedDpr = qgetenv("QT_PIXELGRID_EXPECT_DPR").toDouble(&expectedDprOk);
    if (expectedDprOk && std::abs(dpr - expectedDpr) > 0.001)
        return fail(QStringLiteral("Expected test DPR %1, observed %2")
                    .arg(expectedDpr, 0, 'f', 3).arg(dpr, 0, 'f', 3));

    int baselineFractional = 0;
    qreal worstError = 0;
    QString worstDescription;
    for (const QByteArray &name : required) {
        QQuickItem *before = namedItem(original, name.constData());
        QQuickItem *after = namedItem(aligned, name.constData());
        if (!before || !after)
            return fail(QStringLiteral("Missing named visual leaf: %1").arg(QString::fromLatin1(name)));

        const QPointF originalOrigin = mapOrigin(before, original);
        const QPointF alignedOrigin = mapOrigin(after, aligned);
        if (std::abs(before->x() - after->x()) > 0.000001
            || std::abs(before->y() - after->y()) > 0.000001
            || std::abs(before->width() - after->width()) > 0.000001
            || std::abs(before->height() - after->height()) > 0.000001)
            return fail(QStringLiteral("Pixel policy changed logical geometry for %1")
                        .arg(QString::fromLatin1(name)));
        const qreal physicalX = originalOrigin.x() * dpr;
        const qreal physicalY = originalOrigin.y() * dpr;
        const qreal errX = std::abs(physicalX - std::round(physicalX));
        const qreal errY = std::abs(physicalY - std::round(physicalY));
        if (errX > 0.001 || errY > 0.001)
            ++baselineFractional;
        std::cout << "BASELINE " << name.constData() << " physical origin=("
                  << physicalX << ',' << physicalY << ") fractional-error=("
                  << errX << ',' << errY << ")\n";

#ifdef QT_PIXELGRID_PATCHED_QT
        const qreal alignedPhysicalX = alignedOrigin.x() * dpr;
        const qreal alignedPhysicalY = alignedOrigin.y() * dpr;
        const qreal alignedErrX = std::abs(alignedPhysicalX - std::round(alignedPhysicalX));
        const qreal alignedErrY = std::abs(alignedPhysicalY - std::round(alignedPhysicalY));
        const QString label = QString::fromLatin1(name);
        std::cout << label.toStdString() << " original physical origin=("
                  << physicalX << ',' << physicalY << ") aligned=("
                  << alignedPhysicalX << ',' << alignedPhysicalY << ")\n";
        if (alignedErrX > 0.001 || alignedErrY > 0.001) {
            const qreal current = std::max(alignedErrX, alignedErrY);
            if (current > worstError) {
                worstError = current;
                worstDescription = label;
            }
        }

        const QPointF unitX = after->mapToItem(aligned->contentItem(), QPointF(1, 0)) - alignedOrigin;
        const QPointF unitY = after->mapToItem(aligned->contentItem(), QPointF(0, 1)) - alignedOrigin;
        if (std::abs(unitX.x() - 1.0) > 0.001 || std::abs(unitX.y()) > 0.001
            || std::abs(unitY.x()) > 0.001 || std::abs(unitY.y() - 1.0) > 0.001)
            return fail(QStringLiteral("Non-identity visual leaf transform: %1").arg(label));

        const QRectF bounds = QtQuickPixelGrid::presentationBounds(after);
        const qreal edges[] = { bounds.left() * dpr, bounds.top() * dpr,
                                bounds.right() * dpr, bounds.bottom() * dpr };
        for (qreal edge : edges) {
            if (std::abs(edge - std::round(edge)) > 0.001)
                return fail(QStringLiteral("Unaligned presentation edge on %1: %2")
                            .arg(label).arg(edge, 0, 'f', 4));
        }
#else
        Q_UNUSED(alignedOrigin);
#endif
    }

    if (baselineFractional == 0)
        return fail(QStringLiteral("The unmodified view did not reproduce a fractional leaf origin"));
#ifdef QT_PIXELGRID_PATCHED_QT
    if (!worstDescription.isEmpty())
        return fail(QStringLiteral("Physical-grid error %1 px remains on %2")
                    .arg(worstError, 0, 'f', 4).arg(worstDescription));
#else
    Q_UNUSED(worstDescription);
#endif

    if (!geometryOnly) {
        const QImage originalCapture = original->grabWindow();
        const QImage alignedCapture = aligned->grabWindow();
        if (originalCapture.isNull() || alignedCapture.isNull())
            return fail(QStringLiteral("Window capture is empty"));
        if (originalCapture.size() != alignedCapture.size())
            return fail(QStringLiteral("Comparison windows captured at different native sizes"));
        const QString captureDir = QDir(QCoreApplication::applicationDirPath()).filePath("captures");
        QDir().mkpath(captureDir);
        QString renderer = softwareRenderer ? QStringLiteral("software") : qEnvironmentVariable("QSG_RHI_BACKEND");
        if (renderer.isEmpty())
            renderer = QStringLiteral("native");
        originalCapture.save(QDir(captureDir).filePath(
            QStringLiteral("comparison-original-%1.png").arg(renderer)));
#ifdef QT_PIXELGRID_PATCHED_QT
        alignedCapture.save(QDir(captureDir).filePath(
            QStringLiteral("patched-aligned-%1.png").arg(renderer)));
#else
        alignedCapture.save(QDir(captureDir).filePath(
            QStringLiteral("baseline-aligned-unpatched-%1.png").arg(renderer)));
#endif
        if (const QString error = checkRasterIcons(original, originalCapture, false, "original");
            !error.isEmpty())
            return fail(error);
#ifdef QT_PIXELGRID_PATCHED_QT
        if (const QString error = checkRasterIcons(aligned, alignedCapture, true, "initial");
            !error.isEmpty())
            return fail(error);
        if (const QString error = checkShaderDiagnostics(original, originalCapture, true, "original");
            !error.isEmpty())
            return fail(error);
        if (const QString error = checkShaderDiagnostics(aligned, alignedCapture, true, "initial");
            !error.isEmpty())
            return fail(error);
        if (const QString error = checkRectangleFrames(original, originalCapture, "original"); !error.isEmpty())
            return fail(error);
        if (const QString error = checkRectangleFrames(aligned, alignedCapture, "initial"); !error.isEmpty())
            return fail(error);
        if (const QString error = checkQtButtonFootprint(original, aligned, originalCapture, alignedCapture); !error.isEmpty())
            return fail(error);
#endif
        std::cout << "CHECK initial captures and one-pixel raster borders passed\n";
    } else if (const QString error = checkRasterIcons(aligned, {}, true, "geometry-only");
               !error.isEmpty()) {
        return fail(error);
    }

#ifdef QT_PIXELGRID_PATCHED_QT
    if (geometryOnly) {
        if (const QString error = checkShaderDiagnostics(aligned, {}, true, "geometry-only");
            !error.isEmpty())
            return fail(error);
    } else if (!softwareRenderer) {
        // Toggle the shader's own alignment independently of the window policy.
        state->setViewerShaderSnap(false);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
        for (QQuickView *view : { original, aligned }) {
            const QImage capture = view->grabWindow();
            if (capture.isNull())
                return fail(QStringLiteral("Shader toggle capture was empty"));
            if (const QString error = checkShaderDiagnostics(view, capture, false, "own-snap-off");
                !error.isEmpty())
                return fail(error);
            const QString name = view == original ? QStringLiteral("original") : QStringLiteral("aligned");
            capture.save(QDir(QCoreApplication::applicationDirPath()).filePath(
                QStringLiteral("captures/shader-own-snap-off-%1.png").arg(name)));
        }
        state->setViewerShaderSnap(true);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
    }
#endif

    // Exercise changed text, ancestor-only scene movement, resize and another
    // capture without changing the QML bindings that own those logical values.
    const qreal oldMotion = state->motion();
    const qreal oldCaptionImplicitWidth = namedItem(aligned, "smallCaptionLeaf")
        ->property("implicitWidth").toReal();
    state->move(0.25);
    state->resizeBy(1);
    state->changeContent();
    QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
    if (!geometryOnly) {
        const QImage changedCapture = aligned->grabWindow();
        if (changedCapture.isNull())
            return fail(QStringLiteral("Capture immediately after resize/content change was empty"));
#ifdef QT_PIXELGRID_PATCHED_QT
        if (const QString error = checkRasterIcons(aligned, changedCapture, true, "moved/resized");
            !error.isEmpty())
            return fail(error);
        if (const QString error = checkShaderDiagnostics(aligned, changedCapture, true, "moved/resized");
            !error.isEmpty())
            return fail(error);
#endif
    }
    if (namedItem(aligned, "sampleRectangle")->width() != 112)
        return fail(QStringLiteral("A bound rectangle width did not update after resizing"));
    if (namedItem(aligned, "smallCaptionLeaf")->property("text").toString()
        != state->content())
        return fail(QStringLiteral("Bound text did not update after changing content"));
    const QFont changedFont = namedItem(aligned, "smallCaptionLeaf")->property("font").value<QFont>();
    if (changedFont.pixelSize() != state->captionPixelSize()
        || std::abs(namedItem(aligned, "smallCaptionLeaf")->property("implicitWidth").toReal()
                    - oldCaptionImplicitWidth) < 1.0)
        return fail(QStringLiteral("Bound font change did not update text implicit sizing"));
    QQuickItem *rasterImage = namedItem(aligned, "rasterIconLeaf");
    if (rasterImage->property("source").toString() != state->imageSource()
        || rasterImage->property("status").toInt() != 1)
        return fail(QStringLiteral("Changed raster source did not load its bound image"));
    std::cout << "CHECK content and resize updates passed\n";
#ifdef QT_PIXELGRID_PATCHED_QT
    for (const QByteArray &name : required) {
        QQuickItem *item = namedItem(aligned, name.constData());
        const QPointF origin = mapOrigin(item, aligned);
        const qreal ex = std::abs(origin.x() * dpr - std::round(origin.x() * dpr));
        const qreal ey = std::abs(origin.y() * dpr - std::round(origin.y() * dpr));
        if (ex > 0.001 || ey > 0.001)
            return fail(QStringLiteral("Changed-content/resize alignment failed on %1: (%2,%3) px")
                        .arg(QString::fromLatin1(name))
                        .arg(origin.x() * dpr, 0, 'f', 4)
                        .arg(origin.y() * dpr, 0, 'f', 4));
    }

    QtQuickPixelGrid::setEnabled(aligned, false);
    QtQuickPixelGrid::setEnabled(aligned, true);
    QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
    const QPointF toggledOrigin = mapOrigin(namedItem(aligned, "smallCaptionLeaf"), aligned);
    if (std::abs(toggledOrigin.x() * dpr - std::round(toggledOrigin.x() * dpr)) > 0.001
        || std::abs(toggledOrigin.y() * dpr - std::round(toggledOrigin.y() * dpr)) > 0.001)
        return fail(QStringLiteral("Alignment drifted after toggling the policy"));
    std::cout << "CHECK policy toggle passed\n";

    state->setAnimating(true);
    state->move(8.25);
    QTest::qWait(520);
    QQuickItem *animatedParent = namedItem(aligned, "nestedFractionalParent");
    if (std::abs(animatedParent->x() - (19.35 + state->motion())) > 0.001)
        return fail(QStringLiteral("Fractional-position animation did not reach its endpoint"));
    if (!softwareRenderer && !namedItem(aligned, "viewerShaderLeaf")->property("pixelAligned").toBool())
        return fail(QStringLiteral("F4 viewer snapping did not resume when ancestor animation stopped"));
    for (const QByteArray &name : required) {
        const QPointF origin = mapOrigin(namedItem(aligned, name.constData()), aligned);
        if (std::abs(origin.x() * dpr - std::round(origin.x() * dpr)) > 0.001
            || std::abs(origin.y() * dpr - std::round(origin.y() * dpr)) > 0.001)
            return fail(QStringLiteral("Alignment did not recover at animated endpoint for %1")
                        .arg(QString::fromLatin1(name)));
    }
    state->setAnimating(false);
    if (!geometryOnly) {
        const QImage endpointCapture = aligned->grabWindow();
        if (endpointCapture.isNull())
            return fail(QStringLiteral("Capture at animation endpoint was empty"));
        if (const QString error = checkRasterIcons(aligned, endpointCapture, true, "animation-end");
            !error.isEmpty())
            return fail(error);
        if (const QString error = checkShaderDiagnostics(aligned, endpointCapture, true, "animation-end");
            !error.isEmpty())
            return fail(error);
    }
    state->setMotion(oldMotion);
    QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
    std::cout << "CHECK fractional animation endpoint passed\n";

    if (!geometryOnly) {
        QQuickItem *moveRight = namedItem(aligned, "moveRightControl");
        const QPoint clickPoint = moveRight->mapToItem(aligned->contentItem(),
                                                       QPointF(moveRight->width() / 2,
                                                               moveRight->height() / 2)).toPoint();
        const qreal motionBeforeClick = state->motion();
        QTest::mouseClick(aligned, Qt::LeftButton, Qt::NoModifier, clickPoint);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
        if (state->motion() <= motionBeforeClick)
            return fail(QStringLiteral("Click at the visible corrected button did not activate it"));
        std::cout << "CHECK button input passed\n";

        QQuickItem *editable = namedItem(aligned, "editableTextLeaf");
        const QPoint inputPoint = editable->mapToItem(aligned->contentItem(),
                                                      QPointF(editable->width() / 2,
                                                              editable->height() / 2)).toPoint();
        QTest::mouseClick(aligned, Qt::LeftButton, Qt::NoModifier, inputPoint);
        QTest::keyClick(aligned, Qt::Key_End);
        QTest::keyClick(aligned, Qt::Key_A);
        QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
        std::cout << "CHECK TextInput events passed; text="
                  << editable->property("text").toString().toStdString() << '\n';
        if (!editable->hasActiveFocus()
            || editable->property("text").toString() != QStringLiteral("Editable cursora"))
            return fail(QStringLiteral("TextInput did not accept input at its visible corrected bounds"));

        const QRectF cursorRect = editable->property("cursorRectangle").toRectF();
        const QPointF cursorTopLeft = editable->mapToItem(aligned->contentItem(), cursorRect.topLeft());
        const QPointF cursorBottomRight = editable->mapToItem(aligned->contentItem(), cursorRect.bottomRight());
        const qreal cursorEdges[] = { cursorTopLeft.x() * dpr, cursorTopLeft.y() * dpr,
                                      cursorBottomRight.x() * dpr, cursorBottomRight.y() * dpr };
        for (qreal edge : cursorEdges) {
            if (std::abs(edge - std::round(edge)) > 0.001)
                return fail(QStringLiteral("TextInput cursor presentation edge is not pixel aligned: %1 px")
                            .arg(edge, 0, 'f', 4));
        }
    }
#endif
    state->setMotion(oldMotion);

#ifdef QT_PIXELGRID_PATCHED_QT
    if (const QString error = checkHalfPixelEdges(aligned); !error.isEmpty())
        return fail(error);
    if (const QString error = checkCacheLifecycle(aligned, geometryOnly); !error.isEmpty())
        return fail(error);
    if (!softwareRenderer) {
        if (const QString error = checkCacheLifecycle(aligned, geometryOnly, true); !error.isEmpty())
            return fail(error);
    }
    if (!geometryOnly) {
        const QSize originalSize = aligned->size();
        for (const QSize delta : {QSize(1, 0), QSize(1, 1), QSize(-1, -1), QSize(3, 5), QSize(0, 0)}) {
            aligned->resize(originalSize + delta);
            QCoreApplication::processEvents(QEventLoop::AllEvents, 40);
            const QImage capture = aligned->grabWindow();
            if (capture.isNull())
                return fail(QStringLiteral("Empty rectangle frame resize capture"));
            if (const QString error = checkRectangleFrames(aligned, capture, "window-resize"); !error.isEmpty()) {
                capture.save(QDir(QCoreApplication::applicationDirPath()).filePath("captures/rectangle-resize-failure.png"));
                return fail(error);
            }
        }
    }
    std::cout << "PASS: baseline fractional leaves = " << baselineFractional
              << "; patched scene geometry, logical geometry, unit vectors, bounds, toggles, button input, text editing and cursor geometry passed.\n";
#else
    std::cout << "PASS: baseline characterization reproduced " << baselineFractional
              << " fractional physical-pixel origins.\n";
#endif
    std::cout << (geometryOnly ? "Capture skipped in deterministic geometry-only mode.\n"
                               : "Compare the saved native-resolution captures for text/raster sharpness.\n")
              << (QGuiApplication::platformName() == QStringLiteral("offscreen")
                      ? "Software/offscreen run.\n"
                      : "Native window-system run.\n");
    return 0;
}

static int visualItemCount(const QQuickItem *item)
{
    int count = 1;
    for (const QQuickItem *child : item->childItems())
        count += visualItemCount(child);
    return count;
}

static double percentile(std::vector<double> values, double p)
{
    if (values.empty())
        return 0;
    std::sort(values.begin(), values.end());
    const size_t index = static_cast<size_t>(std::round(p * (values.size() - 1)));
    return values[index];
}

static int runPerformanceBenchmark(QGuiApplication &app, int warmupFrames, int sampleFrames,
                                   bool measureUpdates)
{
    if (warmupFrames < 1 || sampleFrames < 10) {
        qCritical() << "Benchmark requires at least one warmup frame and ten sample frames.";
        return 2;
    }

    QQuickView view;
    view.setScreen(QGuiApplication::primaryScreen());
    view.setResizeMode(QQuickView::SizeRootObjectToView);
    view.setSource(QUrl(QStringLiteral("qrc:/qt/qml/QtPixelGrid/Proof/PerformanceScene.qml")));
    if (view.status() == QQuickView::Error) {
        for (const QQmlError &error : view.errors())
            qCritical().noquote() << error.toString();
        return 2;
    }
    if (measureUpdates)
        view.rootObject()->setProperty("autoAnimate", false);

#ifdef QT_PIXELGRID_PATCHED_QT
    QtQuickPixelGrid::setEnabled(&view, true);
    const int unsupportedCount = QtQuickPixelGrid::unsupportedItems(&view).size();
#else
    const int unsupportedCount = -1;
#endif

    const QRect available = QGuiApplication::primaryScreen()->availableGeometry();
    constexpr int width = 720;
    constexpr int height = 420;
    view.setGeometry(available.x() + (available.width() - width) / 2,
                     available.y() + (available.height() - height) / 2,
                     width, height);
    view.setTitle(QStringLiteral("Qt pixel-grid performance scene"));
    view.show();
    if (!QTest::qWaitForWindowExposed(&view, 5000)) {
        qCritical() << "Benchmark window was not exposed on the primary display.";
        return 3;
    }

    QQuickItem *root = view.rootObject();
    const int visualItems = root ? visualItemCount(root) : 0;
    const int flatLeaves = root ? root->property("flatLeafCount").toInt() : 0;
    const int nestedLeaves = root
        ? root->property("nestedBranchCount").toInt()
            * root->property("nestedLeavesPerBranch").toInt()
        : 0;
    if (visualItems < 400 || flatLeaves != 200 || nestedLeaves != 200) {
        qCritical() << "Unexpected benchmark scene size:" << visualItems
                    << "visual items," << flatLeaves << "flat leaves and"
                    << nestedLeaves << "nested leaves.";
        return 4;
    }

    struct Samples {
        std::mutex mutex;
        std::vector<double> frameIntervalMs;
        std::vector<double> syncToRenderMs;
        std::vector<double> updateMs;
        std::atomic<qint64> syncStartedNs{0};
        std::atomic<qint64> lastSyncNs{0};
        std::atomic<int> warmupCount{0};
        std::atomic<int> sampleCount{0};
        std::atomic<bool> collecting{false};
        std::atomic<bool> quitQueued{false};
        qint64 measurementStartNs = 0;
        double measurementMs = 0;
    } samples;

    QElapsedTimer elapsed;
    elapsed.start();
    int phaseStep = 0;
    if (measureUpdates) {
        QObject::connect(&view, &QQuickWindow::frameSwapped, &view, [&] {
            if (samples.quitQueued.load(std::memory_order_acquire))
                return;
            // One deterministic QML binding update per frame. The timer ends
            // before requesting a render, so it isolates GUI-side mutation and
            // descendant invalidation from vsync and driver waiting.
            const qreal phase = (++phaseStep % 192) * (6.0 / 192);
            QElapsedTimer updateTimer;
            updateTimer.start();
            root->setProperty("phase", phase);
            const double updateMs = updateTimer.nsecsElapsed() / 1.0e6;
            if (samples.collecting.load(std::memory_order_acquire)) {
                std::lock_guard<std::mutex> lock(samples.mutex);
                samples.updateMs.push_back(updateMs);
            }
            view.update();
        }, Qt::QueuedConnection);
        view.update();
    }
    QObject::connect(&view, &QQuickWindow::beforeSynchronizing, &view, [&] {
        if (samples.quitQueued.load(std::memory_order_acquire))
            return;
        const qint64 now = elapsed.nsecsElapsed();
        const qint64 previous = samples.lastSyncNs.exchange(now, std::memory_order_relaxed);
        samples.syncStartedNs.store(now, std::memory_order_relaxed);
        if (samples.collecting.load(std::memory_order_acquire) && previous > 0) {
            std::lock_guard<std::mutex> lock(samples.mutex);
            samples.frameIntervalMs.push_back((now - previous) / 1.0e6);
        }
    }, Qt::DirectConnection);

    QObject::connect(&view, &QQuickWindow::afterRendering, &view, [&] {
        if (samples.quitQueued.load(std::memory_order_acquire))
            return;
        if (!samples.collecting.load(std::memory_order_acquire)) {
            if (samples.warmupCount.fetch_add(1, std::memory_order_relaxed) + 1 >= warmupFrames) {
                samples.measurementStartNs = elapsed.nsecsElapsed();
                samples.collecting.store(true, std::memory_order_release);
            }
            return;
        }

        const qint64 now = elapsed.nsecsElapsed();
        const qint64 start = samples.syncStartedNs.exchange(0, std::memory_order_relaxed);
        if (start > 0) {
            std::lock_guard<std::mutex> lock(samples.mutex);
            samples.syncToRenderMs.push_back((now - start) / 1.0e6);
        }
        const int completed = samples.sampleCount.fetch_add(1, std::memory_order_relaxed) + 1;
        if (completed >= sampleFrames
            && !samples.quitQueued.exchange(true, std::memory_order_acq_rel)) {
            std::lock_guard<std::mutex> lock(samples.mutex);
            samples.measurementMs = (now - samples.measurementStartNs) / 1.0e6;
            QMetaObject::invokeMethod(&app, [&app] { app.quit(); }, Qt::QueuedConnection);
        }
    }, Qt::DirectConnection);

    int timedOut = false;
    QTimer watchdog;
    watchdog.setSingleShot(true);
    QObject::connect(&watchdog, &QTimer::timeout, &app, [&] {
        timedOut = true;
        app.quit();
    });
    watchdog.start(60000);
    const int appResult = app.exec();
    samples.collecting.store(false, std::memory_order_release);
    watchdog.stop();
    view.hide();
    QCoreApplication::processEvents(QEventLoop::AllEvents, 30);

    std::vector<double> frameIntervals;
    std::vector<double> syncToRender;
    std::vector<double> updates;
    {
        std::lock_guard<std::mutex> lock(samples.mutex);
        frameIntervals = samples.frameIntervalMs;
        syncToRender = samples.syncToRenderMs;
        updates = samples.updateMs;
    }
    const int measuredFrames = samples.sampleCount.load(std::memory_order_relaxed);
    if (timedOut || measuredFrames < sampleFrames || syncToRender.empty()) {
        qCritical() << "Benchmark did not collect its requested samples; collected"
                    << measuredFrames << "of" << sampleFrames << "frames.";
        return 5;
    }
    if (measureUpdates && updates.size() < size_t(sampleFrames - 1)) {
        qCritical() << "Controlled benchmark did not update the scene for each measured frame:" << updates.size();
        return 6;
    }

#ifdef QT_PIXELGRID_PATCHED_QT
    const char *mode = "patched";
#else
    const char *mode = "stock";
#endif
    std::cout << std::fixed << std::setprecision(3)
              << "BENCHMARK {\"mode\":\"" << mode
              << "\",\"dpr\":" << view.effectiveDevicePixelRatio()
              << ",\"visual_items\":" << visualItems
              << ",\"flat_leaves\":" << flatLeaves
              << ",\"nested_leaves\":" << nestedLeaves
              << ",\"unsupported_items\":" << unsupportedCount
              << ",\"warmup_frames\":" << warmupFrames
              << ",\"sample_frames\":" << measuredFrames
              << ",\"measurement_ms\":" << samples.measurementMs
              << ",\"controlled_updates\":" << (measureUpdates ? "true" : "false")
              << ",\"update_samples\":" << updates.size()
              << ",\"gui_update_ms_p50\":" << percentile(updates, 0.50)
              << ",\"gui_update_ms_p95\":" << percentile(updates, 0.95)
              << ",\"gui_update_ms_p99\":" << percentile(updates, 0.99)
              << ",\"graphics_api\":" << int(view.rendererInterface()->graphicsApi())
              << ",\"frame_interval_ms_p50\":" << percentile(frameIntervals, 0.50)
              << ",\"frame_interval_ms_p95\":" << percentile(frameIntervals, 0.95)
              << ",\"frame_interval_ms_p99\":" << percentile(frameIntervals, 0.99)
              << ",\"frame_interval_ms_max\":" << percentile(frameIntervals, 1.0)
              << ",\"sync_to_render_ms_p50\":" << percentile(syncToRender, 0.50)
              << ",\"sync_to_render_ms_p95\":" << percentile(syncToRender, 0.95)
              << ",\"sync_to_render_ms_p99\":" << percentile(syncToRender, 0.99)
              << ",\"sync_to_render_ms_max\":" << percentile(syncToRender, 1.0)
              << "}\n";
    std::cout.flush();
    return appResult;
}

int main(int argc, char **argv)
{
    bool check = false;
    bool geometryOnly = false;
    bool benchmark = false;
    bool measureUpdates = false;
    int warmupFrames = 120;
    int sampleFrames = 360;
    for (int index = 1; index < argc; ++index)
        if (QByteArray(argv[index]) == "--check")
            check = true;
        else if (QByteArray(argv[index]) == "--geometry-only")
            geometryOnly = true;
        else if (QByteArray(argv[index]) == "--benchmark")
            benchmark = true;
        else if (QByteArray(argv[index]) == "--benchmark-measure-updates")
            measureUpdates = true;
        else if (QByteArray(argv[index]) == "--benchmark-warmup" && index + 1 < argc)
            warmupFrames = QByteArray(argv[++index]).toInt();
        else if (QByteArray(argv[index]) == "--benchmark-frames" && index + 1 < argc)
            sampleFrames = QByteArray(argv[++index]).toInt();
    QGuiApplication app(argc, argv);
    QQuickStyle::setStyle(QStringLiteral("Basic"));

    if (benchmark)
        return runPerformanceBenchmark(app, warmupFrames, sampleFrames, measureUpdates);

    DemoState state;
    const QRect available = QGuiApplication::primaryScreen()->availableGeometry();
    const int gap = 16;
    const int width = 700;
    const int height = 700;
    int x1 = available.x() + 12;
    int y1 = available.y() + 24;
    int x2 = x1 + width + gap;
    int y2 = y1;
    if (x2 + width > available.right() - 8) {
        x2 = x1;
        y2 = y1 + height + 32;
    }

    const bool headlessGeometry = check && geometryOnly
        && QGuiApplication::platformName() == QStringLiteral("offscreen");
    // Destroy the QML engines before their shared context object.
    const std::unique_ptr<QQuickView> originalOwner(
        createView(&state, false, QRect(x1, y1, width, height), !headlessGeometry));
    const std::unique_ptr<QQuickView> alignedOwner(
        createView(&state, true, QRect(x2, y2, width, height), !headlessGeometry));
    QQuickView *original = originalOwner.get();
    QQuickView *aligned = alignedOwner.get();
    if (check) {
        if (original->status() == QQuickView::Error || aligned->status() == QQuickView::Error)
            return 2;
        if (QGuiApplication::platformName() != QStringLiteral("offscreen")) {
            if (!QTest::qWaitForWindowExposed(original, 3000)
                || !QTest::qWaitForWindowExposed(aligned, 3000))
                return 3;
        }
        QCoreApplication::processEvents(QEventLoop::AllEvents, 200);
        return runChecks(original, aligned, &state, geometryOnly);
    }

    return app.exec();
}

#include "main.moc"
