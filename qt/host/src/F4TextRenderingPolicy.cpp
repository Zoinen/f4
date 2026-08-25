#include "F4TextRenderingPolicy.h"

#include <QGuiApplication>
#include <QMetaProperty>
#include <QQuickItem>
#include <QQuickWindow>
#include <QVariantMap>

namespace
{
QVariantMap makeOption(int value, const QString &name, const QString &description)
{
    return {
        {QStringLiteral("value"), value},
        {QStringLiteral("name"), name},
        {QStringLiteral("description"), description},
    };
}
}

F4TextRenderingPolicy::F4TextRenderingPolicy(QObject *parent)
    : QObject(parent)
    , m_renderType(int(QQuickWindow::NativeTextRendering))
{
    // Native rendering is the product default. Set it before the QML engine
    // creates any text items so their initial render type is deterministic.
    QQuickWindow::setTextRenderType(
        QQuickWindow::NativeTextRendering);
}

int F4TextRenderingPolicy::renderType() const
{
    return m_renderType;
}

void F4TextRenderingPolicy::setRenderType(int renderType)
{
    if (!isSupported(renderType))
        return;

    const bool valueChanged = m_renderType != renderType;
    m_renderType = renderType;
    QQuickWindow::setTextRenderType(
        static_cast<QQuickWindow::TextRenderType>(renderType));
    applyToExistingItems();

    if (valueChanged)
        emit renderTypeChanged();
}

QString F4TextRenderingPolicy::renderTypeName() const
{
    const QVariantList available = options();
    for (const QVariant &value : available) {
        const QVariantMap option = value.toMap();
        if (option.value(QStringLiteral("value")).toInt() == m_renderType)
            return option.value(QStringLiteral("name")).toString();
    }
    return QStringLiteral("NativeRendering");
}

QVariantList F4TextRenderingPolicy::options() const
{
    QVariantList result;
    result.append(makeOption(
        int(QQuickWindow::QtTextRendering),
        QStringLiteral("QtRendering"),
        QStringLiteral("Qt distance-field rendering")));
    result.append(makeOption(
        int(QQuickWindow::NativeTextRendering),
        QStringLiteral("NativeRendering"),
        QStringLiteral("Native platform text rasterization")));
#if QT_VERSION >= QT_VERSION_CHECK(6, 8, 0)
    result.append(makeOption(
        int(QQuickWindow::CurveTextRendering),
        QStringLiteral("CurveRendering"),
        QStringLiteral("Curve-based vector glyph rendering")));
#endif
    return result;
}

bool F4TextRenderingPolicy::setRenderTypeByName(const QString &value)
{
    const QString normalized = value.trimmed();
    if (normalized.isEmpty())
        return false;

    bool numericOk = false;
    const int numericValue = normalized.toInt(&numericOk);
    if (numericOk && isSupported(numericValue)) {
        setRenderType(numericValue);
        return true;
    }

    for (const QVariant &optionValue : options()) {
        const QVariantMap option = optionValue.toMap();
        const QString name = option.value(QStringLiteral("name")).toString();
        if (name.compare(normalized, Qt::CaseInsensitive) == 0) {
            setRenderType(option.value(QStringLiteral("value")).toInt());
            return true;
        }
    }
    return false;
}

void F4TextRenderingPolicy::applyToExistingItems() const
{
    const auto applyToItem = [this](auto &&self, QQuickItem *item) -> void {
        if (!item)
            return;

        const int propertyIndex =
            item->metaObject()->indexOfProperty("renderType");
        if (propertyIndex >= 0) {
            const QMetaProperty property =
                item->metaObject()->property(propertyIndex);
            if (property.isWritable())
                item->setProperty("renderType", m_renderType);
        }

        const QList<QQuickItem *> children = item->childItems();
        for (QQuickItem *child : children)
            self(self, child);
    };

    for (QWindow *window : QGuiApplication::allWindows()) {
        auto *quickWindow = qobject_cast<QQuickWindow *>(window);
        if (!quickWindow || !quickWindow->contentItem())
            continue;
        applyToItem(applyToItem, quickWindow->contentItem());
    }
}

bool F4TextRenderingPolicy::isSupported(int renderType) const
{
    for (const QVariant &optionValue : options()) {
        if (optionValue.toMap().value(QStringLiteral("value")).toInt()
            == renderType) {
            return true;
        }
    }
    return false;
}
