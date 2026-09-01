#include "PanelCatalogModel.h"

bool PanelCatalogModel::rowLoaded(int row) const
{
    return !entryAt(row).value(QStringLiteral("entryId")).toString().isEmpty();
}

int PanelCatalogModel::entryCount() const
{
    return catalogRowsDeferred ? totalCount : entries.size();
}

QVariantMap PanelCatalogModel::entryAt(int row) const
{
    if (row < 0 || row >= entryCount()) {
        return {};
    }
    if (!catalogRowsDeferred) {
        return row < entries.size() ? entries.at(row).toMap() : QVariantMap{};
    }
    const auto offset = entryOffsetByRow.constFind(row);
    if (offset == entryOffsetByRow.cend()
        || offset.value() < 0 || offset.value() >= entries.size()) {
        return {};
    }
    return entries.at(offset.value()).toMap();
}

bool PanelCatalogModel::setEntry(int row, const QVariant &entry)
{
    if (row < 0 || row >= entryCount()
        || entry.metaType().id() != QMetaType::QVariantMap) {
        return false;
    }
    if (!catalogRowsDeferred) {
        if (row >= entries.size()) {
            return false;
        }
        entries[row] = entry;
        return true;
    }
    const auto existing = entryOffsetByRow.constFind(row);
    if (existing != entryOffsetByRow.cend()) {
        entries[existing.value()] = entry;
        return true;
    }
    entryOffsetByRow.insert(row, entries.size());
    entries.push_back(entry);
    return true;
}
