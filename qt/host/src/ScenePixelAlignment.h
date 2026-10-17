#pragma once

#include <QObject>
#include <QPointF>
#include <QPointer>
#include <QtQml>
#include <ZoinGallery/PixelGrid.h>

class QQuickItem;

// A translation in the item parent's coordinates, evaluated after layout and
// before scene synchronization. No scale or raster resampling is introduced.
class ScenePixelAlignment : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_ATTACHED(ScenePixelAlignment)
    QML_UNCREATABLE("Use the attached offset on a visual item")
    Q_PROPERTY(QPointF offset READ offset NOTIFY offsetChanged)
    Q_PROPERTY(ZoinGallery::PixelGridContext *grid READ grid NOTIFY gridChanged)
public:
    explicit ScenePixelAlignment(QObject *parent = nullptr);
    ~ScenePixelAlignment() override;
    static ScenePixelAlignment *qmlAttachedProperties(QObject *object);
    QPointF offset() const;
    ZoinGallery::PixelGridContext *grid() const;
    QQuickItem *item() const;
    void align();
signals:
    void offsetChanged();
    void gridChanged();
private:
    QPointer<QQuickItem> m_item;
    QPointer<ZoinGallery::PixelGridAlignment> m_alignment;
};
QML_DECLARE_TYPEINFO(ScenePixelAlignment, QML_HAS_ATTACHED_PROPERTIES)
