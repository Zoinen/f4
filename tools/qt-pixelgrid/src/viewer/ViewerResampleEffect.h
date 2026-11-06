#pragma once

#include <QPointer>
#include <QQuickItem>
#include <QSizeF>
#include <QVector2D>
#include <QtQmlIntegration/qqmlintegration.h>

// Supplies the final image shader with the render target's physical viewport.
// The QML wrapper owns source selection and its retained half-size pyramid.
class ViewerResampleEffect : public QQuickItem
{
    Q_OBJECT
    QML_ELEMENT

    Q_PROPERTY(QQuickItem *source READ source WRITE setSource NOTIFY sourceChanged)
    Q_PROPERTY(QSizeF viewportSize READ viewportSize WRITE setViewportSize NOTIFY viewportSizeChanged)
    Q_PROPERTY(QSizeF sourceExtent READ sourceExtent WRITE setSourceExtent NOTIFY sourceExtentChanged)
    Q_PROPERTY(QVector2D checkerboardOffset READ checkerboardOffset WRITE setCheckerboardOffset NOTIFY checkerboardOffsetChanged)
    Q_PROPERTY(bool showCheckerboard READ showCheckerboard WRITE setShowCheckerboard NOTIFY showCheckerboardChanged)
    Q_PROPERTY(int checkerboardSize READ checkerboardSize WRITE setCheckerboardSize NOTIFY checkerboardSizeChanged)
    Q_PROPERTY(qreal borderRadius READ borderRadius WRITE setBorderRadius NOTIFY borderRadiusChanged)
    Q_PROPERTY(bool intermediate READ intermediate WRITE setIntermediate NOTIFY intermediateChanged)
    Q_PROPERTY(bool pixelAligned READ pixelAligned WRITE setPixelAligned NOTIFY pixelAlignedChanged)
    Q_PROPERTY(bool pixelAlignedIdentity READ pixelAlignedIdentity WRITE setPixelAlignedIdentity NOTIFY pixelAlignedIdentityChanged)

public:
    explicit ViewerResampleEffect(QQuickItem *parent = nullptr);

    QQuickItem *source() const { return m_source; }
    void setSource(QQuickItem *source);
    QSizeF viewportSize() const { return m_viewportSize; }
    void setViewportSize(QSizeF size);
    QSizeF sourceExtent() const { return m_sourceExtent; }
    void setSourceExtent(QSizeF size);
    QVector2D checkerboardOffset() const { return m_checkerboardOffset; }
    void setCheckerboardOffset(QVector2D offset);
    bool showCheckerboard() const { return m_showCheckerboard; }
    void setShowCheckerboard(bool enabled);
    int checkerboardSize() const { return m_checkerboardSize; }
    void setCheckerboardSize(int size);
    qreal borderRadius() const { return m_borderRadius; }
    void setBorderRadius(qreal radius);
    bool intermediate() const { return m_intermediate; }
    void setIntermediate(bool enabled);
    bool pixelAligned() const { return m_pixelAligned; }
    void setPixelAligned(bool enabled);
    bool pixelAlignedIdentity() const { return m_pixelAlignedIdentity; }
    void setPixelAlignedIdentity(bool enabled);

signals:
    void sourceChanged();
    void viewportSizeChanged();
    void sourceExtentChanged();
    void checkerboardOffsetChanged();
    void showCheckerboardChanged();
    void checkerboardSizeChanged();
    void borderRadiusChanged();
    void intermediateChanged();
    void pixelAlignedChanged();
    void pixelAlignedIdentityChanged();

protected:
    QSGNode *updatePaintNode(QSGNode *oldNode, UpdatePaintNodeData *) override;
    void geometryChange(const QRectF &newGeometry, const QRectF &oldGeometry) override;

private:
    QPointer<QQuickItem> m_source;
    QMetaObject::Connection m_sourceDestroyed;
    QMetaObject::Connection m_sourceWindowChanged;
    QSizeF m_viewportSize{0, 0};
    QSizeF m_sourceExtent{0, 0};
    QVector2D m_checkerboardOffset;
    bool m_showCheckerboard = false;
    int m_checkerboardSize = 4;
    qreal m_borderRadius = 0;
    bool m_intermediate = false;
    bool m_pixelAligned = false;
    bool m_pixelAlignedIdentity = false;
};
