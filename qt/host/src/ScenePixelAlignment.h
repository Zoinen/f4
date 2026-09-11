#pragma once

#include <QObject>
#include <QPointF>
#include <QPointer>
#include <QtQml>

class QQuickItem;
class ScenePixelAlignmentPass;

// A translation in the item parent's coordinates, evaluated after layout and
// before scene synchronization. No scale or raster resampling is introduced.
class ScenePixelAlignment : public QObject
{
    Q_OBJECT
    QML_ELEMENT
    QML_ATTACHED(ScenePixelAlignment)
    QML_UNCREATABLE("Use the attached offset on a visual item")
    Q_PROPERTY(QPointF offset READ offset NOTIFY offsetChanged)
public:
    explicit ScenePixelAlignment(QObject *parent = nullptr);
    ~ScenePixelAlignment() override;
    static ScenePixelAlignment *qmlAttachedProperties(QObject *object);
    QPointF offset() const { return m_offset; }
    QQuickItem *item() const;
    void align();
signals:
    void offsetChanged();
private:
    void attachWindow();
    QPointer<QQuickItem> m_item;
    QPointer<ScenePixelAlignmentPass> m_pass;
    QPointF m_offset;
};
QML_DECLARE_TYPEINFO(ScenePixelAlignment, QML_HAS_ATTACHED_PROPERTIES)
