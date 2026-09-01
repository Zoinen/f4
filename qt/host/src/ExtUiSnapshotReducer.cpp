#include "ExtUiSceneReducer.h"

#include <QMetaType>

#include <utility>

namespace ExtUiSceneReducer
{
bool isAuthoritativePhasedCatalog(const QVariantMap &panel)
{
    const QVariant metadataDeferred = panel.value(
        QStringLiteral("metadataDeferred"));
    const QVariant catalogProvisional = panel.value(
        QStringLiteral("catalogProvisional"));
    qulonglong catalogRevision = 0;
    qulonglong totalCount = 0;
    return metadataDeferred.metaType().id() == QMetaType::Bool
        && metadataDeferred.toBool()
        && catalogProvisional.metaType().id() == QMetaType::Bool
        && !catalogProvisional.toBool()
        && nonNegativeInteger(panel.value(QStringLiteral("catalogRevision")),
                              &catalogRevision)
        && catalogRevision > 0
        && nonNegativeInteger(panel.value(QStringLiteral("totalCount")),
                              &totalCount);
}

bool shellSideIsCovered(const QVariantMap &shell, int side)
{
    for (const QString &key : {QStringLiteral("infoPanels"),
                               QStringLiteral("quickViews")}) {
        const QVariantList covers = shell.value(key).toList();
        for (const QVariant &coverValue : covers) {
            const QVariantMap cover = coverValue.toMap();
            if (!cover.isEmpty()
                && cover.value(QStringLiteral("side")).toInt() == side) {
                return true;
            }
        }
    }
    return false;
}

QVariantList projectCommandMenuStates(const QVariantList &menus)
{
    QVariantList states;
    states.reserve(menus.size());
    for (const QVariant &menuValue : menus) {
        const QVariantMap menu = menuValue.toMap();
        if (menu.isEmpty()) {
            continue;
        }
        states.push_back(QVariantMap{
            {QStringLiteral("id"), menu.value(QStringLiteral("id"))},
            {QStringLiteral("selected"),
             menu.value(QStringLiteral("selected"))},
            {QStringLiteral("top"), menu.value(QStringLiteral("top"))},
        });
    }
    return states;
}

QVariantMap commandMenuStructure(QVariantMap menu)
{
    menu.remove(QStringLiteral("selected"));
    menu.remove(QStringLiteral("top"));
    return menu;
}

bool commandMenuStructuresEqual(const QVariantList &left,
                                const QVariantList &right)
{
    if (left.size() != right.size()) {
        return false;
    }
    for (qsizetype index = 0; index < left.size(); ++index) {
        if (commandMenuStructure(left.at(index).toMap())
            != commandMenuStructure(right.at(index).toMap())) {
            return false;
        }
    }
    return true;
}

QVariantMap mergeSnapshotShell(const QVariantMap &current,
                               QVariantMap replacement)
{
    if (!replacement.contains(QStringLiteral("commandLine"))
        && current.contains(QStringLiteral("commandLine"))) {
        replacement.insert(QStringLiteral("commandLine"),
                           current.value(QStringLiteral("commandLine")));
    }
    QVariantList nextPanels = replacement.value(
        QStringLiteral("panels")).toList();
    const QVariantList currentPanels = current.value(
        QStringLiteral("panels")).toList();
    for (qsizetype index = 0; index < nextPanels.size(); ++index) {
        QVariantMap nextPanel = nextPanels.at(index).toMap();
        if (nextPanel.contains(QStringLiteral("entries"))) {
            continue;
        }
        const int side = nextPanel.value(QStringLiteral("side"), index).toInt();
        for (const QVariant &currentValue : currentPanels) {
            const QVariantMap currentPanel = currentValue.toMap();
            if (currentPanel.value(QStringLiteral("side")).toInt() != side) {
                continue;
            }
            for (const QString &catalogKey : {
                     QStringLiteral("entries"),
                     QStringLiteral("highlightStyles")}) {
                if (currentPanel.contains(catalogKey)) {
                    nextPanel.insert(catalogKey,
                                     currentPanel.value(catalogKey));
                }
            }
            break;
        }
        nextPanels[index] = nextPanel;
    }
    replacement.insert(QStringLiteral("panels"), nextPanels);
    return replacement;
}

QString snapshotPayloadTypeForStream(const QString &streamId)
{
    if (streamId == QStringLiteral("chrome")) {
        return QStringLiteral("chrome_snapshot");
    }
    if (streamId == QStringLiteral("workspaces")) {
        return QStringLiteral("workspaces_snapshot");
    }
    if (streamId == QStringLiteral("menus")) {
        return QStringLiteral("menus_snapshot");
    }
    if (streamId == QStringLiteral("dialogs")) {
        return QStringLiteral("dialogs_snapshot");
    }
    if (streamId == QStringLiteral("operations")) {
        return QStringLiteral("operations_snapshot");
    }
    if (streamId == QStringLiteral("command-line")) {
        return QStringLiteral("command_line_snapshot");
    }
    if (streamId == QStringLiteral("shell")) {
        return QStringLiteral("shell_snapshot");
    }
    if (streamId.startsWith(QStringLiteral("panel/"))
        || streamId.startsWith(QStringLiteral("panel-id/"))) {
        return QStringLiteral("panel_catalog_snapshot");
    }
    if (streamId.startsWith(QStringLiteral("document/"))) {
        return QStringLiteral("document_snapshot");
    }
    return {};
}

bool applyStreamSnapshotPayload(const QString &streamId,
                                const QVariantMap &message,
                                QVariantMap *scene,
                                QVariantMap *catalogPanel,
                                QString *error)
{
    if (!scene || !catalogPanel) {
        return false;
    }
    const QString expectedType = snapshotPayloadTypeForStream(streamId);
    const QVariant stateValue = message.value(QStringLiteral("state"));
    if (streamId.isEmpty() || expectedType.isEmpty()
        || message.value(QStringLiteral("type")).toString() != expectedType
        || stateValue.metaType().id() != QMetaType::QVariantMap) {
        if (error) {
            *error = QStringLiteral("Invalid ExtUI stream snapshot payload");
        }
        return false;
    }
    const QVariantMap state = stateValue.toMap();
    if (streamId == QStringLiteral("shell")) {
        const QVariant shellValue = state.value(QStringLiteral("shell"));
        if (shellValue.metaType().id() != QMetaType::QVariantMap) {
            if (error) {
                *error = QStringLiteral("Shell snapshot has no shell state");
            }
            return false;
        }
        scene->insert(QStringLiteral("shell"), mergeSnapshotShell(
            scene->value(QStringLiteral("shell")).toMap(),
            shellValue.toMap()));
        return true;
    }
    if (streamId == QStringLiteral("command-line")) {
        QVariantMap shell = scene->value(QStringLiteral("shell")).toMap();
        shell.insert(QStringLiteral("commandLine"),
                     state.value(QStringLiteral("commandLine")));
        scene->insert(QStringLiteral("shell"), shell);
        return true;
    }
    if (streamId.startsWith(QStringLiteral("panel/"))
        || streamId.startsWith(QStringLiteral("panel-id/"))) {
        const QVariant panelValue = state.value(QStringLiteral("panel"));
        bool sideOK = false;
        const int side = state.value(QStringLiteral("side")).toInt(&sideOK);
        if (!sideOK || side < 0 || side > 1
            || panelValue.metaType().id() != QMetaType::QVariantMap) {
            if (error) {
                *error = QStringLiteral("Panel snapshot has invalid state");
            }
            return false;
        }
        QVariantMap shell = scene->value(QStringLiteral("shell")).toMap();
        QVariantList panels = shell.value(QStringLiteral("panels")).toList();
        while (panels.size() <= side) {
            panels.push_back(QVariantMap{});
        }
        *catalogPanel = panelValue.toMap();
        panels[side] = *catalogPanel;
        shell.insert(QStringLiteral("panels"), panels);
        scene->insert(QStringLiteral("shell"), shell);
        return true;
    }
    if (streamId.startsWith(QStringLiteral("document/"))) {
        if (state.contains(QStringLiteral("surface"))) {
            scene->insert(QStringLiteral("surface"),
                          state.value(QStringLiteral("surface")));
        } else {
            scene->remove(QStringLiteral("surface"));
        }
        return true;
    }

    QStringList ownedKeys;
    if (streamId == QStringLiteral("chrome")) {
        ownedKeys = {QStringLiteral("schema"), QStringLiteral("version"),
                     QStringLiteral("width"), QStringLiteral("height"),
                     QStringLiteral("presentation"),
                     QStringLiteral("qmlIconSet"),
                     QStringLiteral("keyBar"), QStringLiteral("toast")};
    } else if (streamId == QStringLiteral("workspaces")) {
        ownedKeys = {QStringLiteral("activeScreen"),
                     QStringLiteral("workspaceCount"),
                     QStringLiteral("workspaceTabs")};
    } else if (streamId == QStringLiteral("menus")) {
        ownedKeys = {QStringLiteral("menuBar"), QStringLiteral("menus")};
    } else if (streamId == QStringLiteral("dialogs")) {
        ownedKeys = {QStringLiteral("dialogs")};
    } else if (streamId == QStringLiteral("operations")) {
        ownedKeys = {QStringLiteral("operationsQueue")};
    } else {
        if (error) {
            *error = QStringLiteral("Unknown ExtUI snapshot stream");
        }
        return false;
    }
    for (const QString &key : ownedKeys) {
        if (state.contains(key)) {
            scene->insert(key, state.value(key));
        } else {
            scene->remove(key);
        }
    }
    return true;
}

}
