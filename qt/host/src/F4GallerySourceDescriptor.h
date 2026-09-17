#pragma once

#include <QVariantMap>

namespace F4GallerySourceDescriptor {
inline QVariant descriptorValue(const QVariantMap &source,
                         const QVariantMap &descriptor,
                         const QString &key,
                         const QString &legacyKey = QString())
{
    if (descriptor.contains(key)) {
        return descriptor.value(key);
    }
    return source.value(legacyKey.isEmpty() ? key : legacyKey);
}

inline void insertSourceDescriptorFields(QVariantMap *entry,
                                  const QVariantMap &source)
{
    const QVariantMap descriptor = source.value(
        QStringLiteral("source")).toMap();
    for (const QString &key : {QStringLiteral("resourceId"),
                               QStringLiteral("sourceKey")}) {
        const QVariant value = descriptorValue(source, descriptor, key);
        if (value.isValid()) {
            entry->insert(key, value);
        }
    }
    QVariant contentVersion = descriptorValue(
        source, descriptor, QStringLiteral("version"));
    if (!contentVersion.isValid()) {
        contentVersion = descriptorValue(
            source, descriptor, QStringLiteral("contentVersion"));
    }
    if (contentVersion.isValid()) {
        entry->insert(QStringLiteral("contentVersion"), contentVersion);
        entry->insert(QStringLiteral("version"), contentVersion);
    }
    for (const QString &key : {
             QStringLiteral("versionStrength"),
             QStringLiteral("storageClass"),
             QStringLiteral("accessProfile"),
             QStringLiteral("mimeType"),
             QStringLiteral("sizeKnown")}) {
        const QVariant value = descriptorValue(source, descriptor, key);
        if (value.isValid()) {
            entry->insert(key, value);
        }
    }
    const QVariant size = descriptorValue(
        source, descriptor, QStringLiteral("size"));
    if (size.isValid()) {
        entry->insert(QStringLiteral("size"), size);
    }
}

}
