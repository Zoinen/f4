#include "ScenePixelAlignment.h"

#include <QQuickItem>

ScenePixelAlignment::ScenePixelAlignment(QObject *parent)
    : QObject(parent), m_item(qobject_cast<QQuickItem *>(parent)),
      m_alignment(ZoinGallery::PixelGridAlignment::forItem(m_item))
{
    if (m_alignment) {
        connect(m_alignment, &ZoinGallery::PixelGridAlignment::offsetChanged,
                this, &ScenePixelAlignment::offsetChanged);
        connect(m_alignment, &ZoinGallery::PixelGridAlignment::gridChanged,
                this, &ScenePixelAlignment::gridChanged);
    }
}

ScenePixelAlignment::~ScenePixelAlignment() = default;
QQuickItem *ScenePixelAlignment::item() const { return m_item; }
QPointF ScenePixelAlignment::offset() const { return m_alignment ? m_alignment->offset() : QPointF(); }
ZoinGallery::PixelGridContext *ScenePixelAlignment::grid() const { return m_alignment ? m_alignment->grid() : nullptr; }
void ScenePixelAlignment::align() { if (m_alignment) m_alignment->invalidate(); }
ScenePixelAlignment *ScenePixelAlignment::qmlAttachedProperties(QObject *object)
{
    return new ScenePixelAlignment(object);
}
