#include "ExtUiSceneReducer.h"

#include <QMetaType>

#include <limits>
#include <utility>

namespace ExtUiSceneReducer
{
bool nonNegativeInteger(const QVariant &value, qulonglong *result)
{
    const int type = value.metaType().id();
    const bool signedInteger = type == QMetaType::Char
        || type == QMetaType::SChar || type == QMetaType::Short
        || type == QMetaType::Int || type == QMetaType::LongLong;
    const bool unsignedInteger = type == QMetaType::UChar
        || type == QMetaType::UShort || type == QMetaType::UInt
        || type == QMetaType::ULongLong;
    if (!signedInteger && !unsignedInteger) {
        return false;
    }

    bool ok = false;
    qulonglong converted = 0;
    if (signedInteger) {
        const qlonglong signedValue = value.toLongLong(&ok);
        if (!ok || signedValue < 0) {
            return false;
        }
        converted = static_cast<qulonglong>(signedValue);
    } else {
        converted = value.toULongLong(&ok);
        if (!ok) {
            return false;
        }
    }
    if (result) {
        *result = converted;
    }
    return true;
}

bool integerValue(const QVariant &value, qlonglong *result)
{
    const int type = value.metaType().id();
    const bool integer = type == QMetaType::Char
        || type == QMetaType::SChar || type == QMetaType::UChar
        || type == QMetaType::Short || type == QMetaType::UShort
        || type == QMetaType::Int || type == QMetaType::UInt
        || type == QMetaType::LongLong || type == QMetaType::ULongLong;
    if (!integer) {
        return false;
    }
    bool ok = false;
    const qlonglong converted = value.toLongLong(&ok);
    if (!ok) {
        return false;
    }
    if (result) {
        *result = converted;
    }
    return true;
}

bool valueHasType(const QVariant &value, QMetaType::Type type)
{
    return value.metaType().id() == type;
}

bool validRootPatchValue(const QString &key, const QVariant &value)
{
    if (key == QStringLiteral("width") || key == QStringLiteral("height")
        || key == QStringLiteral("activeScreen")
        || key == QStringLiteral("workspaceCount")) {
        qulonglong number = 0;
        return nonNegativeInteger(value, &number)
            && ((key != QStringLiteral("width")
                 && key != QStringLiteral("height")) || number > 0);
    }
    if (key == QStringLiteral("presentation")
        || key == QStringLiteral("qmlIconSet")) {
        return valueHasType(value, QMetaType::QString);
    }
    if (key == QStringLiteral("dialogs") || key == QStringLiteral("menus")) {
        return valueHasType(value, QMetaType::QVariantList);
    }
    return valueHasType(value, QMetaType::QVariantMap);
}

bool validShellPatchValue(const QString &key, const QVariant &value)
{
    if (key == QStringLiteral("id") || key == QStringLiteral("kind")
        || key == QStringLiteral("title") || key == QStringLiteral("mode")
        || key == QStringLiteral("reason")) {
        return valueHasType(value, QMetaType::QString);
    }
    if (key == QStringLiteral("activePanel")
        || key == QStringLiteral("widePanel")) {
        qlonglong number = 0;
        return integerValue(value, &number)
            && (key != QStringLiteral("activePanel")
                || (number >= 0 && number <= 1));
    }
    if (key == QStringLiteral("infoPanels")
        || key == QStringLiteral("quickViews")) {
        return valueHasType(value, QMetaType::QVariantList);
    }
    if (key == QStringLiteral("commandLine")
        || key == QStringLiteral("terminal")) {
        return valueHasType(value, QMetaType::QVariantMap);
    }
    return valueHasType(value, QMetaType::Bool);
}

bool validSurfaceStatePatchValue(const QString &key, const QVariant &value)
{
    if (key == QStringLiteral("cursorVisible")) {
        return valueHasType(value, QMetaType::Bool);
    }
    if (key == QStringLiteral("cursorShape")) {
        if (!valueHasType(value, QMetaType::QString)) {
            return false;
        }
        const QString shape = value.toString();
        return shape == QStringLiteral("underline")
            || shape == QStringLiteral("block");
    }
    if (key == QStringLiteral("topBarRight")) {
        return valueHasType(value, QMetaType::QString);
    }
    qlonglong number = 0;
    if (!integerValue(value, &number)) {
        return false;
    }
    if (key == QStringLiteral("cursorLine")
        || key == QStringLiteral("cursorPos")
        || key == QStringLiteral("cursorAbsoluteRow")) {
        return number >= 0;
    }
    return key == QStringLiteral("cursorVisualRow")
        || key == QStringLiteral("cursorVisualColumn");
}

bool applyValidatedMapPatch(QVariantMap &target, const QVariant &patchValue,
                            const QSet<QString> &allowedKeys,
                            bool (*validValue)(const QString &,
                                               const QVariant &),
                            QSet<QString> *changedKeys, QString *error)
{
    if (patchValue.metaType().id() != QMetaType::QVariantMap) {
        *error = QStringLiteral("Scene patch section must be a map");
        return false;
    }
    const QVariantMap patch = patchValue.toMap();
    for (auto it = patch.cbegin(); it != patch.cend(); ++it) {
        if (it.key() != QStringLiteral("set")
            && it.key() != QStringLiteral("clear")) {
            *error = QStringLiteral("Unknown scene map-patch field");
            return false;
        }
    }

    QSet<QString> touched;
    if (patch.contains(QStringLiteral("set"))) {
        const QVariant setValue = patch.value(QStringLiteral("set"));
        if (setValue.metaType().id() != QMetaType::QVariantMap) {
            *error = QStringLiteral("Scene patch set must be a map");
            return false;
        }
        const QVariantMap set = setValue.toMap();
        for (auto it = set.cbegin(); it != set.cend(); ++it) {
            if (!allowedKeys.contains(it.key()) || touched.contains(it.key())
                || !validValue(it.key(), it.value())) {
                *error = QStringLiteral("Invalid scene patch set field");
                return false;
            }
            touched.insert(it.key());
            target.insert(it.key(), it.value());
        }
    }
    if (patch.contains(QStringLiteral("clear"))) {
        const QVariant clearValue = patch.value(QStringLiteral("clear"));
        if (clearValue.metaType().id() != QMetaType::QVariantList) {
            *error = QStringLiteral("Scene patch clear must be a list");
            return false;
        }
        for (const QVariant &keyValue : clearValue.toList()) {
            if (keyValue.metaType().id() != QMetaType::QString) {
                *error = QStringLiteral("Scene patch clear key must be a string");
                return false;
            }
            const QString key = keyValue.toString();
            if (!allowedKeys.contains(key) || touched.contains(key)) {
                *error = QStringLiteral("Invalid scene patch clear field");
                return false;
            }
            touched.insert(key);
            target.remove(key);
        }
    }
    if (touched.isEmpty()) {
        *error = QStringLiteral("Empty scene map patch");
        return false;
    }
    if (changedKeys) {
        changedKeys->unite(touched);
    }
    return true;
}

bool shellPanelAtSide(const QVariantMap &scene, int side,
                      QVariantMap *panelOut)
{
    const QVariant shellValue = scene.value(QStringLiteral("shell"));
    if (shellValue.metaType().id() != QMetaType::QVariantMap) {
        return false;
    }
    const QVariant panelsValue = shellValue.toMap().value(
        QStringLiteral("panels"));
    if (panelsValue.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    int matches = 0;
    QVariantMap result;
    const QVariantList panels = panelsValue.toList();
    for (qsizetype index = 0; index < panels.size(); ++index) {
        if (panels.at(index).metaType().id() != QMetaType::QVariantMap) {
            continue;
        }
        const QVariantMap candidate = panels.at(index).toMap();
        bool declaredSideOK = false;
        const int declaredSide = candidate.value(QStringLiteral("side"))
                                     .toInt(&declaredSideOK);
        if ((declaredSideOK ? declaredSide : static_cast<int>(index))
            == side) {
            ++matches;
            result = candidate;
        }
    }
    if (matches != 1) {
        return false;
    }
    *panelOut = result;
    return true;
}

bool validPanelIdentity(const QVariantMap &operation,
                        const QVariantMap &panel, int side,
                        qulonglong *catalogRevisionOut, QString *error)
{
    bool sideOK = false;
    const int declaredSide = operation.value(QStringLiteral("side"))
                                 .toInt(&sideOK);
    qulonglong catalogRevision = 0;
    qulonglong currentCatalogRevision = 0;
    if (!sideOK || declaredSide != side || side < 0 || side > 1
        || operation.value(QStringLiteral("panelId")).metaType().id()
            != QMetaType::QString
        || operation.value(QStringLiteral("panelId"))
            != panel.value(QStringLiteral("id"))
        || !nonNegativeInteger(
            operation.value(QStringLiteral("catalogRevision")),
            &catalogRevision)
        || !nonNegativeInteger(
            panel.value(QStringLiteral("catalogRevision")),
            &currentCatalogRevision)
        || catalogRevision != currentCatalogRevision) {
        *error = QStringLiteral("Scene patch panel identity mismatch");
        return false;
    }
    if (catalogRevisionOut) {
        *catalogRevisionOut = catalogRevision;
    }
    return true;
}

bool validPanelState(const QVariantMap &state, const QVariantMap &current,
                     int side, QString *error)
{
    static const QSet<QString> stringKeys = {
        QStringLiteral("id"), QStringLiteral("kind"),
        QStringLiteral("path"), QStringLiteral("title"),
        QStringLiteral("galleryLayoutMode"),
        QStringLiteral("sourceKind"),
        QStringLiteral("cursorEntryId"),
        QStringLiteral("sortModeName"),
        QStringLiteral("fastFindText"),
        QStringLiteral("fastFindMatchColor"),
    };
    static const QSet<QString> boolKeys = {
        QStringLiteral("active"), QStringLiteral("previewCapable"),
        QStringLiteral("metadataDeferred"),
        QStringLiteral("catalogRowsDeferred"),
        QStringLiteral("sortReverse"),
        QStringLiteral("separateFileExtensions"),
        QStringLiteral("loading"),
        QStringLiteral("catalogProvisional"),
        QStringLiteral("fastFind"),
        QStringLiteral("showFileInfo"),
    };
    static const QSet<QString> nonNegativeIntegerKeys = {
        QStringLiteral("side"),
        QStringLiteral("galleryColumnCount"),
        QStringLiteral("galleryLayoutRevision"),
        QStringLiteral("catalogRevision"),
        QStringLiteral("metadataRevision"),
        QStringLiteral("selectedCount"),
        QStringLiteral("totalCount"),
    };
    for (auto it = state.cbegin(); it != state.cend(); ++it) {
        const QString &key = it.key();
        bool valid = false;
        if (stringKeys.contains(key)) {
            valid = it.value().metaType().id() == QMetaType::QString;
        } else if (boolKeys.contains(key)) {
            valid = it.value().metaType().id() == QMetaType::Bool;
        } else if (nonNegativeIntegerKeys.contains(key)) {
            valid = nonNegativeInteger(it.value());
        } else if (key == QStringLiteral("cursor")) {
            qlonglong cursor = -1;
            valid = integerValue(it.value(), &cursor) && cursor >= -1;
        } else if (key == QStringLiteral("galleryDensity")) {
            bool densityOK = false;
            const double density = it.value().toDouble(&densityOK);
            valid = densityOK && density >= 0.0;
        } else if (key == QStringLiteral("galleryDensities")) {
            const QVariantMap densities = it.value().toMap();
            static const QSet<QString> densityModes = {
                QStringLiteral("masonry"), QStringLiteral("columns"),
                QStringLiteral("details"), QStringLiteral("grid"),
                QStringLiteral("icons"),
            };
            valid = it.value().metaType().id() == QMetaType::QVariantMap
                && densities.size() <= densityModes.size();
            for (auto densityIt = densities.cbegin();
                 valid && densityIt != densities.cend(); ++densityIt) {
                bool densityOK = false;
                const double density = densityIt.value().toDouble(&densityOK);
                valid = densityModes.contains(densityIt.key())
                    && densityOK && density > 0.0 && density <= 500.0;
            }
        } else if (key == QStringLiteral("galleryColumns")) {
            valid = it.value().metaType().id() == QMetaType::QVariantList;
        } else if (key == QStringLiteral("fastFindMatches")) {
            valid = it.value().metaType().id() == QMetaType::QVariantMap;
        }
        if (!valid) {
            *error = QStringLiteral("Invalid or unbounded panel state field");
            return false;
        }
    }
    bool stateSideOK = false;
    const int stateSide = state.value(QStringLiteral("side")).toInt(
        &stateSideOK);
    qulonglong stateCatalogRevision = 0;
    qulonglong currentCatalogRevision = 0;
    qulonglong metadataRevision = 0;
    if (!stateSideOK || stateSide != side
        || state.value(QStringLiteral("id"))
            != current.value(QStringLiteral("id"))
        || state.value(QStringLiteral("kind")).toString()
            != QStringLiteral("filePanel")
        || !nonNegativeInteger(
            state.value(QStringLiteral("catalogRevision")),
            &stateCatalogRevision)
        || !nonNegativeInteger(
            current.value(QStringLiteral("catalogRevision")),
            &currentCatalogRevision)
        || stateCatalogRevision != currentCatalogRevision
        || state.value(QStringLiteral("metadataDeferred")) != true
        || !nonNegativeInteger(
            state.value(QStringLiteral("metadataRevision")),
            &metadataRevision)) {
        *error = QStringLiteral("Invalid row-free panel state patch");
        return false;
    }
    return true;
}

bool applyScenePatch(const QVariantMap &message,
                     const QVariantMap &currentScene,
                     const QVariantMap &currentPresentationScene,
                     qulonglong currentRevision,
                     AppliedScenePatch *result, QString *error)
{
    static const QSet<QString> envelopeKeys = {
        QStringLiteral("type"), QStringLiteral("schema"),
        QStringLiteral("version"), QStringLiteral("baseRevision"),
        QStringLiteral("revision"), QStringLiteral("root"),
        QStringLiteral("shell"), QStringLiteral("surface"),
    };
    static const QSet<QString> rootKeys = {
        QStringLiteral("width"), QStringLiteral("height"),
        QStringLiteral("activeScreen"), QStringLiteral("workspaceCount"),
        QStringLiteral("workspaceTabs"), QStringLiteral("presentation"),
        QStringLiteral("qmlIconSet"), QStringLiteral("menuBar"),
        QStringLiteral("keyBar"), QStringLiteral("toast"),
        QStringLiteral("dialogs"), QStringLiteral("menus"),
        QStringLiteral("surface"), QStringLiteral("shell"),
        QStringLiteral("operationsQueue"),
    };
    static const QSet<QString> shellKeys = {
        QStringLiteral("id"), QStringLiteral("kind"),
        QStringLiteral("title"), QStringLiteral("mode"),
        QStringLiteral("activePanel"), QStringLiteral("showPanels"),
        QStringLiteral("showLeftPanel"), QStringLiteral("showRightPanel"),
        QStringLiteral("wide"), QStringLiteral("widePanel"),
        QStringLiteral("showKeyBar"), QStringLiteral("terminalBusy"),
        QStringLiteral("terminalActive"), QStringLiteral("macroRecording"),
        QStringLiteral("fallback"), QStringLiteral("reason"),
        QStringLiteral("infoPanels"), QStringLiteral("quickViews"),
        QStringLiteral("commandLine"), QStringLiteral("terminal"),
    };
    for (auto it = message.cbegin(); it != message.cend(); ++it) {
        if (!envelopeKeys.contains(it.key())
            && !it.key().startsWith(QStringLiteral("benchmark"))) {
            *error = QStringLiteral("Unknown scene patch envelope field");
            return false;
        }
    }
    qulonglong version = 0;
    qulonglong baseRevision = 0;
    qulonglong revision = 0;
    if (message.value(QStringLiteral("type")).toString()
            != QStringLiteral("scene_patch")
        || message.value(QStringLiteral("schema")).toString()
            != QStringLiteral("app")
        || !nonNegativeInteger(message.value(QStringLiteral("version")),
                               &version)
        || version != 4
        || !nonNegativeInteger(
            message.value(QStringLiteral("baseRevision")), &baseRevision)
        || !nonNegativeInteger(message.value(QStringLiteral("revision")),
                               &revision)
        || baseRevision != currentRevision || revision != baseRevision + 1
        || currentScene.value(QStringLiteral("schema")).toString()
            != QStringLiteral("app")) {
        *error = QStringLiteral("Invalid or out-of-order scene patch");
        return false;
    }
    const bool hasRoot = message.contains(QStringLiteral("root"));
    const bool hasShell = message.contains(QStringLiteral("shell"));
    const bool hasSurface = message.contains(QStringLiteral("surface"));
    if (!hasRoot && !hasShell && !hasSurface) {
        *error = QStringLiteral("Empty scene patch");
        return false;
    }

    AppliedScenePatch next;
    next.scene = currentScene;
    next.presentationScene = currentPresentationScene;
    next.revision = revision;
    if (hasRoot) {
        if (!applyValidatedMapPatch(next.scene,
                                    message.value(QStringLiteral("root")),
                                    rootKeys, validRootPatchValue,
                                    &next.rootKeys, error)
            || !applyValidatedMapPatch(
                next.presentationScene,
                message.value(QStringLiteral("root")), rootKeys,
                validRootPatchValue, nullptr, error)) {
            return false;
        }
    }

    if (hasShell) {
        const QVariant shellPatchValue = message.value(
            QStringLiteral("shell"));
        if (shellPatchValue.metaType().id() != QMetaType::QVariantMap) {
            *error = QStringLiteral("Scene shell patch must be a map");
            return false;
        }
        const QVariantMap shellPatch = shellPatchValue.toMap();
        for (auto it = shellPatch.cbegin(); it != shellPatch.cend(); ++it) {
            if (it.key() != QStringLiteral("set")
                && it.key() != QStringLiteral("clear")
                && it.key() != QStringLiteral("panels")) {
                *error = QStringLiteral("Unknown scene shell-patch field");
                return false;
            }
        }

        QVariantMap shell = next.scene.value(QStringLiteral("shell")).toMap();
        QVariantMap presentationShell = next.presentationScene.value(
            QStringLiteral("shell")).toMap();
        if (shell.isEmpty() || presentationShell.isEmpty()) {
            *error = QStringLiteral("Scene shell patch has no base shell");
            return false;
        }
        QVariantMap mapPatch;
        if (shellPatch.contains(QStringLiteral("set"))) {
            mapPatch.insert(QStringLiteral("set"), shellPatch.value(
                QStringLiteral("set")));
        }
        if (shellPatch.contains(QStringLiteral("clear"))) {
            mapPatch.insert(QStringLiteral("clear"), shellPatch.value(
                QStringLiteral("clear")));
        }
        if (!mapPatch.isEmpty()) {
            if (!applyValidatedMapPatch(shell, mapPatch, shellKeys,
                                        validShellPatchValue,
                                        &next.shellKeys, error)
                || !applyValidatedMapPatch(
                    presentationShell, mapPatch, shellKeys,
                    validShellPatchValue, nullptr, error)) {
                return false;
            }
            next.scene.insert(QStringLiteral("shell"), shell);
            next.presentationScene.insert(QStringLiteral("shell"),
                                           presentationShell);
        }

        if (shellPatch.contains(QStringLiteral("panels"))) {
            const QVariant panelsValue = shellPatch.value(
                QStringLiteral("panels"));
            if (panelsValue.metaType().id() != QMetaType::QVariantList) {
                *error = QStringLiteral("Scene panel patches must be a list");
                return false;
            }
            for (const QVariant &operationValue : panelsValue.toList()) {
                if (operationValue.metaType().id()
                    != QMetaType::QVariantMap) {
                    *error = QStringLiteral("Scene panel patch must be a map");
                    return false;
                }
                const QVariantMap operation = operationValue.toMap();
                bool sideOK = false;
                const int side = operation.value(QStringLiteral("side"))
                                     .toInt(&sideOK);
                QVariantMap panel;
                QVariantMap presentationPanel;
                if (!sideOK || !shellPanelAtSide(next.scene, side, &panel)
                    || !shellPanelAtSide(next.presentationScene, side,
                                         &presentationPanel)) {
                    if (error->isEmpty()) {
                        *error = QStringLiteral("Invalid scene panel patch");
                    }
                    return false;
                }
                const QString op = operation.value(QStringLiteral("op"))
                                       .toString();
                QVariantMap signalOperation = operation;
                QVariantMap nextPanel = panel;
                QVariantMap nextPresentationPanel = presentationPanel;
                bool layoutOnlyStateUpdate = false;
                if (op == QStringLiteral("catalog_replace")) {
                    static const QSet<QString> allowed = {
                        QStringLiteral("op"), QStringLiteral("side"),
                        QStringLiteral("panelId"),
                        QStringLiteral("baseCatalogRevision"),
                        QStringLiteral("catalogRevision"),
                        QStringLiteral("panel"),
                    };
                    for (auto it = operation.cbegin();
                         it != operation.cend(); ++it) {
                        if (!allowed.contains(it.key())) {
                            *error = QStringLiteral(
                                "Unknown catalog-replacement field");
                            return false;
                        }
                    }
                    qulonglong baseCatalogRevision = 0;
                    qulonglong catalogRevision = 0;
                    qulonglong currentCatalogRevision = 0;
                    QVariant replacementValue = operation.value(
                        QStringLiteral("panel"));
                    if (operation.value(QStringLiteral("panelId"))
                            != panel.value(QStringLiteral("id"))
                        || !nonNegativeInteger(operation.value(
                            QStringLiteral("baseCatalogRevision")),
                            &baseCatalogRevision)
                        || !nonNegativeInteger(operation.value(
                            QStringLiteral("catalogRevision")),
                            &catalogRevision)
                        || !nonNegativeInteger(panel.value(
                            QStringLiteral("catalogRevision")),
                            &currentCatalogRevision)
                        || baseCatalogRevision != currentCatalogRevision
                        || catalogRevision <= baseCatalogRevision
                        || replacementValue.metaType().id()
                            != QMetaType::QVariantMap) {
                        *error = QStringLiteral(
                            "Invalid catalog replacement revision");
                        return false;
                    }
                    const QVariantMap replacement = replacementValue.toMap();
                    bool replacementRevisionOK = false;
                    const qulonglong replacementRevision = replacement.value(
                        QStringLiteral("catalogRevision"))
                                                            .toULongLong(
                                                                &replacementRevisionOK);
                    bool activePanelOK = false;
                    const int activePanel = next.scene.value(
                        QStringLiteral("shell")).toMap().value(
                            QStringLiteral("activePanel")).toInt(
                                &activePanelOK);
                    int validatedSide = -1;
                    QVariantMap validatedPanel;
                    const QVariantMap compatibilityEnvelope = {
                        {QStringLiteral("type"),
                         QStringLiteral("panel_catalog")},
                        {QStringLiteral("activePanel"), activePanel},
                        {QStringLiteral("side"), side},
                        {QStringLiteral("panel"), replacement},
                    };
                    if (!replacementRevisionOK
                        || replacementRevision != catalogRevision
                        || !activePanelOK
                        || !validPanelCatalogEnvelope(
                             compatibilityEnvelope, next.scene,
                             &validatedSide, &validatedPanel)
                        || validatedSide != side) {
                        *error = QStringLiteral(
                            "Invalid replacement panel catalog");
                        return false;
                    }
                    nextPanel = validatedPanel;
                    nextPresentationPanel = withoutNativePanelPayload(
                        validatedPanel);
                    if (!replaceShellPanel(next.scene, side, nextPanel)
                        || !replaceShellPanel(next.presentationScene, side,
                                              nextPresentationPanel)) {
                        *error = QStringLiteral(
                            "Could not commit replacement panel catalog");
                        return false;
                    }
                    next.catalogPanels.push_back(nextPanel);
                    next.compactPatches.push_back(QVariantMap{
                        {QStringLiteral("type"),
                         QStringLiteral("scene_patch")},
                        {QStringLiteral("side"), side},
                        {QStringLiteral("panel"), nextPresentationPanel},
                    });
                    continue;
                }
                if (op == QStringLiteral("catalog_append")) {
                    static const QSet<QString> allowed = {
                        QStringLiteral("op"), QStringLiteral("side"),
                        QStringLiteral("panelId"),
                        QStringLiteral("catalogRevision"),
                        QStringLiteral("offset"),
                        QStringLiteral("totalCount"),
                        QStringLiteral("final"), QStringLiteral("entries"),
                    };
                    for (auto it = operation.cbegin();
                         it != operation.cend(); ++it) {
                        if (!allowed.contains(it.key())) {
                            *error = QStringLiteral(
                                "Unknown catalog-append field");
                            return false;
                        }
                    }

                    qulonglong operationRevision = 0;
                    qulonglong currentRevision = 0;
                    qulonglong currentTotal = 0;
                    qulonglong offsetValue = 0;
                    qulonglong totalValue = 0;
                    const QVariant entriesValue = operation.value(
                        QStringLiteral("entries"));
                    const QVariant finalValue = operation.value(
                        QStringLiteral("final"));
                    const QVariant currentEntriesValue = panel.value(
                        QStringLiteral("entries"));
                    const QVariant currentProvisionalValue = panel.value(
                        QStringLiteral("catalogProvisional"));
                    const QVariant currentTotalValue = panel.value(
                        QStringLiteral("totalCount"));
                    if (operation.value(QStringLiteral("panelId"))
                            .metaType().id() != QMetaType::QString
                        || operation.value(QStringLiteral("panelId"))
                            != panel.value(QStringLiteral("id"))
                        || !nonNegativeInteger(operation.value(
                            QStringLiteral("catalogRevision")),
                            &operationRevision)
                        || !nonNegativeInteger(panel.value(
                            QStringLiteral("catalogRevision")),
                            &currentRevision)
                        || operationRevision != currentRevision
                        || currentProvisionalValue.metaType().id()
                            != QMetaType::Bool
                        || !currentProvisionalValue.toBool()
                        || currentEntriesValue.metaType().id()
                            != QMetaType::QVariantList
                        || !nonNegativeInteger(operation.value(
                            QStringLiteral("offset")), &offsetValue)
                        || !nonNegativeInteger(operation.value(
                            QStringLiteral("totalCount")), &totalValue)
                        || !nonNegativeInteger(currentTotalValue,
                                               &currentTotal)
                        || currentTotal != totalValue
                        || currentTotal <= offsetValue
                        || finalValue.metaType().id() != QMetaType::Bool
                        || entriesValue.metaType().id()
                            != QMetaType::QVariantList
                        || entriesValue.toList().isEmpty()
                        || offsetValue != static_cast<qulonglong>(
                               currentEntriesValue.toList().size())
                        || totalValue <= offsetValue
                        || totalValue < offsetValue
                            + static_cast<qulonglong>(
                                entriesValue.toList().size())
                        || finalValue.toBool() !=
                               (offsetValue + static_cast<qulonglong>(
                                   entriesValue.toList().size()) == totalValue)) {
                        *error = QStringLiteral(
                            "Invalid catalog append envelope");
                        return false;
                    }

                    const QVariantList currentEntries = currentEntriesValue.toList();
                    QSet<QString> entryIds;
                    entryIds.reserve(totalValue > static_cast<qulonglong>(
                                         std::numeric_limits<int>::max())
                                         ? currentEntries.size()
                                         : static_cast<int>(totalValue));
                    for (const QVariant &currentEntryValue : currentEntries) {
                        if (currentEntryValue.metaType().id()
                                != QMetaType::QVariantMap) {
                            *error = QStringLiteral(
                                "Invalid current catalog row");
                            return false;
                        }
                        const QString entryId = currentEntryValue.toMap().value(
                            QStringLiteral("entryId")).toString();
                        if (entryId.isEmpty() || entryIds.contains(entryId)) {
                            *error = QStringLiteral(
                                "Invalid current catalog identity");
                            return false;
                        }
                        entryIds.insert(entryId);
                    }

                    static const QSet<QString> entryKeys = {
                        QStringLiteral("index"),
                        QStringLiteral("entryId"),
                        QStringLiteral("name"),
                        QStringLiteral("displayBaseName"),
                        QStringLiteral("displayExtension"),
                        QStringLiteral("isDir"), QStringLiteral("isUp"),
                        QStringLiteral("isImage"),
                        QStringLiteral("isHidden"),
                        QStringLiteral("selected"),
                        QStringLiteral("highlightStyleId"),
                        QStringLiteral("source"),
                    };
                    static const QSet<QString> sourceKeys = {
                        QStringLiteral("resourceId"),
                        QStringLiteral("sourceKey"),
                        QStringLiteral("version"),
                        QStringLiteral("versionStrength"),
                        QStringLiteral("size"),
                        QStringLiteral("sizeKnown"),
                        QStringLiteral("accessProfile"),
                        QStringLiteral("storageClass"),
                    };
                    const QVariantList appendedEntries = entriesValue.toList();
                    for (qsizetype index = 0;
                         index < appendedEntries.size(); ++index) {
                        if (appendedEntries.at(index).metaType().id()
                                != QMetaType::QVariantMap) {
                            *error = QStringLiteral(
                                "Catalog append row must be a map");
                            return false;
                        }
                        const QVariantMap entry = appendedEntries.at(index).toMap();
                        for (auto it = entry.cbegin(); it != entry.cend(); ++it) {
                            if (!entryKeys.contains(it.key())) {
                                *error = QStringLiteral(
                                    "Catalog append row contains heavy or unknown data");
                                return false;
                            }
                        }
                        qlonglong row = -1;
                        const QString entryId = entry.value(
                            QStringLiteral("entryId")).toString();
                        if (!integerValue(entry.value(QStringLiteral("index")),
                                          &row)
                            || row != static_cast<qlonglong>(offsetValue)
                                + index
                            || entry.value(QStringLiteral("entryId"))
                                   .metaType().id() != QMetaType::QString
                            || entryId.isEmpty() || entryIds.contains(entryId)
                            || entry.value(QStringLiteral("name"))
                                   .metaType().id() != QMetaType::QString
                            || entry.value(QStringLiteral("isDir"))
                                   .metaType().id() != QMetaType::Bool
                            || entry.value(QStringLiteral("isUp"))
                                   .metaType().id() != QMetaType::Bool
                            || entry.value(QStringLiteral("isImage"))
                                   .metaType().id() != QMetaType::Bool
                            || entry.value(QStringLiteral("selected"))
                                   .metaType().id() != QMetaType::Bool
                            || (entry.contains(QStringLiteral("isHidden"))
                                && entry.value(QStringLiteral("isHidden"))
                                       .metaType().id() != QMetaType::Bool)
                            || (entry.contains(
                                    QStringLiteral("displayBaseName"))
                                && entry.value(QStringLiteral(
                                      "displayBaseName")).metaType().id()
                                    != QMetaType::QString)
                            || (entry.contains(
                                    QStringLiteral("displayExtension"))
                                && entry.value(QStringLiteral(
                                      "displayExtension")).metaType().id()
                                    != QMetaType::QString)
                            || (entry.contains(
                                    QStringLiteral("highlightStyleId"))
                                && (entry.value(QStringLiteral(
                                         "highlightStyleId")).metaType().id()
                                        != QMetaType::QString
                                    || entry.value(QStringLiteral(
                                         "highlightStyleId")).toString()
                                           .isEmpty()))) {
                            *error = QStringLiteral(
                                "Invalid catalog append row");
                            return false;
                        }
                        if (entry.contains(QStringLiteral("source"))) {
                            const QVariant sourceValue = entry.value(
                                QStringLiteral("source"));
                            if (sourceValue.metaType().id()
                                    != QMetaType::QVariantMap) {
                                *error = QStringLiteral(
                                    "Invalid catalog append source");
                                return false;
                            }
                            const QVariantMap source = sourceValue.toMap();
                            for (auto it = source.cbegin();
                                 it != source.cend(); ++it) {
                                if (!sourceKeys.contains(it.key())) {
                                    *error = QStringLiteral(
                                        "Unknown catalog append source field");
                                    return false;
                                }
                                if (it.key() == QStringLiteral("size")) {
                                    qlonglong size = 0;
                                    if (!integerValue(it.value(), &size)) {
                                        *error = QStringLiteral(
                                            "Invalid catalog append source size");
                                        return false;
                                    }
                                } else if (it.key() == QStringLiteral(
                                               "sizeKnown")) {
                                    if (it.value().metaType().id()
                                            != QMetaType::Bool) {
                                        *error = QStringLiteral(
                                            "Invalid catalog append source flag");
                                        return false;
                                    }
                                } else if (it.value().metaType().id()
                                           != QMetaType::QString) {
                                    *error = QStringLiteral(
                                        "Invalid catalog append source identity");
                                    return false;
                                }
                            }
                        }
                        entryIds.insert(entryId);
                    }

                    QVariantList combinedEntries = currentEntries;
                    combinedEntries += appendedEntries;
                    nextPanel.insert(QStringLiteral("entries"),
                                     combinedEntries);
                    nextPanel.insert(QStringLiteral("totalCount"),
                                     operation.value(QStringLiteral("totalCount")));
                    nextPanel.insert(QStringLiteral("catalogProvisional"),
                                     !finalValue.toBool());
                    nextPresentationPanel = withoutNativePanelPayload(nextPanel);
                    if (!replaceShellPanel(next.scene, side, nextPanel)
                        || !replaceShellPanel(next.presentationScene, side,
                                              nextPresentationPanel)) {
                        *error = QStringLiteral(
                            "Could not commit catalog append");
                        return false;
                    }
                    next.catalogAppends.push_back(signalOperation);
                    if (finalValue.toBool()) {
                        next.compactPatches.push_back(QVariantMap{
                            {QStringLiteral("type"),
                             QStringLiteral("scene_patch")},
                            {QStringLiteral("side"), side},
                            {QStringLiteral("panel"), nextPresentationPanel},
                        });
                    }
                    continue;
                }
                if (!validPanelIdentity(operation, panel, side, nullptr,
                                        error)) {
                    return false;
                }
                if (op == QStringLiteral("state_update")) {
                    static const QSet<QString> allowed = {
                        QStringLiteral("op"), QStringLiteral("side"),
                        QStringLiteral("panelId"),
                        QStringLiteral("catalogRevision"),
                        QStringLiteral("state"),
                    };
                    for (auto it = operation.cbegin();
                         it != operation.cend(); ++it) {
                        if (!allowed.contains(it.key())) {
                            *error = QStringLiteral(
                                "Unknown panel state-patch field");
                            return false;
                        }
                    }
                    const QVariant stateValue = operation.value(
                        QStringLiteral("state"));
                    if (stateValue.metaType().id()
                        != QMetaType::QVariantMap) {
                        *error = QStringLiteral("Panel state must be a map");
                        return false;
                    }
                    const QVariantMap state = stateValue.toMap();
                    if (!validPanelState(state, panel, side, error)) {
                        return false;
                    }
                    QVariantMap stateForCommit = state;
                    // Panel state is merged into a cached row-free map. The
                    // search match map is transient and has no delete
                    // operation of its own, so closing Fast Find must clear
                    // it explicitly at this boundary even for older Go
                    // producers that only send fastFind=false.
                    if (state.value(QStringLiteral("fastFind")).metaType().id()
                            == QMetaType::Bool
                        && !state.value(QStringLiteral("fastFind"))
                               .toBool()) {
                        stateForCommit.insert(QStringLiteral("fastFindMatches"),
                                              QVariantMap{});
                        stateForCommit.insert(QStringLiteral("fastFindMatchColor"),
                                              QString{});
                    }
                    static const QSet<QString> identityKeys = {
                        QStringLiteral("id"), QStringLiteral("kind"),
                        QStringLiteral("side"),
                        QStringLiteral("catalogRevision"),
                        QStringLiteral("metadataDeferred"),
                        QStringLiteral("metadataRevision"),
                    };
                    static const QSet<QString> layoutKeys = {
                        QStringLiteral("galleryLayoutMode"),
                        QStringLiteral("galleryColumnCount"),
                        QStringLiteral("galleryDensity"),
                        QStringLiteral("galleryDensities"),
                        QStringLiteral("galleryLayoutRevision"),
                        QStringLiteral("galleryColumns"),
                        QStringLiteral("separateFileExtensions"),
                    };
                    QSet<QString> semanticStateKeys(stateForCommit.keyBegin(),
                                                    stateForCommit.keyEnd());
                    semanticStateKeys.subtract(identityKeys);
                    QSet<QString> nonLayoutKeys = semanticStateKeys;
                    nonLayoutKeys.subtract(layoutKeys);
                    layoutOnlyStateUpdate = !semanticStateKeys.isEmpty()
                        && nonLayoutKeys.isEmpty();
                    for (auto it = stateForCommit.cbegin();
                         it != stateForCommit.cend(); ++it) {
                        nextPanel.insert(it.key(), it.value());
                        nextPresentationPanel.insert(it.key(), it.value());
                    }

                    // Keep the bridge's internal state-update signal in sync
                    // with the normalized panel that was committed above.
                    // QML receives the same normalized values via the compact
                    // presentation patch below.
                    signalOperation.insert(QStringLiteral("state"),
                                           stateForCommit);
                } else if (op == QStringLiteral("selection_delta")
                           || op == QStringLiteral("selection_replace")) {
                    static const QSet<QString> deltaKeys = {
                        QStringLiteral("op"), QStringLiteral("side"),
                        QStringLiteral("panelId"),
                        QStringLiteral("catalogRevision"),
                        QStringLiteral("baseSelectionRevision"),
                        QStringLiteral("selectionRevision"),
                        QStringLiteral("changes"),
                    };
                    static const QSet<QString> replaceKeys = {
                        QStringLiteral("op"), QStringLiteral("side"),
                        QStringLiteral("panelId"),
                        QStringLiteral("catalogRevision"),
                        QStringLiteral("baseSelectionRevision"),
                        QStringLiteral("selectionRevision"),
                        QStringLiteral("selectedEntryIds"),
                    };
                    const QSet<QString> &allowed =
                        op == QStringLiteral("selection_delta")
                            ? deltaKeys : replaceKeys;
                    for (auto it = operation.cbegin();
                         it != operation.cend(); ++it) {
                        if (!allowed.contains(it.key())) {
                            *error = QStringLiteral(
                                "Unknown selection-patch field");
                            return false;
                        }
                    }
                    qulonglong baseSelectionRevision = 0;
                    qulonglong selectionRevision = 0;
                    qulonglong currentSelectionRevision = 0;
                    if (!nonNegativeInteger(operation.value(
                            QStringLiteral("baseSelectionRevision")),
                            &baseSelectionRevision)
                        || !nonNegativeInteger(operation.value(
                            QStringLiteral("selectionRevision")),
                            &selectionRevision)
                        || !nonNegativeInteger(panel.value(
                            QStringLiteral("selectionRevision")),
                            &currentSelectionRevision)
                        || baseSelectionRevision != currentSelectionRevision
                        || selectionRevision <= baseSelectionRevision) {
                        *error = QStringLiteral(
                            "Out-of-order panel selection patch");
                        return false;
                    }
                    const QVariant entriesValue = panel.value(
                        QStringLiteral("entries"));
                    if (entriesValue.metaType().id()
                        != QMetaType::QVariantList) {
                        *error = QStringLiteral(
                            "Selection patch has no base catalog");
                        return false;
                    }
                    const QVariantList entries = entriesValue.toList();
                    if (op == QStringLiteral("selection_delta")) {
                        const QVariant changesValue = operation.value(
                            QStringLiteral("changes"));
                        if (changesValue.metaType().id()
                            != QMetaType::QVariantList
                            || changesValue.toList().isEmpty()) {
                            *error = QStringLiteral(
                                "Selection delta must contain changes");
                            return false;
                        }
                        QSet<int> changedIndexes;
                        QSet<QString> changedIds;
                        for (const QVariant &changeValue
                             : changesValue.toList()) {
                            if (changeValue.metaType().id()
                                != QMetaType::QVariantMap) {
                                *error = QStringLiteral(
                                    "Selection change must be a map");
                                return false;
                            }
                            const QVariantMap change = changeValue.toMap();
                            static const QSet<QString> changeKeys = {
                                QStringLiteral("index"),
                                QStringLiteral("entryId"),
                                QStringLiteral("selected"),
                            };
                            if (QSet<QString>(change.keyBegin(),
                                              change.keyEnd()) != changeKeys) {
                                *error = QStringLiteral(
                                    "Invalid selection change fields");
                                return false;
                            }
                            qlonglong index = -1;
                            const QString entryId = change.value(
                                QStringLiteral("entryId")).toString();
                            if (!integerValue(change.value(
                                    QStringLiteral("index")), &index)
                                || index < 0 || index >= entries.size()
                                || entryId.isEmpty()
                                || change.value(QStringLiteral("entryId"))
                                       .metaType().id() != QMetaType::QString
                                || change.value(QStringLiteral("selected"))
                                       .metaType().id() != QMetaType::Bool
                                || entries.at(index).toMap().value(
                                       QStringLiteral("entryId")) != entryId
                                || changedIndexes.contains(
                                       static_cast<int>(index))
                                || changedIds.contains(entryId)) {
                                *error = QStringLiteral(
                                    "Selection change does not match catalog");
                                return false;
                            }
                            changedIndexes.insert(static_cast<int>(index));
                            changedIds.insert(entryId);
                        }
                    } else {
                        const QVariant idsValue = operation.value(
                            QStringLiteral("selectedEntryIds"));
                        if (idsValue.metaType().id()
                            != QMetaType::QVariantList) {
                            *error = QStringLiteral(
                                "Selection replacement must contain IDs");
                            return false;
                        }
                        QSet<QString> requestedIds;
                        for (const QVariant &idValue : idsValue.toList()) {
                            if (idValue.metaType().id() != QMetaType::QString
                                || idValue.toString().isEmpty()
                                || requestedIds.contains(idValue.toString())) {
                                *error = QStringLiteral(
                                    "Invalid replacement selection ID");
                                return false;
                            }
                            requestedIds.insert(idValue.toString());
                        }
                        QSet<QString> unresolved = requestedIds;
                        for (const QVariant &entryValue : entries) {
                            unresolved.remove(entryValue.toMap().value(
                                QStringLiteral("entryId")).toString());
                            if (unresolved.isEmpty()) {
                                break;
                            }
                        }
                        if (!unresolved.isEmpty()) {
                            *error = QStringLiteral(
                                "Replacement selection ID is not in catalog");
                            return false;
                        }
                    }
                    nextPanel.insert(QStringLiteral("selectionRevision"),
                                     QVariant::fromValue(selectionRevision));
                    nextPresentationPanel.insert(
                        QStringLiteral("selectionRevision"),
                        QVariant::fromValue(selectionRevision));
                } else {
                    *error = QStringLiteral("Unknown panel patch operation");
                    return false;
                }
                if (!replaceShellPanel(next.scene, side, nextPanel)
                    || !replaceShellPanel(next.presentationScene, side,
                                          nextPresentationPanel)) {
                    *error = QStringLiteral("Could not commit panel patch");
                    return false;
                }
                QVariantMap signalPatch = signalOperation;
                signalPatch.insert(QStringLiteral("panel"),
                                   withoutNativePanelPayload(nextPanel));
                next.panelPatches.push_back(signalPatch);
                if (layoutOnlyStateUpdate) {
                    static const QSet<QString> compactLayoutKeys = {
                        QStringLiteral("id"), QStringLiteral("kind"),
                        QStringLiteral("side"),
                        QStringLiteral("catalogRevision"),
                        QStringLiteral("metadataDeferred"),
                        QStringLiteral("metadataRevision"),
                        QStringLiteral("galleryLayoutMode"),
                        QStringLiteral("galleryColumnCount"),
                        QStringLiteral("galleryDensity"),
                        QStringLiteral("galleryDensities"),
                        QStringLiteral("galleryLayoutRevision"),
                        QStringLiteral("galleryColumns"),
                        QStringLiteral("separateFileExtensions"),
                    };
                    const QVariantMap layoutDelta = signalOperation.value(
                        QStringLiteral("state")).toMap();
                    QVariantMap layoutState;
                    for (const QString &key : compactLayoutKeys) {
                        if (layoutDelta.contains(key)) {
                            layoutState.insert(key,
                                               layoutDelta.value(key));
                        }
                    }
                    next.compactPatches.push_back(QVariantMap{
                        {QStringLiteral("type"),
                         QStringLiteral("scene_patch")},
                        {QStringLiteral("side"), side},
                        {QStringLiteral("panelLayoutState"), layoutState},
                    });
                } else {
                    next.compactPatches.push_back(QVariantMap{
                        {QStringLiteral("type"),
                         QStringLiteral("scene_patch")},
                        {QStringLiteral("side"), side},
                        {QStringLiteral("panel"),
                         withoutNativePanelPayload(nextPanel)},
                    });
                }
            }
        }
    }
    if (hasSurface) {
        const QVariant surfacePatchValue = message.value(
            QStringLiteral("surface"));
        if (surfacePatchValue.metaType().id() != QMetaType::QVariantMap) {
            *error = QStringLiteral("Scene surface patch must be a map");
            return false;
        }
        const QVariantMap surfacePatch = surfacePatchValue.toMap();
        for (auto it = surfacePatch.cbegin();
             it != surfacePatch.cend(); ++it) {
            if (it.key() != QStringLiteral("id")
                && it.key() != QStringLiteral("set")) {
                *error = QStringLiteral("Unknown scene surface-patch field");
                return false;
            }
        }
        const QVariant surfaceId = surfacePatch.value(QStringLiteral("id"));
        QVariantMap surface = next.scene.value(
            QStringLiteral("surface")).toMap();
        QVariantMap presentationSurface = next.presentationScene.value(
            QStringLiteral("surface")).toMap();
        if (surfaceId.metaType().id() != QMetaType::QString
            || surfaceId.toString().isEmpty()
            || surface.value(QStringLiteral("id")) != surfaceId
            || presentationSurface.value(QStringLiteral("id")) != surfaceId
            || surface.value(QStringLiteral("kind")).toString()
                != QStringLiteral("editor")
            || presentationSurface.value(QStringLiteral("kind")).toString()
                != QStringLiteral("editor")) {
            *error = QStringLiteral("Scene surface patch identity mismatch");
            return false;
        }
        static const QSet<QString> surfaceStateKeys = {
            QStringLiteral("cursorLine"),
            QStringLiteral("cursorPos"),
            QStringLiteral("cursorVisualRow"),
            QStringLiteral("cursorVisualColumn"),
            QStringLiteral("cursorVisible"),
            QStringLiteral("cursorShape"),
            QStringLiteral("cursorAbsoluteRow"),
            QStringLiteral("topBarRight"),
        };
        const QVariantMap mapPatch = {
            {QStringLiteral("set"),
             surfacePatch.value(QStringLiteral("set"))},
        };
        if (!applyValidatedMapPatch(
                surface, mapPatch, surfaceStateKeys,
                validSurfaceStatePatchValue, &next.surfaceKeys, error)
            || !applyValidatedMapPatch(
                presentationSurface, mapPatch, surfaceStateKeys,
                validSurfaceStatePatchValue, nullptr, error)) {
            return false;
        }
        next.scene.insert(QStringLiteral("surface"), surface);
        next.presentationScene.insert(QStringLiteral("surface"),
                                      presentationSurface);
    }
    next.scene.insert(QStringLiteral("revision"),
                      QVariant::fromValue(revision));
    next.presentationScene.insert(QStringLiteral("revision"),
                                  QVariant::fromValue(revision));
    *result = std::move(next);
    return true;
}

}
