#include "ScenePixelAlignment.h"

#include <QCoreApplication>
#include <QQuickItem>
#include <QQuickWindow>
#include <algorithm>
#include <cmath>

// Qt emits afterAnimating on the GUI thread after polishing the layout, before
// synchronizing the scene graph (also for QQuickRenderControl). All ancestors
// must be corrected before their descendants read the final scene transform.
class ScenePixelAlignmentPass final : public QObject
{
public:
    explicit ScenePixelAlignmentPass(QQuickWindow *window) : QObject(window)
    {
        setObjectName(QStringLiteral("_f4ScenePixelAlignmentPass"));
        connect(window, &QQuickWindow::afterAnimating, this, [this] { alignItems(); });
    }
    void add(ScenePixelAlignment *alignment) { m_items.append(alignment); }
    void remove(ScenePixelAlignment *alignment) { m_items.removeAll(alignment); }
private:
    void alignItems()
    {
        struct Entry { QPointer<ScenePixelAlignment> alignment; int depth; };
        QList<Entry> ordered;
        ordered.reserve(m_items.size());
        for (auto *alignment : std::as_const(m_items)) {
            int depth = 0;
            for (auto *item = alignment->item(); item; item = item->parentItem()) ++depth;
            ordered.append({alignment, depth});
        }
        std::stable_sort(ordered.begin(), ordered.end(), [](const Entry &a, const Entry &b) {
            return a.depth < b.depth;
        });
        for (const auto &entry : std::as_const(ordered))
            if (entry.alignment) entry.alignment->align();
    }
    QList<ScenePixelAlignment *> m_items;
};

ScenePixelAlignment::ScenePixelAlignment(QObject *parent)
    : QObject(parent), m_item(qobject_cast<QQuickItem *>(parent))
{
    if (m_item) {
        connect(m_item, &QQuickItem::windowChanged, this, &ScenePixelAlignment::attachWindow);
        attachWindow();
    }
}
ScenePixelAlignment::~ScenePixelAlignment()
{
    if (m_pass) m_pass->remove(this);
}
QQuickItem *ScenePixelAlignment::item() const { return m_item; }

void ScenePixelAlignment::attachWindow()
{
    if (m_pass) m_pass->remove(this);
    m_pass = nullptr;
    auto *window = m_item ? m_item->window() : nullptr;
    if (!window) return;
    auto *existing = window->findChild<QObject *>(QStringLiteral("_f4ScenePixelAlignmentPass"),
                                                  Qt::FindDirectChildrenOnly);
    auto *pass = existing ? static_cast<ScenePixelAlignmentPass *>(existing)
                          : new ScenePixelAlignmentPass(window);
    m_pass = pass;
    pass->add(this);
    window->update();
}

void ScenePixelAlignment::align()
{
    if (!m_item || !m_item->parentItem() || !m_item->window()) return;
    auto *window = m_item->window();
    auto *parent = m_item->parentItem();
    const auto origin = parent->mapToItem(window->contentItem(), m_item->position());
    const qreal dpr = window->effectiveDevicePixelRatio();
    const QPointF snapped(std::round(origin.x() * dpr) / dpr,
                          std::round(origin.y() * dpr) / dpr);
    const auto offset = parent->mapFromItem(window->contentItem(), snapped) - m_item->position();
    if (std::abs(offset.x() - m_offset.x()) < 0.0000001
        && std::abs(offset.y() - m_offset.y()) < 0.0000001) return;
    m_offset = offset;
    emit offsetChanged();
}

ScenePixelAlignment *ScenePixelAlignment::qmlAttachedProperties(QObject *object)
{
    return new ScenePixelAlignment(object);
}
