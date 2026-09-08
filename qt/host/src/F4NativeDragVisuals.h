#pragma once
// Action artwork from ZoinGallery fc1d150, FileListModel.cpp.
#include <QGuiApplication>
#include <QPainter>
#include <QFontMetricsF>
#include <QPixmap>
#ifdef Q_OS_WIN
#include <qt_windows.h>
#endif
namespace F4NativeDragVisuals {
// Paint the caption directly on the physical pixel grid, without resampling
// either the existing preview or native text at fractional display scales.
inline QPixmap withFileName(const QPixmap &preview, const QString &name, int total, bool dark)
{
    if (preview.isNull() || name.isEmpty()) return preview;
    const qreal dpr = preview.devicePixelRatio();
    const int padding = qRound(8 * dpr);
    const int gap = qRound(4 * dpr);
    QFont font = QGuiApplication::font();
    font.setPixelSize(qRound(13 * dpr));
    const QFontMetrics metrics(font);
    const QString suffix = total > 1 ? QStringLiteral("  (+%1)").arg(total - 1) : QString();
    const QString label = metrics.elidedText(name, Qt::ElideMiddle,
        qRound(360 * dpr) - 2 * padding - metrics.horizontalAdvance(suffix)) + suffix;
    const int width = metrics.horizontalAdvance(label) + 2 * padding;
    const QRect caption(0, preview.height() + gap, width, metrics.height() + 2 * padding);
    QPixmap result(qMax(preview.width(), width), caption.bottom() + 1);
    result.fill(Qt::transparent);
    QPainter painter(&result);
    // Both source and destination extents are physical pixels; copy 1:1.
    QImage pixels = preview.toImage();
    pixels.setDevicePixelRatio(1);
    painter.drawImage(QPoint(0, 0), pixels);
    painter.setRenderHint(QPainter::Antialiasing);
    painter.setPen(QPen(dark ? QColor(255,255,255,70) : QColor(0,0,0,55),1));
    painter.setBrush(dark ? QColor(48,48,48,195) : QColor(238,238,238,195));
    const int radius = qRound(6 * dpr);
    painter.drawRoundedRect(QRectF(caption).adjusted(0.5,0.5,-0.5,-0.5),radius,radius);
    painter.setFont(font);
    painter.setPen(dark ? Qt::white : Qt::black);
    painter.drawText(QPoint(padding, caption.top() + padding + metrics.ascent()), label);
    painter.end();
    result.setDevicePixelRatio(dpr);
    return result;
}
#ifdef Q_OS_WIN
inline QPixmap systemCursorPixmap(LPCWSTR cursorId, qreal dpr) {
    const HCURSOR cursor = LoadCursorW(nullptr, cursorId);
    if (!cursor) {
        return {};
    }

    ICONINFO iconInfo{};
    BITMAP cursorBitmap{};
    if (!GetIconInfo(cursor, &iconInfo)) {
        return {};
    }
    const HBITMAP sizeBitmap = iconInfo.hbmColor
        ? iconInfo.hbmColor : iconInfo.hbmMask;
    const bool haveBitmapInfo = sizeBitmap &&
        GetObjectW(sizeBitmap, sizeof(BITMAP), &cursorBitmap) == sizeof(BITMAP);
    if (iconInfo.hbmColor) {
        DeleteObject(iconInfo.hbmColor);
    }
    if (iconInfo.hbmMask) {
        DeleteObject(iconInfo.hbmMask);
    }
    if (!haveBitmapInfo) {
        return {};
    }
    const int cursorWidth = cursorBitmap.bmWidth;
    const int cursorHeight = iconInfo.hbmColor
        ? cursorBitmap.bmHeight : cursorBitmap.bmHeight / 2;

    BITMAPINFO bitmapInfo{};
    bitmapInfo.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bitmapInfo.bmiHeader.biWidth = cursorWidth;
    bitmapInfo.bmiHeader.biHeight = -cursorHeight;
    bitmapInfo.bmiHeader.biPlanes = 1;
    bitmapInfo.bmiHeader.biBitCount = 32;
    bitmapInfo.bmiHeader.biCompression = BI_RGB;

    void *pixels = nullptr;
    const HDC screenDc = GetDC(nullptr);
    const HBITMAP bitmap = CreateDIBSection(
        screenDc, &bitmapInfo, DIB_RGB_COLORS, &pixels, nullptr, 0);
    const HDC memoryDc = CreateCompatibleDC(screenDc);
    ReleaseDC(nullptr, screenDc);
    if (!bitmap || !memoryDc || !pixels) {
        if (memoryDc) {
            DeleteDC(memoryDc);
        }
        if (bitmap) {
            DeleteObject(bitmap);
        }
        return {};
    }

    const HGDIOBJ previousBitmap = SelectObject(memoryDc, bitmap);
    memset(pixels, 0, size_t(cursorWidth) * size_t(cursorHeight) * 4);
    DrawIconEx(memoryDc, 0, 0, cursor, cursorWidth, cursorHeight,
               0, nullptr, DI_NORMAL);
    const QImage image(static_cast<uchar *>(pixels), cursorWidth, cursorHeight,
                       cursorWidth * 4, QImage::Format_ARGB32_Premultiplied);
    QPixmap pixmap = QPixmap::fromImage(image.copy());
    SelectObject(memoryDc, previousBitmap);
    DeleteDC(memoryDc);
    DeleteObject(bitmap);
    pixmap.setDevicePixelRatio(dpr);
    return pixmap;
}

inline QFont windowsDragPillFont() {
    QFont font = QGuiApplication::font();
    font.setPixelSize(13);
    font.setWeight(QFont::DemiBold);
    return font;
}

inline QSizeF windowsDragPillSize(Qt::DropAction action) {
    const QString label = action == Qt::CopyAction
        ? QStringLiteral("Copy") : QStringLiteral("Move");
    const QFontMetricsF metrics(windowsDragPillFont());
    constexpr qreal horizontalPadding = 11.0;
    constexpr qreal iconWidth = 14.0;
    constexpr qreal iconTextSpacing = 7.0;
    constexpr qreal verticalPadding = 6.0;
    return QSizeF(qCeil(horizontalPadding + iconWidth + iconTextSpacing +
                        metrics.horizontalAdvance(label) + horizontalPadding),
                  qCeil(qMax(iconWidth, metrics.height()) +
                        verticalPadding * 2.0));
}

inline QPixmap windowsDragCursorPixmap(qreal dpr, const QSizeF &previewSize,
                                const QPointF &hotSpot,
                                Qt::DropAction action) {
    const bool showPill = action == Qt::CopyAction ||
                          action == Qt::MoveAction;
    const QString label = action == Qt::CopyAction
        ? QStringLiteral("Copy") : QStringLiteral("Move");
    const QFont labelFont = windowsDragPillFont();
    const QSizeF copyPillSize = windowsDragPillSize(Qt::CopyAction);
    const QSizeF movePillSize = windowsDragPillSize(Qt::MoveAction);
    const QSizeF pillSize = action == Qt::CopyAction
        ? copyPillSize : movePillSize;
    const qreal pillWidth = pillSize.width();
    const qreal pillHeight = pillSize.height();
    constexpr qreal shadowExtent = 7.0;
    const QSizeF effectivePreview = previewSize.isEmpty()
        ? QSizeF(52.0, 52.0) : previewSize;
    // QWindowsOleDropSource draws this action cursor with its origin at the
    // pointer. Offset the pill so it sits below the separate drag preview.
    const qreal pillCenterX = -hotSpot.x() +
                              effectivePreview.width() / 2.0;
    const qreal copyPillX = qMax(shadowExtent,
        pillCenterX - copyPillSize.width() / 2.0);
    const qreal movePillX = qMax(shadowExtent,
        pillCenterX - movePillSize.width() / 2.0);
    const qreal pillX = action == Qt::CopyAction
        ? copyPillX : movePillX;
    const qreal pillY = qMax(36.0,
        -hotSpot.y() + effectivePreview.height() + 8.0);
    // Every action cursor must have exactly the same outer geometry. Qt's
    // Windows backend combines this canvas with the drag preview into one
    // HCURSOR; differing sizes make the preview jump and flash on transitions.
    const qreal commonRight = qMax(copyPillX + copyPillSize.width(),
                                   movePillX + movePillSize.width());
    const qreal commonPillHeight = qMax(copyPillSize.height(),
                                        movePillSize.height());
    const QSizeF logicalSize(
        qMax(32.0, commonRight + shadowExtent),
        pillY + commonPillHeight + shadowExtent + 2.0);

    QPixmap pixmap(qCeil(logicalSize.width() * dpr),
                   qCeil(logicalSize.height() * dpr));
    pixmap.fill(Qt::transparent);

    QPainter painter(&pixmap);
    painter.setRenderHint(QPainter::Antialiasing);
    painter.setRenderHint(QPainter::TextAntialiasing);
    painter.scale(dpr, dpr);

    // Use the actual Windows cursor artwork; only the action pill is custom.
    const QPixmap systemCursor = systemCursorPixmap(
        action == Qt::IgnoreAction ? IDC_NO : IDC_ARROW, dpr);
    if (!systemCursor.isNull()) {
        painter.drawPixmap(QPointF(0, 0), systemCursor);
    }

    if (showPill) {
        const QRectF pillRect(pillX, pillY, pillWidth, pillHeight);
        // Approximate a soft Windows-style elevation shadow without baking a
        // harsh, visibly offset duplicate of the pill.
        painter.setPen(Qt::NoPen);
        for (int layer = 7; layer >= 1; --layer) {
            const qreal spread = layer * 0.7;
            const QRectF shadowRect = pillRect
                .adjusted(-spread, -spread * 0.45,
                          spread, spread * 1.25)
                .translated(0, 1.5);
            painter.setBrush(QColor(0, 0, 0, 3 + (7 - layer) * 2));
            painter.drawRoundedRect(shadowRect,
                                    pillHeight / 3.0 + spread,
                                    pillHeight / 3.0 + spread);
        }

        painter.setPen(QPen(QColor(0, 0, 0, 60), 1.0));
        painter.setBrush(QColor(250, 250, 250, 246));
        painter.drawRoundedRect(pillRect, pillHeight / 3.0,
                                pillHeight / 3.0);

        constexpr qreal horizontalPadding = 11.0;
        constexpr qreal iconWidth = 14.0;
        constexpr qreal iconTextSpacing = 7.0;
        const QPointF iconCenter(
            pillX + horizontalPadding + iconWidth / 2.0,
            pillY + pillHeight / 2.0);
        painter.setPen(QPen(QColor(25, 25, 25), 2.0, Qt::SolidLine,
                            Qt::RoundCap, Qt::RoundJoin));
        if (action == Qt::CopyAction) {
            painter.drawLine(iconCenter + QPointF(-4.5, 0),
                             iconCenter + QPointF(4.5, 0));
            painter.drawLine(iconCenter + QPointF(0, -4.5),
                             iconCenter + QPointF(0, 4.5));
        }
        else {
            painter.drawLine(iconCenter + QPointF(-5.0, 0),
                             iconCenter + QPointF(4.5, 0));
            painter.drawLine(iconCenter + QPointF(0.5, -4.0),
                             iconCenter + QPointF(4.5, 0));
            painter.drawLine(iconCenter + QPointF(0.5, 4.0),
                             iconCenter + QPointF(4.5, 0));
        }

        painter.setFont(labelFont);
        painter.setPen(QColor(25, 25, 25));
        const qreal textX = pillX + horizontalPadding + iconWidth +
                            iconTextSpacing;
        const QRectF textRect(
            textX, pillY,
            pillRect.right() - horizontalPadding - textX, pillHeight);
        painter.drawText(textRect, Qt::AlignVCenter | Qt::AlignLeft, label);
    }

    // QWindowsOleDropSource consumes action cursors as physical pixels; it
    // does not apply the drag preview's DPR scaling to a custom cursor.
    pixmap.setDevicePixelRatio(1.0);
    return pixmap;
}

#endif
// BrickDelegate.qml: five 46px tiles, 6px spacing/padding and a +N tile.
inline QPixmap compactPreview(const QList<QImage> &images, int total, qreal dpr, bool dark)
{
    const int shown = qMin(5, int(images.size()));
    const int remaining = qMax(0, total - shown);
    const int tiles = shown + (remaining > 0 ? 1 : 0);
    const QSizeF size(12 + tiles * 46 + qMax(0, tiles - 1) * 6, 58);
    QPixmap result(qCeil(size.width()*dpr), qCeil(size.height()*dpr));
    result.fill(Qt::transparent);
    QPainter p(&result);
    p.setRenderHint(QPainter::Antialiasing);
    p.setRenderHint(QPainter::SmoothPixmapTransform);
    p.scale(dpr, dpr);
    p.setBrush(QColor(dark ? "#303030" : "#d5d5d5"));
    p.setPen(QPen(QColor(dark ? "#404040" : "#bcbcbc"), 1));
    p.drawRoundedRect(QRectF(0.5,0.5,size.width()-1,57),6,6);
    for (int i=0; i<tiles; ++i) {
        QRectF tile(6+i*52,6,46,46);
        p.setBrush(i<shown ? QColor(0,0,0,dark?77:51) : QColor(dark?255:0,dark?255:0,dark?255:0,15));
        p.setPen(QPen(i<shown ? QColor(dark?255:0,dark?255:0,dark?255:0,20) : QColor("#ffd43b"),1));
        p.drawRoundedRect(tile.adjusted(0.5,0.5,-0.5,-0.5),4,4);
        if (i<shown) {
            const auto &im=images[i];
            if (!im.isNull()) {
                const QSizeF fit=im.size().scaled(QSize(40,40),Qt::KeepAspectRatio);
                const QPointF origin=tile.center()-QPointF(fit.width()/2,fit.height()/2);
                p.drawImage(QRectF(origin,fit),im);
            }
        } else {
            QFont font=QGuiApplication::font(); font.setBold(true); p.setFont(font);
            p.setPen(dark?Qt::white:Qt::black);
            p.drawText(tile,Qt::AlignCenter,QString("+%1").arg(remaining));
        }
    }
    p.end(); result.setDevicePixelRatio(dpr); return result;
}
}
