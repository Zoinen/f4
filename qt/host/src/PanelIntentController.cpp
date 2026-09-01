#include "PanelIntentController.h"

PanelIntentController::PanelIntentController(QObject *parent)
    : QObject(parent)
{
    qRegisterMetaType<PanelIntent>();
}

void PanelIntentController::dispatch(const PanelIntent &intent)
{
    emit intentRequested(intent);
}

QVariantMap PanelIntentController::toWireMap(const PanelIntent &intent)
{
    QString action;
    switch (intent.kind) {
    case PanelIntent::Kind::Activate:
        action = QStringLiteral("panel.activate");
        break;
    case PanelIntent::Kind::Cursor:
        action = QStringLiteral("panel.cursor");
        break;
    case PanelIntent::Kind::Open:
        action = QStringLiteral("panel.open");
        break;
    case PanelIntent::Kind::SetSelection:
        action = QStringLiteral("panel.setSelection");
        break;
    case PanelIntent::Kind::SetGalleryLayout:
        action = QStringLiteral("panel.setGalleryLayout");
        break;
    case PanelIntent::Kind::SetGalleryDensity:
        action = QStringLiteral("panel.setGalleryDensity");
        break;
    case PanelIntent::Kind::Sort:
        action = QStringLiteral("panel.sort");
        break;
    case PanelIntent::Kind::SortMenu:
        action = QStringLiteral("panel.sortMenu");
        break;
    }

    QVariantMap wire{
        {QStringLiteral("action"), action},
        {QStringLiteral("side"), intent.side},
    };
    if (!intent.entryId.isEmpty()) {
        wire.insert(QStringLiteral("entryId"), intent.entryId);
    }
    if (intent.index >= 0) {
        wire.insert(QStringLiteral("index"), intent.index);
    }
    if (intent.includeCatalogRevision && intent.catalogRevision != 0) {
        wire.insert(QStringLiteral("catalogRevision"),
                    intent.catalogRevision);
    }
    if (intent.activate) {
        wire.insert(QStringLiteral("activate"), true);
    }

    switch (intent.kind) {
    case PanelIntent::Kind::SetSelection:
        wire.insert(QStringLiteral("mode"), intent.mode);
        if (!intent.entryIds.isEmpty() || intent.changes.isEmpty()) {
            wire.insert(QStringLiteral("entryIds"), intent.entryIds);
        }
        if (!intent.changes.isEmpty()) {
            wire.insert(QStringLiteral("changes"), intent.changes);
        }
        if (!intent.cursorEntryId.isEmpty()) {
            wire.insert(QStringLiteral("cursorEntryId"),
                        intent.cursorEntryId);
            wire.insert(QStringLiteral("cursorIndex"), intent.cursorIndex);
        }
        if (intent.selectionRevision != 0) {
            wire.insert(QStringLiteral("selectionRevision"),
                        intent.selectionRevision);
        }
        break;
    case PanelIntent::Kind::SetGalleryLayout:
        wire.insert(QStringLiteral("layoutMode"), intent.layoutMode);
        if (intent.columnCount > 0) {
            wire.insert(QStringLiteral("columnCount"), intent.columnCount);
        }
        break;
    case PanelIntent::Kind::SetGalleryDensity:
        wire.insert(QStringLiteral("layoutMode"), intent.layoutMode);
        wire.insert(QStringLiteral("density"), intent.density);
        break;
    case PanelIntent::Kind::Sort:
        wire.insert(QStringLiteral("mode"), intent.mode);
        break;
    default:
        break;
    }
    return wire;
}
