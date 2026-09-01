#include "ExtUiSceneReducer.h"

#include <QMetaType>

#include <limits>
#include <utility>

namespace ExtUiSceneReducer
{
bool integerValue(const QVariant &value, qlonglong *result = nullptr);

QVariantMap withoutNativePanelPayloads(QVariantMap container)
{
    const QVariant panelsValue = container.value(QStringLiteral("panels"));
    if (panelsValue.metaType().id() != QMetaType::QVariantList) {
        return container;
    }

    const QVariantList panels = panelsValue.toList();
    QVariantList presentationPanels;
    presentationPanels.reserve(panels.size());
    for (const QVariant &panelValue : panels) {
        if (panelValue.metaType().id() != QMetaType::QVariantMap) {
            presentationPanels.push_back(panelValue);
            continue;
        }
        QVariantMap panel = panelValue.toMap();
        // Gallery consumes these directly from the full C++ scene. QML only
        // needs the panel's chrome/layout fields; exposing the catalog here
        // recursively converts every row and style into QV4 objects on the GUI
        // thread for no benefit.
        panel.remove(QStringLiteral("entries"));
        panel.remove(QStringLiteral("highlightStyles"));
        presentationPanels.push_back(panel);
    }
    container.insert(QStringLiteral("panels"), presentationPanels);
    return container;
}

QVariant withoutNativePanelPayloadsFromFrames(const QVariant &framesValue)
{
    if (framesValue.metaType().id() != QMetaType::QVariantList) {
        return framesValue;
    }

    const QVariantList frames = framesValue.toList();
    QVariantList presentationFrames;
    presentationFrames.reserve(frames.size());
    for (const QVariant &frameValue : frames) {
        presentationFrames.push_back(
            frameValue.metaType().id() == QMetaType::QVariantMap
                ? QVariant(withoutNativePanelPayloads(frameValue.toMap()))
                : frameValue);
    }
    return presentationFrames;
}

QVariant withoutNativePanelPayloadsFromScreens(const QVariant &screensValue)
{
    if (screensValue.metaType().id() != QMetaType::QVariantList) {
        return screensValue;
    }

    const QVariantList screens = screensValue.toList();
    QVariantList presentationScreens;
    presentationScreens.reserve(screens.size());
    for (const QVariant &screenValue : screens) {
        if (screenValue.metaType().id() != QMetaType::QVariantMap) {
            presentationScreens.push_back(screenValue);
            continue;
        }

        QVariantMap screen = screenValue.toMap();
        const auto frames = screen.constFind(QStringLiteral("frames"));
        if (frames != screen.cend()) {
            screen.insert(QStringLiteral("frames"),
                          withoutNativePanelPayloadsFromFrames(*frames));
        }
        presentationScreens.push_back(screen);
    }
    return presentationScreens;
}

QVariantMap withoutNativePanelPayloadAliases(QVariantMap scene)
{
    scene = withoutNativePanelPayloads(std::move(scene));

    const QVariant shellValue = scene.value(QStringLiteral("shell"));
    if (shellValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("shell"),
                     withoutNativePanelPayloads(shellValue.toMap()));
    }

    const auto frames = scene.constFind(QStringLiteral("frames"));
    if (frames != scene.cend()) {
        scene.insert(QStringLiteral("frames"),
                     withoutNativePanelPayloadsFromFrames(*frames));
    }
    const auto screens = scene.constFind(QStringLiteral("screens"));
    if (screens != scene.cend()) {
        scene.insert(QStringLiteral("screens"),
                     withoutNativePanelPayloadsFromScreens(*screens));
    }
    return scene;
}

QVariant sanitizePresentationValue(const QVariant &value);

QVariantMap makePresentationScene(QVariantMap scene)
{
    scene = withoutNativePanelPayloadAliases(std::move(scene));

    const QVariant legacyValue = scene.value(QStringLiteral("legacy"));
    if (legacyValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("legacy"),
                     withoutNativePanelPayloadAliases(legacyValue.toMap()));
    }
    return sanitizePresentationValue(scene).toMap();
}

QVariant sanitizePresentationValue(const QVariant &value)
{
    if (value.metaType().id() == QMetaType::QVariantMap) {
        const QVariantMap source = value.toMap();
        QVariantMap sanitized;
        for (auto it = source.cbegin(); it != source.cend(); ++it) {
            if (it.key() == QStringLiteral("resourceId")
                || it.key() == QStringLiteral("leaseId")
                || it.key() == QStringLiteral("mediaEndpoint")
                || it.key() == QStringLiteral("mediaNonce")
                || it.key() == QStringLiteral("mediaProtocol")
                || it.key() == QStringLiteral("mediaMaxChunkSize")) {
                continue;
            }
            if (it.key() == QStringLiteral("source")
                && it.value().toMap().contains(QStringLiteral("resourceId"))) {
                continue;
            }
            sanitized.insert(it.key(), sanitizePresentationValue(it.value()));
        }
        return sanitized;
    }
    if (value.metaType().id() == QMetaType::QVariantList) {
        QVariantList sanitized;
        const QVariantList source = value.toList();
        sanitized.reserve(source.size());
        for (const QVariant &item : source) {
            sanitized.push_back(sanitizePresentationValue(item));
        }
        return sanitized;
    }
    return value;
}

QVariantMap makePresentationMessage(const QVariantMap &message,
                                    const QVariantMap &presentationScene)
{
    const QString type = message.value(QStringLiteral("type")).toString();
    if (type == QStringLiteral("scene")) {
        return presentationScene;
    }
    if (type == QStringLiteral("hello")) {
        QVariantMap presentation = sanitizePresentationValue(message).toMap();
        presentation.remove(QStringLiteral("nonce"));
        return presentation;
    }
    if (type == QStringLiteral("palette")) {
        return {
            {QStringLiteral("type"), type},
            {QStringLiteral("colors"), message.value(QStringLiteral("colors"))},
        };
    }
    if (type == QStringLiteral("frame")) {
        return {
            {QStringLiteral("type"), type},
            {QStringLiteral("width"), message.value(QStringLiteral("width"))},
            {QStringLiteral("height"), message.value(QStringLiteral("height"))},
            {QStringLiteral("full"), message.value(QStringLiteral("full"))},
            {QStringLiteral("cells"), message.value(QStringLiteral("cells"))},
        };
    }
    if (type == QStringLiteral("cursor")) {
        return {{QStringLiteral("type"), type},
                {QStringLiteral("x"), message.value(QStringLiteral("x"))},
                {QStringLiteral("y"), message.value(QStringLiteral("y"))},
                {QStringLiteral("visible"), message.value(QStringLiteral("visible"))},
                {QStringLiteral("shape"), message.value(QStringLiteral("shape"))}};
    }
    if (type == QStringLiteral("clipboard_set")) {
        return {{QStringLiteral("type"), type},
                {QStringLiteral("text"), message.value(QStringLiteral("text"))}};
    }
    if (type == QStringLiteral("quit")) {
        return {{QStringLiteral("type"), type}};
    }
    return sanitizePresentationValue(message).toMap();
}

QVariantMap withPanelActivation(QVariantMap container, int activeSide)
{
    if (container.contains(QStringLiteral("activePanel"))) {
        container.insert(QStringLiteral("activePanel"), activeSide);
    }
    const QVariant panelsValue = container.value(QStringLiteral("panels"));
    if (panelsValue.metaType().id() != QMetaType::QVariantList) {
        return container;
    }

    const QVariantList panels = panelsValue.toList();
    QVariantList updatedPanels;
    updatedPanels.reserve(panels.size());
    for (qsizetype index = 0; index < panels.size(); ++index) {
        const QVariant &panelValue = panels.at(index);
        if (panelValue.metaType().id() != QMetaType::QVariantMap) {
            updatedPanels.push_back(panelValue);
            continue;
        }
        QVariantMap panel = panelValue.toMap();
        bool sideOK = false;
        const int declaredSide = panel.value(QStringLiteral("side"))
                                     .toInt(&sideOK);
        const int side = sideOK ? declaredSide : static_cast<int>(index);
        panel.insert(QStringLiteral("active"), side == activeSide);
        updatedPanels.push_back(panel);
    }
    container.insert(QStringLiteral("panels"), updatedPanels);
    return container;
}

QVariant withPanelActivationInFrames(const QVariant &framesValue,
                                     int activeSide)
{
    if (framesValue.metaType().id() != QMetaType::QVariantList) {
        return framesValue;
    }
    const QVariantList frames = framesValue.toList();
    QVariantList updatedFrames;
    updatedFrames.reserve(frames.size());
    for (const QVariant &frameValue : frames) {
        updatedFrames.push_back(
            frameValue.metaType().id() == QMetaType::QVariantMap
                ? QVariant(withPanelActivation(frameValue.toMap(),
                                               activeSide))
                : frameValue);
    }
    return updatedFrames;
}

QVariant withPanelActivationInScreens(const QVariant &screensValue,
                                      int activeSide)
{
    if (screensValue.metaType().id() != QMetaType::QVariantList) {
        return screensValue;
    }
    const QVariantList screens = screensValue.toList();
    QVariantList updatedScreens;
    updatedScreens.reserve(screens.size());
    for (const QVariant &screenValue : screens) {
        if (screenValue.metaType().id() != QMetaType::QVariantMap) {
            updatedScreens.push_back(screenValue);
            continue;
        }
        QVariantMap screen = withPanelActivation(screenValue.toMap(),
                                                 activeSide);
        const auto frames = screen.constFind(QStringLiteral("frames"));
        if (frames != screen.cend()) {
            screen.insert(QStringLiteral("frames"),
                          withPanelActivationInFrames(*frames, activeSide));
        }
        updatedScreens.push_back(screen);
    }
    return updatedScreens;
}

QVariantMap withPanelActivationAliases(QVariantMap scene, int activeSide)
{
    scene = withPanelActivation(std::move(scene), activeSide);
    const QVariant shellValue = scene.value(QStringLiteral("shell"));
    if (shellValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("shell"),
                     withPanelActivation(shellValue.toMap(), activeSide));
    }
    const auto frames = scene.constFind(QStringLiteral("frames"));
    if (frames != scene.cend()) {
        scene.insert(QStringLiteral("frames"),
                     withPanelActivationInFrames(*frames, activeSide));
    }
    const auto screens = scene.constFind(QStringLiteral("screens"));
    if (screens != scene.cend()) {
        scene.insert(QStringLiteral("screens"),
                     withPanelActivationInScreens(*screens, activeSide));
    }
    return scene;
}

QVariantMap applyPanelActivationPatch(QVariantMap scene, int activeSide)
{
    scene = withPanelActivationAliases(std::move(scene), activeSide);
    const QVariant legacyValue = scene.value(QStringLiteral("legacy"));
    if (legacyValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("legacy"),
                     withPanelActivationAliases(legacyValue.toMap(),
                                                activeSide));
    }
    return scene;
}

QVariantMap withoutNativePanelPayload(QVariantMap panel)
{
    panel.remove(QStringLiteral("entries"));
    panel.remove(QStringLiteral("highlightStyles"));
    return panel;
}

QVariantMap compactPresentationPatch(
    const QVariantMap &message,
    const QVariantMap &presentationPanel)
{
    QVariantMap patch;
    for (const QString &key : {
             QStringLiteral("type"), QStringLiteral("activePanel"),
             QStringLiteral("side"), QStringLiteral("workspaceTabs")}) {
        if (message.contains(key)) {
            patch.insert(key, message.value(key));
        }
    }
    if (!presentationPanel.isEmpty()) {
        patch.insert(QStringLiteral("panel"), presentationPanel);
    }
    return patch;
}

bool replaceShellPanel(QVariantMap &scene, int side,
                       const QVariantMap &replacement)
{
    const QVariant shellValue = scene.value(QStringLiteral("shell"));
    if (shellValue.metaType().id() != QMetaType::QVariantMap) {
        return false;
    }
    QVariantMap shell = shellValue.toMap();
    const QVariant panelsValue = shell.value(QStringLiteral("panels"));
    if (panelsValue.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    QVariantList panels = panelsValue.toList();
    int match = -1;
    for (qsizetype index = 0; index < panels.size(); ++index) {
        if (panels.at(index).metaType().id() != QMetaType::QVariantMap) {
            continue;
        }
        const QVariantMap candidate = panels.at(index).toMap();
        bool declaredSideOK = false;
        const int declaredSide = candidate.value(QStringLiteral("side"))
                                     .toInt(&declaredSideOK);
        if ((declaredSideOK ? declaredSide : static_cast<int>(index))
            != side) {
            continue;
        }
        if (match != -1) {
            return false;
        }
        match = static_cast<int>(index);
    }
    if (match < 0) {
        return false;
    }
    panels[match] = replacement;
    shell.insert(QStringLiteral("panels"), panels);
    scene.insert(QStringLiteral("shell"), shell);
    return true;
}

bool validPanelCatalogEnvelope(const QVariantMap &message,
                               const QVariantMap &currentScene,
                               int *sideOut, QVariantMap *panelOut)
{
    static const QSet<QString> envelopeKeys = {
        QStringLiteral("type"), QStringLiteral("activePanel"),
        QStringLiteral("side"), QStringLiteral("panel"),
        QStringLiteral("commandLine"), QStringLiteral("shellTitle"),
        QStringLiteral("workspaceTabs"), QStringLiteral("menus"),
    };
    for (auto it = message.cbegin(); it != message.cend(); ++it) {
        if (!envelopeKeys.contains(it.key())
            && !it.key().startsWith(QStringLiteral("benchmark"))) {
            return false;
        }
    }

    bool sideOK = false;
    bool activePanelOK = false;
    const int side = message.value(QStringLiteral("side")).toInt(&sideOK);
    const int activePanel = message.value(QStringLiteral("activePanel"))
                                .toInt(&activePanelOK);
    const QVariant panelValue = message.value(QStringLiteral("panel"));
    if (!sideOK || side < 0 || side > 1 || !activePanelOK
        || activePanel < 0 || activePanel > 1
        || panelValue.metaType().id() != QMetaType::QVariantMap) {
        return false;
    }
    const QVariantMap panel = panelValue.toMap();
    bool panelSideOK = false;
    bool catalogRevisionOK = false;
    bool selectionRevisionOK = false;
    bool metadataRevisionOK = false;
    const int panelSide = panel.value(QStringLiteral("side"))
                              .toInt(&panelSideOK);
    panel.value(QStringLiteral("catalogRevision"))
        .toULongLong(&catalogRevisionOK);
    panel.value(QStringLiteral("selectionRevision"))
        .toULongLong(&selectionRevisionOK);
    panel.value(QStringLiteral("metadataRevision"))
        .toULongLong(&metadataRevisionOK);
    const QVariant entriesValue = panel.value(QStringLiteral("entries"));
    bool catalogProvisional = false;
    if (panel.contains(QStringLiteral("catalogProvisional"))) {
        const QVariant provisionalValue = panel.value(
            QStringLiteral("catalogProvisional"));
        if (provisionalValue.metaType().id() != QMetaType::Bool) {
            return false;
        }
        catalogProvisional = provisionalValue.toBool();
    }
    bool catalogRowsDeferred = false;
    if (panel.contains(QStringLiteral("catalogRowsDeferred"))) {
        const QVariant deferredValue = panel.value(
            QStringLiteral("catalogRowsDeferred"));
        if (deferredValue.metaType().id() != QMetaType::Bool) {
            return false;
        }
        catalogRowsDeferred = deferredValue.toBool();
    }
    qulonglong totalCount = 0;
    const bool hasTotalCount = panel.contains(QStringLiteral("totalCount"));
    if (hasTotalCount
        && !nonNegativeInteger(panel.value(QStringLiteral("totalCount")),
                               &totalCount)) {
        return false;
    }
    if (!panelSideOK || panelSide != side
        || panel.value(QStringLiteral("id")).toString().isEmpty()
        || panel.value(QStringLiteral("kind")).toString()
            != QStringLiteral("filePanel")
        || panel.value(QStringLiteral("active")).metaType().id()
            != QMetaType::Bool
        || panel.value(QStringLiteral("active")).toBool()
            != (side == activePanel)
        || panel.value(QStringLiteral("path")).metaType().id()
            != QMetaType::QString
        || !catalogRevisionOK || !selectionRevisionOK || !metadataRevisionOK
        || panel.value(QStringLiteral("metadataDeferred")).metaType().id()
            != QMetaType::Bool
        || !panel.value(QStringLiteral("metadataDeferred")).toBool()
        || entriesValue.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    const qsizetype entryCount = entriesValue.toList().size();
    if (hasTotalCount
        && (totalCount < static_cast<qulonglong>(entryCount)
            || (!catalogProvisional && !catalogRowsDeferred
                && totalCount != static_cast<qulonglong>(entryCount)))) {
        return false;
    }
    for (const QString &heavyKey : {
             QStringLiteral("highlightRevision"),
             QStringLiteral("selectedSize"),
             QStringLiteral("totalSize")}) {
        if (panel.contains(heavyKey)) {
            return false;
        }
    }
    const QVariant highlightStylesValue = panel.value(
        QStringLiteral("highlightStyles"));
    if (panel.contains(QStringLiteral("highlightStyles"))) {
        if (highlightStylesValue.metaType().id() != QMetaType::QVariantMap) {
            return false;
        }
        const QVariantMap highlightStyles = highlightStylesValue.toMap();
        for (auto it = highlightStyles.cbegin();
             it != highlightStyles.cend(); ++it) {
            if (it.key().isEmpty()
                || it.value().metaType().id() != QMetaType::QVariantMap) {
                return false;
            }
        }
    }

    QSet<QString> entryIds;
    const QVariantList entries = entriesValue.toList();
    int firstSourceIndex = -1;
    for (qsizetype row = 0; row < entries.size(); ++row) {
            if (entries.at(row).metaType().id() != QMetaType::QVariantMap) {
                return false;
            }
            const QVariantMap entry = entries.at(row).toMap();
            bool indexOK = false;
            const int index = entry.value(QStringLiteral("index"))
                                  .toInt(&indexOK);
            const QString entryId = entry.value(
                QStringLiteral("entryId")).toString();
            if (row == 0) {
                firstSourceIndex = index;
            }
            const int expectedIndex = catalogRowsDeferred
                ? firstSourceIndex + static_cast<int>(row)
                : static_cast<int>(row);
            if (!indexOK || index < 0
                || (hasTotalCount
                    && static_cast<qulonglong>(index) >= totalCount)
                || index != expectedIndex || entryId.isEmpty()
                || entryIds.contains(entryId)
                || entry.value(QStringLiteral("name")).metaType().id()
                    != QMetaType::QString
                || (entry.contains(QStringLiteral("path"))
                    && entry.value(QStringLiteral("path")).metaType().id()
                        != QMetaType::QString)
                || entry.value(QStringLiteral("isDir")).metaType().id()
                    != QMetaType::Bool
                || entry.value(QStringLiteral("isUp")).metaType().id()
                    != QMetaType::Bool
                || (entry.contains(QStringLiteral("isHidden"))
                    && entry.value(QStringLiteral("isHidden")).metaType().id()
                        != QMetaType::Bool)
                || entry.value(QStringLiteral("isImage")).metaType().id()
                    != QMetaType::Bool
                || entry.value(QStringLiteral("selected")).metaType().id()
                    != QMetaType::Bool
                || (entry.contains(QStringLiteral("highlightStyleId"))
                    && (entry.value(QStringLiteral("highlightStyleId"))
                            .metaType().id() != QMetaType::QString
                        || entry.value(QStringLiteral("highlightStyleId"))
                               .toString().isEmpty()))) {
                return false;
            }
            entryIds.insert(entryId);
            for (const QString &heavyKey : {
                     QStringLiteral("localPath"), QStringLiteral("size"),
                     QStringLiteral("sizeText"),
                     QStringLiteral("isExecutable"),
                     QStringLiteral("sizeCalculated"),
                     QStringLiteral("mtime"),
                     QStringLiteral("mtimeNanos"),
                     QStringLiteral("version"), QStringLiteral("mode")}) {
                if (entry.contains(heavyKey)) {
                    return false;
                }
            }
    }

    if ((panel.contains(QStringLiteral("fastFind"))
         && panel.value(QStringLiteral("fastFind")).metaType().id()
             != QMetaType::Bool)
        || (panel.contains(QStringLiteral("fastFindText"))
            && panel.value(QStringLiteral("fastFindText")).metaType().id()
                != QMetaType::QString)
        || (panel.contains(QStringLiteral("fastFindMatchColor"))
            && panel.value(QStringLiteral("fastFindMatchColor")).metaType().id()
                != QMetaType::QString)
        || (panel.contains(QStringLiteral("fastFindMatches"))
            && panel.value(QStringLiteral("fastFindMatches")).metaType().id()
                != QMetaType::QVariantMap)) {
        return false;
    }
    const QVariantMap fastFindMatches = panel.value(
        QStringLiteral("fastFindMatches")).toMap();
    for (auto it = fastFindMatches.cbegin();
         it != fastFindMatches.cend(); ++it) {
        if (!entryIds.contains(it.key())
            || it.value().metaType().id() != QMetaType::QVariantMap) {
            return false;
        }
        const QVariantMap match = it.value().toMap();
        bool startOK = false;
        bool lengthOK = false;
        const int start = match.value(QStringLiteral("start"))
                              .toInt(&startOK);
        const int length = match.value(QStringLiteral("length"))
                               .toInt(&lengthOK);
        if (match.size() != 2 || !startOK || start < 0
            || !lengthOK || length <= 0) {
            return false;
        }
    }

    const QVariantMap shell = currentScene.value(
        QStringLiteral("shell")).toMap();
    bool currentActiveOK = false;
    const int currentActive = shell.value(QStringLiteral("activePanel"))
                                  .toInt(&currentActiveOK);
    if (!currentActiveOK || currentActive != activePanel) {
        return false;
    }
    const QVariantList currentPanels = shell.value(
        QStringLiteral("panels")).toList();
    int currentMatches = 0;
    for (qsizetype index = 0; index < currentPanels.size(); ++index) {
        const QVariantMap current = currentPanels.at(index).toMap();
        bool declaredSideOK = false;
        const int declaredSide = current.value(QStringLiteral("side"))
                                     .toInt(&declaredSideOK);
        if ((declaredSideOK ? declaredSide : static_cast<int>(index))
            == side) {
            ++currentMatches;
            if (current.value(QStringLiteral("id"))
                    != panel.value(QStringLiteral("id"))) {
                return false;
            }
        }
    }
    if (currentMatches != 1
        || (message.contains(QStringLiteral("commandLine"))
            && message.value(QStringLiteral("commandLine")).metaType().id()
                != QMetaType::QVariantMap)
        || (message.contains(QStringLiteral("shellTitle"))
            && message.value(QStringLiteral("shellTitle")).metaType().id()
                != QMetaType::QString)
        || (message.contains(QStringLiteral("workspaceTabs"))
            && message.value(QStringLiteral("workspaceTabs")).metaType().id()
                != QMetaType::QVariantMap)
        || (message.contains(QStringLiteral("menus"))
            && message.value(QStringLiteral("menus")).metaType().id()
                != QMetaType::QVariantList)) {
        return false;
    }

    *sideOut = side;
    *panelOut = panel;
    return true;
}

bool validPanelChromeEnvelope(const QVariantMap &message,
                              int *activePanelOut)
{
    static const QSet<QString> envelopeKeys = {
        QStringLiteral("type"), QStringLiteral("activePanel"),
        QStringLiteral("commandLine"), QStringLiteral("shellTitle"),
        QStringLiteral("workspaceTabs"), QStringLiteral("menus"),
    };
    for (auto it = message.cbegin(); it != message.cend(); ++it) {
        if (!envelopeKeys.contains(it.key())
            && !it.key().startsWith(QStringLiteral("benchmark"))) {
            return false;
        }
    }

    const QVariant typeValue = message.value(QStringLiteral("type"));
    const QVariant activePanelValue = message.value(
        QStringLiteral("activePanel"));
    const int activePanelType = activePanelValue.metaType().id();
    const bool activePanelIsInteger =
        activePanelType == QMetaType::Char
        || activePanelType == QMetaType::SChar
        || activePanelType == QMetaType::UChar
        || activePanelType == QMetaType::Short
        || activePanelType == QMetaType::UShort
        || activePanelType == QMetaType::Int
        || activePanelType == QMetaType::UInt
        || activePanelType == QMetaType::LongLong
        || activePanelType == QMetaType::ULongLong;
    bool activePanelOK = false;
    const int activePanel = activePanelValue.toInt(&activePanelOK);
    if (typeValue.metaType().id() != QMetaType::QString
        || typeValue.toString() != QStringLiteral("panel_chrome")
        || !activePanelIsInteger || !activePanelOK
        || activePanel < 0 || activePanel > 1
        || (message.contains(QStringLiteral("commandLine"))
            && message.value(QStringLiteral("commandLine")).metaType().id()
                != QMetaType::QVariantMap)
        || (message.contains(QStringLiteral("shellTitle"))
            && message.value(QStringLiteral("shellTitle")).metaType().id()
                != QMetaType::QString)
        || (message.contains(QStringLiteral("workspaceTabs"))
            && message.value(QStringLiteral("workspaceTabs")).metaType().id()
                != QMetaType::QVariantMap)
        || (message.contains(QStringLiteral("menus"))
            && message.value(QStringLiteral("menus")).metaType().id()
                != QMetaType::QVariantList)) {
        return false;
    }

    *activePanelOut = activePanel;
    return true;
}

void applyPanelCatalogCompactFields(QVariantMap &scene,
                                    const QVariantMap &message,
                                    int activePanel)
{
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    shell.insert(QStringLiteral("activePanel"), activePanel);
    if (message.contains(QStringLiteral("commandLine"))) {
        shell.insert(QStringLiteral("commandLine"),
                     message.value(QStringLiteral("commandLine")));
    }
    if (message.contains(QStringLiteral("shellTitle"))) {
        shell.insert(QStringLiteral("title"),
                     message.value(QStringLiteral("shellTitle")));
    }
    scene.insert(QStringLiteral("shell"), shell);
    for (const QString &key : {QStringLiteral("workspaceTabs"),
                               QStringLiteral("menus")}) {
        if (message.contains(key)) {
            scene.insert(key, message.value(key));
        }
    }
    for (auto it = message.cbegin(); it != message.cend(); ++it) {
        if (it.key().startsWith(QStringLiteral("benchmark"))) {
            scene.insert(it.key(), it.value());
        }
    }
}

bool hasNonEmptyMap(const QVariantMap &container, const QString &key)
{
    const QVariant value = container.value(key);
    return value.metaType().id() == QMetaType::QVariantMap
        && !value.toMap().isEmpty();
}

}
