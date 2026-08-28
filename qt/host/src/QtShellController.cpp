#include "QtShellController.h"

#include "NavigationBenchmarkTrace.h"

#include <QCoreApplication>
#include <QDebug>
#include <QElapsedTimer>
#include <QMetaObject>
#include <QPointer>
#include <QSet>
#include <QTimer>

#include <msgpack.hpp>

#include <array>
#include <cstdint>
#include <exception>
#include <utility>

namespace
{
constexpr quint32 MaxMessageSize = 64 * 1024 * 1024;
// Keep decode-ahead bounded by both bytes and frame count. The byte budget is
// no larger than the socket buffer that the old single-frame pipeline left
// queued in QTcpSocket, while the count cap prevents an unlimited burst of
// tiny protocol updates from filling the decoder and GUI event queues.
constexpr qsizetype MaxQueuedDecodeBytes = MaxMessageSize;
constexpr qsizetype MaxQueuedDecodeFrames = 8;
constexpr int InitialConnectDeadlineMs = 2000;
constexpr int ExtUiProtocolVersion = 3;

void packString(msgpack::packer<msgpack::sbuffer> &packer, const QString &value)
{
    const QByteArray bytes = value.toUtf8();
    packer.pack_str(static_cast<uint32_t>(bytes.size()));
    packer.pack_str_body(bytes.constData(), static_cast<uint32_t>(bytes.size()));
}

void packVariant(msgpack::packer<msgpack::sbuffer> &packer, const QVariant &value)
{
    if (!value.isValid() || value.isNull()) {
        packer.pack_nil();
        return;
    }

    switch (value.typeId()) {
    case QMetaType::Bool:
        packer.pack(value.toBool());
        return;
    case QMetaType::Int:
    case QMetaType::Short:
    case QMetaType::SChar:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::UInt:
    case QMetaType::UShort:
    case QMetaType::UChar:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::LongLong:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::ULongLong:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::Double:
    case QMetaType::Float:
        packer.pack_double(value.toDouble());
        return;
    case QMetaType::QString:
        packString(packer, value.toString());
        return;
    case QMetaType::QVariantList: {
        const QVariantList list = value.toList();
        packer.pack_array(static_cast<uint32_t>(list.size()));
        for (const QVariant &item : list) {
            packVariant(packer, item);
        }
        return;
    }
    case QMetaType::QVariantMap: {
        const QVariantMap map = value.toMap();
        packer.pack_map(static_cast<uint32_t>(map.size()));
        for (auto it = map.cbegin(); it != map.cend(); ++it) {
            packString(packer, it.key());
            packVariant(packer, it.value());
        }
        return;
    }
    default:
        if (value.canConvert<QString>()) {
            packString(packer, value.toString());
        } else {
            packer.pack_nil();
        }
        return;
    }
}

QVariant unpackObject(const msgpack::object &object)
{
    switch (object.type) {
    case msgpack::type::NIL:
        return QVariant();
    case msgpack::type::BOOLEAN:
        return QVariant(object.via.boolean);
    case msgpack::type::POSITIVE_INTEGER:
        return QVariant::fromValue<qulonglong>(object.via.u64);
    case msgpack::type::NEGATIVE_INTEGER:
        return QVariant::fromValue<qlonglong>(object.via.i64);
    case msgpack::type::FLOAT32:
    case msgpack::type::FLOAT64:
        return QVariant(object.via.f64);
    case msgpack::type::STR:
        return QString::fromUtf8(object.via.str.ptr, static_cast<qsizetype>(object.via.str.size));
    case msgpack::type::BIN:
        return QByteArray(object.via.bin.ptr, static_cast<qsizetype>(object.via.bin.size));
    case msgpack::type::ARRAY: {
        QVariantList list;
        list.reserve(static_cast<qsizetype>(object.via.array.size));
        for (uint32_t i = 0; i < object.via.array.size; ++i) {
            list.push_back(unpackObject(object.via.array.ptr[i]));
        }
        return list;
    }
    case msgpack::type::MAP: {
        QVariantMap map;
        for (uint32_t i = 0; i < object.via.map.size; ++i) {
            const QString key = unpackObject(object.via.map.ptr[i].key).toString();
            map.insert(key, unpackObject(object.via.map.ptr[i].val));
        }
        return map;
    }
    default:
        return QVariant();
    }
}

quint32 readBigEndianSize(const QByteArray &buffer)
{
    return (static_cast<quint32>(static_cast<unsigned char>(buffer[0])) << 24)
        | (static_cast<quint32>(static_cast<unsigned char>(buffer[1])) << 16)
        | (static_cast<quint32>(static_cast<unsigned char>(buffer[2])) << 8)
        | static_cast<quint32>(static_cast<unsigned char>(buffer[3]));
}

void writeBigEndianSize(QByteArray &buffer, quint32 size)
{
    buffer.resize(4);
    buffer[0] = static_cast<char>((size >> 24) & 0xff);
    buffer[1] = static_cast<char>((size >> 16) & 0xff);
    buffer[2] = static_cast<char>((size >> 8) & 0xff);
    buffer[3] = static_cast<char>(size & 0xff);
}

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

QVariantMap makePresentationScene(QVariantMap scene)
{
    scene = withoutNativePanelPayloadAliases(std::move(scene));

    const QVariant legacyValue = scene.value(QStringLiteral("legacy"));
    if (legacyValue.metaType().id() == QMetaType::QVariantMap) {
        scene.insert(QStringLiteral("legacy"),
                     withoutNativePanelPayloadAliases(legacyValue.toMap()));
    }
    return scene;
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
    const QVariantMap &presentationPanel = QVariantMap())
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
        if (!indexOK || index != row || entryId.isEmpty()
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
                 QStringLiteral("sizeText"), QStringLiteral("isHidden"),
                 QStringLiteral("isExecutable"), QStringLiteral("isCached"),
                 QStringLiteral("sizeCalculated"), QStringLiteral("mtime"),
                 QStringLiteral("mtimeNanos"), QStringLiteral("version"),
                 QStringLiteral("mode")}) {
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

bool nonNegativeInteger(const QVariant &value, qulonglong *result = nullptr)
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

bool integerValue(const QVariant &value, qlonglong *result = nullptr)
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

struct AppliedScenePatch
{
    QVariantMap scene;
    QVariantMap presentationScene;
    QList<QVariantMap> catalogPanels;
    QList<QVariantMap> panelPatches;
    QList<QVariantMap> compactPatches;
    QSet<QString> rootKeys;
    QSet<QString> shellKeys;
    QSet<QString> surfaceKeys;
    qulonglong revision = 0;
};

bool applyScenePatch(const QVariantMap &message,
                     const QVariantMap &currentScene,
                     const QVariantMap &currentPresentationScene,
                     qulonglong currentRevision, AppliedScenePatch *result,
                     QString *error)
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
                    const QVariant replacementValue = operation.value(
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
}

class QtShellMessageDecoder final : public QObject
{
    Q_OBJECT

public:
    explicit QtShellMessageDecoder(QObject *parent = nullptr)
        : QObject(parent)
    {
    }

    void decode(QByteArray payload, quint64 epoch, quint64 sequence)
    {
        const bool traceEnabled = F4NavigationBenchmarkTrace::enabled();
        QElapsedTimer decodeTimer;
        qint64 decodeStartedNs = 0;
        if (traceEnabled) {
            decodeStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            decodeTimer.start();
        }
        try {
            msgpack::object_handle handle = msgpack::unpack(
                payload.constData(), static_cast<size_t>(payload.size()));
            QVariant decodedValue = unpackObject(handle.get());
            qint64 decodeDurationNs = 0;
            qint64 decodeCompletedNs = 0;
            QVariant traceId;
            if (traceEnabled) {
                decodeDurationNs = decodeTimer.nsecsElapsed();
                decodeCompletedNs =
                    F4NavigationBenchmarkTrace::monotonicNanoseconds();
                const QVariantMap message = decodedValue.toMap();
                traceId = F4NavigationBenchmarkTrace::benchmarkTraceId(message);
            }
            emit decoded(epoch, sequence, decodedValue);
            if (traceEnabled) {
                const QVariantMap message = decodedValue.toMap();
                const QVariantMap fields = {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), payload.size()},
                    {QStringLiteral("messageType"),
                     message.value(QStringLiteral("type")).toString()},
                };
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.start"), decodeStartedNs,
                    traceId, fields);
                QVariantMap completedFields = fields;
                completedFields.insert(QStringLiteral("durationNs"),
                                       decodeDurationNs);
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.end"), decodeCompletedNs,
                    traceId, completedFields);
            }
        } catch (const std::exception &e) {
            const qint64 decodeDurationNs = traceEnabled
                ? decodeTimer.nsecsElapsed() : 0;
            const qint64 decodeCompletedNs = traceEnabled
                ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
            emit failed(epoch, sequence, QString::fromUtf8(e.what()));
            if (traceEnabled) {
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.start"), decodeStartedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                    });
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.failed"), decodeCompletedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                        {QStringLiteral("durationNs"),
                         decodeDurationNs},
                        {QStringLiteral("error"),
                         QString::fromUtf8(e.what())},
                    });
            }
        } catch (...) {
            const qint64 decodeDurationNs = traceEnabled
                ? decodeTimer.nsecsElapsed() : 0;
            const qint64 decodeCompletedNs = traceEnabled
                ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
            emit failed(epoch, sequence,
                        QStringLiteral("unknown MessagePack error"));
            if (traceEnabled) {
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.start"), decodeStartedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                    });
                F4NavigationBenchmarkTrace::eventAt(
                    QStringLiteral("qt.decode.failed"), decodeCompletedNs, {}, {
                        {QStringLiteral("sequence"), sequence},
                        {QStringLiteral("payloadBytes"), payload.size()},
                        {QStringLiteral("durationNs"),
                         decodeDurationNs},
                        {QStringLiteral("error"),
                         QStringLiteral("unknown MessagePack error")},
                    });
            }
        }
    }

signals:
    void decoded(quint64 epoch, quint64 sequence, const QVariant &value);
    void failed(quint64 epoch, quint64 sequence, const QString &message);
};

QtShellController::QtShellController(const QString &connectAddress,
                                     const QString &nonce,
                                     int cols,
                                     int rows,
                                     QObject *parent)
    : QObject(parent)
    , m_socket(new QTcpSocket(this))
    , m_nonce(nonce)
    , m_initialCols(cols)
    , m_initialRows(rows)
{
    if (!parseConnectAddress(connectAddress)) {
        m_startupError = QStringLiteral(
            "Invalid ExtUI connect address: %1").arg(connectAddress);
        // main.cpp connects fatalError immediately after construction. Queue
        // malformed-address reporting so that startup failures cannot be lost
        // before that observer (and the application event loop) exist.
        QTimer::singleShot(0, this, [this]() {
            emit fatalError(m_startupError);
        });
        return;
    }

    m_decodeThread.setObjectName(QStringLiteral("f4-msgpack-decoder"));
    m_decoder = new QtShellMessageDecoder;
    m_decoder->moveToThread(&m_decodeThread);
    connect(&m_decodeThread, &QThread::finished,
            m_decoder, &QObject::deleteLater);
    connect(m_decoder, &QtShellMessageDecoder::decoded,
            this, &QtShellController::onFrameDecoded,
            Qt::QueuedConnection);
    connect(m_decoder, &QtShellMessageDecoder::failed,
            this, &QtShellController::onFrameDecodeFailed,
            Qt::QueuedConnection);
    m_decodeThread.start();

    connect(m_socket, &QTcpSocket::connected, this, &QtShellController::onConnected);
    connect(m_socket, &QTcpSocket::readyRead, this, &QtShellController::onReadyRead);
    connect(m_socket, &QTcpSocket::disconnected, this, &QtShellController::onDisconnected);
    connect(m_socket, &QTcpSocket::errorOccurred, this, &QtShellController::onSocketError);

    // Keep one further maximum-sized frame in Qt's socket buffer. The decoded
    // work submitted below has its own one-frame byte budget, so TCP
    // backpressure still bounds memory and stale-scene accumulation.
    m_socket->setReadBufferSize(static_cast<qint64>(MaxMessageSize) + 4);
    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.connect.begin"), {}, {
                {QStringLiteral("host"), m_host},
                {QStringLiteral("port"), m_port},
            });
    }
    m_socket->connectToHost(m_host, m_port);
    QTimer::singleShot(InitialConnectDeadlineMs, this, [this]() {
        if (m_connected || m_helloSent || !m_startupError.isEmpty()) {
            return;
        }
        m_startupError = QStringLiteral(
            "Timed out connecting to the f4 core at %1:%2")
            .arg(m_host).arg(m_port);
        m_socket->abort();
        emit fatalError(m_startupError);
    });
}

QtShellController::~QtShellController()
{
    invalidateDecodeSession();
    if (m_decodeThread.isRunning()) {
        // A decode already executing cannot be interrupted safely because
        // msgpack owns its temporary zone. Let that one finish, discard its
        // epoch-tagged result, and prevent the worker event loop from taking
        // any more work.
        m_decodeThread.quit();
        m_decodeThread.wait();
    }
    m_decoder = nullptr;
}

void QtShellController::updateCommandMenus(const QVariantList &menus,
                                           bool allowStateOnlyUpdate)
{
    const QVariantList states = projectCommandMenuStates(menus);
    if (allowStateOnlyUpdate && commandMenuStructuresEqual(m_commandMenus,
                                                            menus)) {
        if (states != m_commandMenuStates) {
            m_commandMenuStates = states;
            emit commandMenuStatesChanged(m_commandMenuStates);
        }
        return;
    }

    const bool structureChanged = menus != m_commandMenus;
    const bool stateChanged = states != m_commandMenuStates;
    m_commandMenus = menus;
    m_commandMenuStates = states;
    if (structureChanged) {
        emit commandMenusChanged();
    }
    if (stateChanged) {
        emit commandMenuStatesChanged(m_commandMenuStates);
    }
}

bool QtShellController::initialSceneReadyForDisplay(const QVariantMap &scene)
{
    if (scene.isEmpty()) {
        return false;
    }
    if (hasNonEmptyMap(scene, QStringLiteral("surface"))
        || hasNonEmptyMap(scene, QStringLiteral("operationsQueue"))) {
        return true;
    }

    const QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    if (!shell.isEmpty()) {
        // Text presentation and the terminal surface do not depend on a
        // native catalog becoming ready.
        if (scene.value(QStringLiteral("presentation")).toString()
                == QStringLiteral("text")
            || shell.value(QStringLiteral("terminalActive")).toBool()) {
            return true;
        }

        const QVariantList panels = shell.value(
            QStringLiteral("panels")).toList();
        if (panels.isEmpty()) {
            return false;
        }

        const bool wide = shell.value(QStringLiteral("wide")).toBool();
        const int wideSide = shell.value(QStringLiteral("widePanel")).toInt();
        for (int side = 0; side < 2; ++side) {
            const QString visibilityKey = side == 0
                ? QStringLiteral("showLeftPanel")
                : QStringLiteral("showRightPanel");
            const bool sideVisible = wide
                ? side == wideSide
                : (!shell.contains(visibilityKey)
                   || shell.value(visibilityKey).toBool());
            if (!sideVisible || shellSideIsCovered(shell, side)) {
                continue;
            }

            bool found = false;
            for (const QVariant &panelValue : panels) {
                const QVariantMap panel = panelValue.toMap();
                if (panel.isEmpty()
                    || panel.value(QStringLiteral("side")).toInt() != side) {
                    continue;
                }
                found = true;
                if (panel.value(QStringLiteral("loading")).toBool()
                    && !isAuthoritativePhasedCatalog(panel)) {
                    return false;
                }
                break;
            }
            if (!found) {
                return false;
            }
        }
        return true;
    }

    // Compatibility scenes have no app shell but can still fully populate
    // the fallback grid before the native window is exposed.
    return !scene.value(QStringLiteral("frames")).toList().isEmpty()
        || !scene.value(QStringLiteral("screens")).toList().isEmpty();
}

bool QtShellController::waitForInitialHandshake(int timeoutMs)
{
    if (!m_socket || m_host.isEmpty() || m_port == 0) {
        return false;
    }

    QElapsedTimer timer;
    if (F4NavigationBenchmarkTrace::enabled()) {
        timer.start();
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.handshake.wait.begin"));
    }

    bool connected = m_socket->state() == QAbstractSocket::ConnectedState;
    if (!connected && timeoutMs > 0
        && (m_socket->state() == QAbstractSocket::HostLookupState
            || m_socket->state() == QAbstractSocket::ConnectingState)) {
        connected = m_socket->waitForConnected(timeoutMs);
    }
    if (connected && !m_helloSent) {
        // QAbstractSocket normally emits connected() from waitForConnected(),
        // invoking onConnected() directly on this thread. Keep this explicit
        // call as an idempotent guarantee across socket-engine backends.
        onConnected();
    }

    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.handshake.wait.end"), {}, {
                {QStringLiteral("connected"), connected},
                {QStringLiteral("helloSent"), m_helloSent},
                {QStringLiteral("durationNs"), timer.nsecsElapsed()},
            });
    }
    return connected && m_helloSent;
}

bool QtShellController::completeInitialHandshake()
{
    if (waitForInitialHandshake(InitialConnectDeadlineMs)) {
        return true;
    }
    if (!m_startupError.isEmpty()) {
        return false;
    }

    QString message;
    if (m_socket
        && m_socket->error() != QAbstractSocket::UnknownSocketError) {
        message = m_socket->errorString();
    }
    if (message.isEmpty()) {
        message = QStringLiteral(
            "Timed out connecting to the f4 core at %1:%2")
                      .arg(m_host).arg(m_port);
    }
    m_startupError = message;
    if (m_socket) {
        m_socket->abort();
    }
    emit fatalError(message);
    return false;
}

void QtShellController::sendResize(int cols, int rows)
{
    if (cols <= 0 || rows <= 0) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("resize")},
        {QStringLiteral("cols"), cols},
        {QStringLiteral("rows"), rows},
    });
}

void QtShellController::sendKey(int vk, int ch, bool down, int mods)
{
    sendKeyEvent(vk, ch, down, mods, false);
}

void QtShellController::sendKeyEvent(int vk, int ch, bool down, int mods, bool repeat)
{
    QVariantMap message{
        {QStringLiteral("type"), QStringLiteral("key")},
        {QStringLiteral("vk"), vk},
        {QStringLiteral("char"), ch},
        {QStringLiteral("down"), down},
        {QStringLiteral("mods"), mods},
        {QStringLiteral("repeat"), repeat},
    };
    if (F4NavigationBenchmarkTrace::enabled()) {
        const quint64 keySequence = m_nextKeySequence++;
        message.insert(QStringLiteral("keySequence"), keySequence);
        message.insert(
            QStringLiteral("benchmarkTraceId"),
            QStringLiteral("qt:key:%1:%2")
                .arg(QCoreApplication::applicationPid())
                .arg(keySequence));
    }
    sendMessage(message);
}

void QtShellController::sendText(const QString &text, int mods)
{
    if (text.isEmpty()) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("text")},
        {QStringLiteral("text"), text},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendMouse(int x, int y, int button, int flags, bool down, int mods)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("mouse")},
        {QStringLiteral("x"), x},
        {QStringLiteral("y"), y},
        {QStringLiteral("button"), button},
        {QStringLiteral("flags"), flags},
        {QStringLiteral("down"), down},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendWheel(int x, int y, int dir, int mods)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("wheel")},
        {QStringLiteral("x"), x},
        {QStringLiteral("y"), y},
        {QStringLiteral("dir"), dir},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendPaste(const QString &text)
{
    if (text.isEmpty()) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("paste")},
        {QStringLiteral("text"), text},
    });
}

void QtShellController::sendClipboardGet()
{
    sendMessage({{QStringLiteral("type"), QStringLiteral("clipboard_get")}});
}

void QtShellController::sendClipboardSet(const QString &text)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("clipboard_set")},
        {QStringLiteral("text"), text},
    });
}

void QtShellController::sendUiAction(const QVariantMap &action)
{
    QVariantMap message = action;
    message.insert(QStringLiteral("type"), QStringLiteral("ui_action"));
    if (F4NavigationBenchmarkTrace::enabled()
        && !F4NavigationBenchmarkTrace::benchmarkTraceId(message).isValid()) {
        const quint64 actionSequence = m_nextActionSequence++;
        message.insert(QStringLiteral("benchmarkTraceId"),
                       QStringLiteral("qt:action:%1:%2")
                           .arg(QCoreApplication::applicationPid())
                           .arg(actionSequence));
    }
    sendMessage(message);
}

void QtShellController::sendPanelCatalogMetadataRequest(
    const QVariantMap &request)
{
    QVariantMap message = request;
    message.insert(QStringLiteral("type"),
                   QStringLiteral("panel_catalog_metadata_request"));
    sendMessage(message);
}

void QtShellController::sendQuit()
{
    sendMessage({{QStringLiteral("type"), QStringLiteral("quit")}});
}

void QtShellController::onConnected()
{
    if (!m_connected) {
        m_connected = true;
        emit connectedChanged();
    }
    if (m_helloSent) {
        return;
    }
    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.connected"));
    }

    const int cellWidth = 10;
    const int cellHeight = 20;
    const bool helloWritten = sendMessage({
        {QStringLiteral("type"), QStringLiteral("hello")},
        {QStringLiteral("nonce"), m_nonce},
        {QStringLiteral("cols"), m_initialCols},
        {QStringLiteral("rows"), m_initialRows},
        {QStringLiteral("pixelWidth"), m_initialCols * cellWidth},
        {QStringLiteral("pixelHeight"), m_initialRows * cellHeight},
        {QStringLiteral("cellWidth"), cellWidth},
        {QStringLiteral("cellHeight"), cellHeight},
        {QStringLiteral("capabilities"), QVariantMap{
             {QStringLiteral("panelCatalogMetadataV1"), true},
         }},
    });
    m_helloSent = helloWritten;
    if (!helloWritten && m_startupError.isEmpty()) {
        // sendMessage already emitted the fatal diagnostic. Persist the same
        // startup result so completeInitialHandshake() cannot emit a second
        // timeout while unwinding that failed hello write.
        m_startupError = QStringLiteral("Failed to write complete IPC frame");
    }
    if (F4NavigationBenchmarkTrace::enabled()) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.startup.hello.sent"), {}, {
                {QStringLiteral("success"), helloWritten},
            });
    }
}

void QtShellController::onReadyRead()
{
    processBuffer();
}

void QtShellController::onDisconnected()
{
    invalidateDecodeSession();
    const bool wasConnected = m_connected;
    if (m_connected) {
        m_connected = false;
        emit connectedChanged();
    }
    if (!m_initialHandshakeComplete) {
        // A peer can accept the TCP connection and close it after receiving
        // our hello but before its protocol hello is decoded. Treat that as a
        // startup failure. A queued quit before app.exec() would otherwise be
        // discarded, leaving an invisible host in its event loop forever.
        if (m_startupError.isEmpty()) {
            m_startupError = QStringLiteral(
                "The f4 core disconnected before completing the protocol handshake");
            const QString message = m_startupError;
            QTimer::singleShot(0, this, [this, message]() {
                emit fatalError(message);
            });
        }
        return;
    }
    if (!wasConnected) {
        return;
    }
    // A loopback peer can disconnect while waitForConnected() is running,
    // before app.exec(). A queued quit works both before and during the loop.
    if (QCoreApplication *application = QCoreApplication::instance()) {
        QMetaObject::invokeMethod(application, []() {
            QCoreApplication::quit();
        }, Qt::QueuedConnection);
    }
}

void QtShellController::onSocketError(QAbstractSocket::SocketError)
{
    invalidateDecodeSession();
    const QString message = m_socket->errorString();
    if (!m_initialHandshakeComplete) {
        if (!m_startupError.isEmpty()) {
            return;
        }
        m_startupError = message;
    }
    // connectToHost() can fail synchronously in the constructor, before
    // main.cpp attaches its observer. Re-emit on the first event-loop turn so
    // both startup and later socket failures follow the same reliable path.
    QTimer::singleShot(0, this, [this, message]() {
        emit fatalError(message);
    });
}

bool QtShellController::sendMessage(const QVariantMap &message)
{
    const QString messageType = message.value(
        QStringLiteral("type")).toString();
    const bool traceAction = messageType == QStringLiteral("ui_action");
    const bool traceKey = messageType == QStringLiteral("key");
    const bool traceMessage = F4NavigationBenchmarkTrace::enabled()
        && (traceAction || traceKey);
    const QVariant traceId = traceMessage
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(message) : QVariant();
    const quint64 outboundSequence = traceMessage ? m_nextSendSequence++ : 0;
    if (!m_socket || m_socket->state() != QAbstractSocket::ConnectedState) {
        return false;
    }

    QElapsedTimer packTimer;
    qint64 packStartedNs = 0;
    if (traceMessage) {
        packStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        packTimer.start();
    }
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);
    const qint64 packDurationNs = traceMessage
        ? packTimer.nsecsElapsed() : 0;
    const qint64 packCompletedNs = traceMessage
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    const QString actionName = traceAction
        ? message.value(QStringLiteral("action")).toString() : QString();
    const QString traceEventPrefix = traceAction
        ? QStringLiteral("qt.action") : QStringLiteral("qt.key");
    const auto traceFields = [&]() {
        QVariantMap fields = {
            {QStringLiteral("outboundSequence"), outboundSequence},
            {QStringLiteral("messageType"), messageType},
        };
        if (traceAction) {
            fields.insert(QStringLiteral("action"), actionName);
        } else if (traceKey) {
            fields.insert(QStringLiteral("keySequence"),
                          message.value(QStringLiteral("keySequence")));
            fields.insert(QStringLiteral("vk"),
                          message.value(QStringLiteral("vk")));
            fields.insert(QStringLiteral("down"),
                          message.value(QStringLiteral("down")));
            fields.insert(QStringLiteral("repeat"),
                          message.value(QStringLiteral("repeat")));
            fields.insert(QStringLiteral("mods"),
                          message.value(QStringLiteral("mods")));
        }
        return fields;
    };
    const auto logPack = [&]() {
        QVariantMap fields = traceFields();
        fields.insert(QStringLiteral("payloadBytes"),
                      static_cast<qulonglong>(payload.size()));
        F4NavigationBenchmarkTrace::eventAt(
            traceEventPrefix + QStringLiteral(".pack.begin"), packStartedNs,
            traceId, fields);
        QVariantMap completedFields = fields;
        completedFields.insert(QStringLiteral("durationNs"),
                               packDurationNs);
        F4NavigationBenchmarkTrace::eventAt(
            traceEventPrefix + QStringLiteral(".pack.end"), packCompletedNs,
            traceId, completedFields);
    };

    if (payload.size() > MaxMessageSize) {
        if (traceMessage) {
            logPack();
        }
        emit fatalError(QStringLiteral("Message too large for f4 Qt protocol"));
        return false;
    }

    QByteArray frame;
    writeBigEndianSize(frame, static_cast<quint32>(payload.size()));
    frame.append(payload.data(), static_cast<qsizetype>(payload.size()));

    QElapsedTimer writeTimer;
    qint64 writeStartedNs = 0;
    if (traceMessage) {
        writeStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        writeTimer.start();
    }
    const qint64 written = m_socket->write(frame);
    const bool complete = written == frame.size();
    const qint64 socketWriteDurationNs = traceMessage
        ? writeTimer.nsecsElapsed() : 0;
    QElapsedTimer flushTimer;
    qint64 flushStartedNs = 0;
    if (traceMessage) {
        flushStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        flushTimer.start();
    }
    const bool flushed = complete && m_socket->flush();
    const qint64 flushDurationNs = traceMessage
        ? flushTimer.nsecsElapsed() : 0;
    const qint64 flushCompletedNs = traceMessage
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    const qint64 writeDurationNs = traceMessage
        ? writeTimer.nsecsElapsed() : 0;
    const qint64 writeCompletedNs = traceMessage
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    if (traceMessage) {
        logPack();
        QVariantMap fields = traceFields();
        fields.insert(QStringLiteral("wireBytes"), frame.size());
        F4NavigationBenchmarkTrace::eventAt(
            traceEventPrefix + QStringLiteral(".write.begin"), writeStartedNs,
            traceId, fields);
        QVariantMap completedFields = fields;
        completedFields.insert(QStringLiteral("writtenBytes"), written);
        completedFields.insert(QStringLiteral("success"), complete);
        completedFields.insert(QStringLiteral("flushSuccess"), flushed);
        completedFields.insert(QStringLiteral("socketWriteDurationNs"),
                               socketWriteDurationNs);
        completedFields.insert(QStringLiteral("flushDurationNs"),
                               flushDurationNs);
        completedFields.insert(QStringLiteral("durationNs"),
                               writeDurationNs);
        F4NavigationBenchmarkTrace::eventAt(
            traceEventPrefix + QStringLiteral(".write.end"), writeCompletedNs,
            traceId, completedFields);

        QVariantMap flushFields = fields;
        flushFields.insert(QStringLiteral("writtenBytes"), written);
        flushFields.insert(QStringLiteral("attempted"), complete);
        F4NavigationBenchmarkTrace::eventAt(
            traceEventPrefix + QStringLiteral(".flush.begin"),
            flushStartedNs, traceId, flushFields);
        flushFields.insert(QStringLiteral("success"), flushed);
        flushFields.insert(QStringLiteral("durationNs"), flushDurationNs);
        F4NavigationBenchmarkTrace::eventAt(
            traceEventPrefix + QStringLiteral(".flush.end"),
            flushCompletedNs, traceId, flushFields);
    }
    if (written != frame.size()) {
        emit fatalError(QStringLiteral("Failed to write complete IPC frame"));
        return false;
    }
    return true;
}

bool QtShellController::parseConnectAddress(const QString &address)
{
    const int split = address.lastIndexOf(QLatin1Char(':'));
    if (split <= 0 || split == address.size() - 1) {
        return false;
    }

    bool ok = false;
    const int port = address.mid(split + 1).toInt(&ok);
    if (!ok || port <= 0 || port > 65535) {
        return false;
    }

    m_host = address.left(split);
    m_port = static_cast<quint16>(port);
    return true;
}

bool QtShellController::canQueueFrame(quint32 payloadSize) const
{
    const qsizetype retainedFrames = m_queuedFrames.size()
        + (m_applyInProgress ? 1 : 0);
    if (!m_acceptDecodedFrames || !m_decoder
        || !m_decodeThread.isRunning()
        || retainedFrames >= MaxQueuedDecodeFrames) {
        return false;
    }

    const qsizetype size = static_cast<qsizetype>(payloadSize);
    const qsizetype retainedBytes = m_queuedPayloadBytes
        + m_applyingPayloadBytes;
    return size <= MaxQueuedDecodeBytes
        && retainedBytes <= MaxQueuedDecodeBytes - size;
}

void QtShellController::processBuffer()
{
    // Read directly into the final frame allocation. In particular, avoid a
    // growing aggregate QByteArray followed by a large mid()/remove() pair on
    // the GUI thread: semantic scenes can be tens of megabytes.
    while (m_socket && m_socket->bytesAvailable() > 0) {
        const qsizetype retainedFrames = m_queuedFrames.size()
            + (m_applyInProgress ? 1 : 0);
        if (retainedFrames >= MaxQueuedDecodeFrames) {
            return;
        }
        if (m_expectedFrameSize == 0) {
            if (m_frameHeader.isEmpty()
                && F4NavigationBenchmarkTrace::enabled()) {
                m_frameReceiveTimer.start();
            }
            const qsizetype headerRemaining = 4 - m_frameHeader.size();
            const QByteArray headerPart = m_socket->read(headerRemaining);
            if (headerPart.isEmpty()) {
                return;
            }
            m_frameHeader.append(headerPart);
            if (m_frameHeader.size() < 4) {
                return;
            }

            const quint32 size = readBigEndianSize(m_frameHeader);
            m_frameHeader.clear();
            if (size == 0 || size > MaxMessageSize) {
                failProtocol(QStringLiteral("Invalid IPC frame size from f4"));
                return;
            }

            m_expectedFrameSize = size;
            m_frameBytesRead = 0;
        }

        // Do not consume a frame that would exceed the decode/apply backlog
        // budget. Its header may already have been read, but the payload stays
        // in QTcpSocket so normal TCP backpressure remains effective.
        if (m_framePayload.isEmpty()) {
            if (!canQueueFrame(m_expectedFrameSize)) {
                return;
            }
            m_framePayload.resize(
                static_cast<qsizetype>(m_expectedFrameSize));
        }

        const qsizetype remaining = static_cast<qsizetype>(m_expectedFrameSize)
            - m_frameBytesRead;
        const qint64 count = m_socket->read(
            m_framePayload.data() + m_frameBytesRead, remaining);
        if (count <= 0) {
            return;
        }
        m_frameBytesRead += static_cast<qsizetype>(count);
        if (m_frameBytesRead == static_cast<qsizetype>(m_expectedFrameSize)) {
            QByteArray payload = std::move(m_framePayload);
            const qint64 receiveDurationNs = m_frameReceiveTimer.isValid()
                ? m_frameReceiveTimer.nsecsElapsed() : 0;
            m_frameReceiveTimer.invalidate();
            m_framePayload = QByteArray();
            m_expectedFrameSize = 0;
            m_frameBytesRead = 0;
            enqueueFrame(std::move(payload), receiveDurationNs);
        }
    }
}

void QtShellController::enqueueFrame(QByteArray payload,
                                     qint64 receiveDurationNs)
{
    if (!canQueueFrame(static_cast<quint32>(payload.size()))) {
        failProtocol(QStringLiteral("IPC decode queue capacity changed unexpectedly"));
        return;
    }

    const quint64 epoch = m_decodeEpoch;
    const quint64 sequence = m_nextDecodeSequence++;
    const qsizetype payloadBytes = payload.size();
    m_queuedPayloadBytes += payloadBytes;
    m_queuedFrames.enqueue({
        sequence,
        payloadBytes,
        F4NavigationBenchmarkTrace::enabled()
            ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0,
        receiveDurationNs,
    });

    QPointer<QtShellMessageDecoder> decoder(m_decoder);
    const bool queued = QMetaObject::invokeMethod(
        m_decoder,
        [decoder, payload = std::move(payload), epoch, sequence]() mutable {
            if (decoder) {
                decoder->decode(std::move(payload), epoch, sequence);
            }
        },
        Qt::QueuedConnection);
    if (!queued) {
        failProtocol(QStringLiteral("Failed to queue IPC frame for decoding"));
        return;
    }
    // Publish the diagnostic boundary only after the serial worker owns this
    // frame. A connected observer may enter a nested event loop; posting first
    // prevents a recursively drained later frame from overtaking this one.
    emit frameDecodeQueued(sequence);
}

void QtShellController::onFrameDecoded(quint64 epoch, quint64 sequence,
                                       const QVariant &decoded)
{
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    DeferredDecodeResult result{
        epoch, sequence, decoded, QString(), false,
    };
    if (m_applyInProgress || m_deferredDecodeScheduled
        || !m_deferredDecodeResults.isEmpty()) {
        m_deferredDecodeResults.enqueue(std::move(result));
        scheduleDeferredDecodeResult();
        return;
    }
    applyDecodeResult(std::move(result));
}

void QtShellController::applyFrameDecoded(quint64 epoch, quint64 sequence,
                                          const QVariant &decoded)
{
    // Results from a closed socket (or a preceding decode failure) may still
    // arrive while its worker invocation unwinds. Epochs make those harmless.
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    if (sequence != m_nextApplySequence) {
        failProtocol(QStringLiteral("Out-of-order IPC decode result"));
        return;
    }
    if (m_queuedFrames.isEmpty()
        || m_queuedFrames.head().sequence != sequence) {
        failProtocol(QStringLiteral("Missing IPC decode queue metadata"));
        return;
    }
    ++m_nextApplySequence;
    const QueuedFrameMetadata frame = m_queuedFrames.dequeue();
    m_queuedPayloadBytes -= frame.payloadBytes;
    m_applyingPayloadBytes = frame.payloadBytes;

    // Refill the bounded worker queue before any synchronous scene observers
    // occupy the GUI thread. This lets already-buffered cursor/title/frame
    // updates and the following scene decode while the current scene applies.
    processBuffer();
    if (!m_acceptDecodedFrames) {
        return;
    }

    if (decoded.metaType().id() != QMetaType::QVariantMap) {
        return;
    }

    const QVariantMap message = decoded.toMap();
    const QString messageType = message.value(QStringLiteral("type")).toString();
    if (messageType == QStringLiteral("hello")) {
        bool protocolOK = false;
        const int protocol = message.value(QStringLiteral("protocol"))
                                 .toInt(&protocolOK);
        if (!protocolOK || protocol != ExtUiProtocolVersion
            || message.value(QStringLiteral("nonce")).toString()
                != m_nonce) {
            failProtocol(QStringLiteral(
                "Incompatible f4 Qt protocol handshake"));
            return;
        }
        m_initialHandshakeComplete = true;
    }
    const bool traceEnabled = F4NavigationBenchmarkTrace::enabled();
    const QVariant traceId = traceEnabled
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(message) : QVariant();
    QElapsedTimer applyTimer;
    qint64 applyStartedNs = 0;
    if (traceEnabled) {
        applyStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        applyTimer.start();
    }
    qint64 presentationDurationNs = 0;
    qint64 sceneSignalDurationNs = 0;
    qint64 presentationStartedNs = 0;
    qint64 sceneSignalStartedNs = 0;
    qint64 presentationCompletedNs = 0;
    qint64 sceneSignalCompletedNs = 0;
    qint64 catalogValidationDurationNs = 0;
    qint64 catalogScenePatchDurationNs = 0;
    qint64 compactApplyingDurationNs = 0;
    qint64 panelCatalogSignalDurationNs = 0;
    qint64 catalogPresentationSignalDurationNs = 0;
    qint64 scenePatchCoreDurationNs = 0;
    qint64 scenePatchCompactApplyingDurationNs = 0;
    qint64 scenePatchCompactRootDurationNs = 0;
    qint64 scenePatchPresentationSignalDurationNs = 0;
    if (messageType == QStringLiteral("scene")) {
        if (message.value(QStringLiteral("schema")).toString()
            == QStringLiteral("app")) {
            qulonglong version = 0;
            qulonglong revision = 0;
            if (!nonNegativeInteger(
                    message.value(QStringLiteral("version")), &version)
                || version != 4
                || !nonNegativeInteger(
                    message.value(QStringLiteral("revision")), &revision)
                || revision == 0
                || (m_sceneRevision > 0
                    && revision != m_sceneRevision + 1)) {
                failProtocol(QStringLiteral(
                    "Invalid or out-of-order full app scene"));
                return;
            }
            m_sceneRevision = revision;
        } else if (m_sceneRevision > 0) {
            failProtocol(QStringLiteral(
                "Legacy scene cannot replace an app scene"));
            return;
        }
        m_scene = message;
        QElapsedTimer presentationTimer;
        if (traceEnabled) {
            presentationStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            presentationTimer.start();
        }
        m_presentationScene = makePresentationScene(message);
        if (traceEnabled) {
            presentationDurationNs = presentationTimer.nsecsElapsed();
            presentationCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
        const QVariantMap nextCommandLine = m_scene.value(QStringLiteral("shell"))
                                                .toMap()
                                                .value(QStringLiteral("commandLine"))
                                                .toMap();
        if (nextCommandLine != m_commandLine) {
            m_commandLine = nextCommandLine;
            emit commandLineChanged();
        }
        updateCommandMenus(m_scene.value(QStringLiteral("menus")).toList(),
                           false);
        QElapsedTimer sceneSignalTimer;
        if (traceEnabled) {
            sceneSignalStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            sceneSignalTimer.start();
        }
        emit sceneChanged();
        emit presentationSceneChanged();
        if (traceEnabled) {
            sceneSignalDurationNs = sceneSignalTimer.nsecsElapsed();
            sceneSignalCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
    } else if (messageType == QStringLiteral("scene_patch")) {
        QElapsedTimer scenePatchStageTimer;
        if (traceEnabled) {
            scenePatchStageTimer.start();
        }
        AppliedScenePatch applied;
        QString patchError;
        if (!applyScenePatch(message, m_scene, m_presentationScene,
                             m_sceneRevision, &applied, &patchError)) {
            failProtocol(patchError.isEmpty()
                ? QStringLiteral("Invalid app scene patch") : patchError);
            return;
        }

        const QString previousIconSet = m_scene.value(
            QStringLiteral("qmlIconSet")).toString();
        m_scene = std::move(applied.scene);
        m_presentationScene = std::move(applied.presentationScene);
        m_sceneRevision = applied.revision;

        const QVariantMap nextCommandLine = m_scene.value(
            QStringLiteral("shell")).toMap().value(
                QStringLiteral("commandLine")).toMap();
        if (nextCommandLine != m_commandLine) {
            m_commandLine = nextCommandLine;
            emit commandLineChanged();
        }
        updateCommandMenus(m_scene.value(QStringLiteral("menus")).toList(),
                           true);
        const QString nextIconSet = m_scene.value(
            QStringLiteral("qmlIconSet")).toString();
        if (nextIconSet != previousIconSet) {
            emit qmlIconSetChanged(nextIconSet);
        }

        if (traceEnabled) {
            scenePatchCoreDurationNs = scenePatchStageTimer.nsecsElapsed();
            scenePatchStageTimer.restart();
        }

        emit compactMessageApplying(message);
        if (traceEnabled) {
            scenePatchCompactApplyingDurationNs =
                scenePatchStageTimer.nsecsElapsed();
        }
        for (const QVariantMap &panel : applied.catalogPanels) {
            emit panelCatalogChanged(panel);
        }
        for (const QVariantMap &panelPatch : applied.panelPatches) {
            emit panelStateChanged(panelPatch);
        }
        for (const QVariantMap &compactPatch : applied.compactPatches) {
            emit compactPresentationChanged(compactPatch);
        }
        QVariantMap compactRootPatch;
        compactRootPatch.insert(QStringLiteral("type"),
                                QStringLiteral("scene_patch"));
        for (const QString &key : {
                 QStringLiteral("workspaceTabs"),
                 QStringLiteral("menuBar"),
                 QStringLiteral("keyBar"),
                 QStringLiteral("toast")}) {
            if (!applied.rootKeys.contains(key)) {
                continue;
            }
            const QVariant value = m_scene.value(key);
            compactRootPatch.insert(
                key, value.metaType().id() == QMetaType::QVariantMap
                    ? value : QVariant(QVariantMap{}));
        }
        if (applied.shellKeys.contains(QStringLiteral("activePanel"))) {
            compactRootPatch.insert(QStringLiteral("activePanel"),
                m_scene.value(QStringLiteral("shell")).toMap().value(
                    QStringLiteral("activePanel")));
        }
        for (const QString &key : {QStringLiteral("shell"),
                                   QStringLiteral("surface")}) {
            if (!applied.rootKeys.contains(key)) {
                continue;
            }
            const bool present = m_presentationScene.contains(key)
                && m_presentationScene.value(key).metaType().id()
                    == QMetaType::QVariantMap;
            compactRootPatch.insert(key + QStringLiteral("Present"),
                                    present);
            compactRootPatch.insert(
                key, present ? m_presentationScene.value(key)
                             : QVariant(QVariantMap{}));
        }
        if (applied.rootKeys.contains(QStringLiteral("shell"))) {
            // A panels-to-panels workspace activation replaces the complete
            // row-free shell while both native catalogs stay in Gallery's
            // identity cache. Tell QML to swap that retained panel pair as a
            // single structural transaction instead of ignoring shellPresent
            // merely because another Commander shell is already visible.
            compactRootPatch.insert(QStringLiteral("replaceShell"), true);
            const QVariantMap replacementShell = m_presentationScene.value(
                QStringLiteral("shell")).toMap();
            compactRootPatch.insert(QStringLiteral("activePanel"),
                replacementShell.value(QStringLiteral("activePanel")));
        }
        if (!applied.surfaceKeys.isEmpty()) {
            const QVariantMap surface = m_presentationScene.value(
                QStringLiteral("surface")).toMap();
            QVariantMap state{
                {QStringLiteral("id"),
                 surface.value(QStringLiteral("id"))},
            };
            for (const QString &key : {
                     QStringLiteral("cursorLine"),
                     QStringLiteral("cursorPos"),
                     QStringLiteral("cursorVisualRow"),
                     QStringLiteral("cursorVisualColumn"),
                     QStringLiteral("cursorVisible"),
                     QStringLiteral("cursorShape"),
                     QStringLiteral("cursorAbsoluteRow")}) {
                state.insert(key, surface.value(key));
            }
            compactRootPatch.insert(QStringLiteral("surfaceState"), state);
        }
        if (compactRootPatch.size() > 1) {
            if (traceEnabled) {
                scenePatchStageTimer.restart();
            }
            emit compactPresentationChanged(compactRootPatch);
            if (traceEnabled) {
                scenePatchCompactRootDurationNs =
                    scenePatchStageTimer.nsecsElapsed();
            }
        }

        QSet<QString> presentationRootKeys = applied.rootKeys;
        presentationRootKeys.remove(QStringLiteral("menus"));
        presentationRootKeys.remove(QStringLiteral("workspaceTabs"));
        presentationRootKeys.remove(QStringLiteral("menuBar"));
        presentationRootKeys.remove(QStringLiteral("keyBar"));
        presentationRootKeys.remove(QStringLiteral("toast"));
        presentationRootKeys.remove(QStringLiteral("width"));
        presentationRootKeys.remove(QStringLiteral("height"));
        presentationRootKeys.remove(QStringLiteral("activeScreen"));
        presentationRootKeys.remove(QStringLiteral("workspaceCount"));
        presentationRootKeys.remove(QStringLiteral("qmlIconSet"));
        // Shell/surface presence and bounded cursor state have dedicated
        // compact QML projections above. Invalidating the complete
        // presentation scene here would synchronously reevaluate the entire
        // persistent Commander object tree for a document-only change.
        presentationRootKeys.remove(QStringLiteral("shell"));
        presentationRootKeys.remove(QStringLiteral("surface"));
        QSet<QString> presentationShellKeys = applied.shellKeys;
        presentationShellKeys.remove(QStringLiteral("commandLine"));
        presentationShellKeys.remove(QStringLiteral("activePanel"));
        if (!presentationRootKeys.isEmpty()
            || !presentationShellKeys.isEmpty()) {
            if (traceEnabled) {
                scenePatchStageTimer.restart();
            }
            emit presentationSceneChanged();
            if (traceEnabled) {
                scenePatchPresentationSignalDurationNs =
                    scenePatchStageTimer.nsecsElapsed();
            }
        }
    } else if (messageType == QStringLiteral("panel_catalog")) {
        int side = -1;
        QVariantMap panel;
        QElapsedTimer catalogStageTimer;
        if (traceEnabled) {
            catalogStageTimer.start();
        }
        const bool validCatalog = validPanelCatalogEnvelope(
            message, m_scene, &side, &panel);
        if (traceEnabled) {
            catalogValidationDurationNs = catalogStageTimer.nsecsElapsed();
        }
        if (validCatalog) {
            if (traceEnabled) {
                catalogStageTimer.restart();
            }
            QVariantMap nextScene = m_scene;
            QVariantMap nextPresentationScene = m_presentationScene;
            const QVariantMap presentationPanel =
                withoutNativePanelPayload(panel);
            const QVariantMap presentationPatch =
                compactPresentationPatch(message, presentationPanel);
            bool activePanelOK = false;
            const int activePanel = message.value(
                QStringLiteral("activePanel")).toInt(&activePanelOK);
            if (activePanelOK
                && replaceShellPanel(nextScene, side, panel)
                && replaceShellPanel(nextPresentationScene, side,
                                     presentationPanel)) {
                applyPanelCatalogCompactFields(nextScene, message,
                                               activePanel);
                applyPanelCatalogCompactFields(nextPresentationScene,
                                               message, activePanel);
                m_scene = std::move(nextScene);
                m_presentationScene = std::move(nextPresentationScene);
                if (traceEnabled) {
                    catalogScenePatchDurationNs =
                        catalogStageTimer.nsecsElapsed();
                }

                if (message.contains(QStringLiteral("commandLine"))) {
                    const QVariantMap nextCommandLine = message.value(
                        QStringLiteral("commandLine")).toMap();
                    if (nextCommandLine != m_commandLine) {
                        m_commandLine = nextCommandLine;
                        emit commandLineChanged();
                    }
                }
                if (message.contains(QStringLiteral("menus"))) {
                    updateCommandMenus(message.value(
                        QStringLiteral("menus")).toList(), true);
                }

                // The bridge installs the authoritative base catalog before
                // QML observes its compact chrome/presentation counterpart.
                if (traceEnabled) {
                    catalogStageTimer.restart();
                }
                emit compactMessageApplying(message);
                if (traceEnabled) {
                    compactApplyingDurationNs =
                        catalogStageTimer.nsecsElapsed();
                    catalogStageTimer.restart();
                }
                emit panelCatalogChanged(panel);
                if (traceEnabled) {
                    panelCatalogSignalDurationNs =
                        catalogStageTimer.nsecsElapsed();
                    catalogStageTimer.restart();
                }
                emit compactPresentationChanged(presentationPatch);
                if (traceEnabled) {
                    catalogPresentationSignalDurationNs =
                        catalogStageTimer.nsecsElapsed();
                }
            }
        }
    } else if (messageType == QStringLiteral("panel_chrome")) {
        int activePanel = -1;
        if (validPanelChromeEnvelope(message, &activePanel)) {
            applyPanelCatalogCompactFields(m_scene, message, activePanel);
            applyPanelCatalogCompactFields(m_presentationScene, message,
                                           activePanel);
            if (message.contains(QStringLiteral("commandLine"))) {
                const QVariantMap nextCommandLine = message.value(
                    QStringLiteral("commandLine")).toMap();
                if (nextCommandLine != m_commandLine) {
                    m_commandLine = nextCommandLine;
                    emit commandLineChanged();
                }
            }
            if (message.contains(QStringLiteral("menus"))) {
                updateCommandMenus(message.value(
                    QStringLiteral("menus")).toList(), true);
            }
            // Keep both authoritative caches current without invalidating
            // the complete QML presentation. Only the validated row-free
            // projection crosses into QML; command line and menus retain
            // their dedicated properties.
            emit compactPresentationChanged(
                compactPresentationPatch(message));
        }
    } else if (messageType == QStringLiteral("panel_activation")) {
        bool sideOK = false;
        bool revisionOK = false;
        const int activePanel = message.value(QStringLiteral("activePanel"))
                                    .toInt(&sideOK);
        const qulonglong revision = message.value(QStringLiteral("revision"))
                                        .toULongLong(&revisionOK);
        const bool shellTitleOK = !message.contains(
            QStringLiteral("shellTitle"))
            || message.value(QStringLiteral("shellTitle")).metaType().id()
                == QMetaType::QString;
        const bool commandLineOK = !message.contains(
            QStringLiteral("commandLine"))
            || message.value(QStringLiteral("commandLine")).metaType().id()
                == QMetaType::QVariantMap;
        if (sideOK && activePanel >= 0 && activePanel <= 1
            && revisionOK && shellTitleOK && commandLineOK
            && revision > m_panelActivationRevision) {
            m_panelActivationRevision = revision;
            m_scene = applyPanelActivationPatch(std::move(m_scene),
                                                activePanel);
            m_presentationScene = applyPanelActivationPatch(
                std::move(m_presentationScene), activePanel);
            QVariantMap compactFields;
            if (message.contains(QStringLiteral("shellTitle"))) {
                compactFields.insert(QStringLiteral("shellTitle"),
                                     message.value(QStringLiteral("shellTitle")));
            }
            if (message.contains(QStringLiteral("commandLine"))) {
                compactFields.insert(QStringLiteral("commandLine"),
                                     message.value(QStringLiteral("commandLine")));
            }
            for (auto it = message.cbegin(); it != message.cend(); ++it) {
                if (it.key().startsWith(QStringLiteral("benchmark"))) {
                    compactFields.insert(it.key(), it.value());
                }
            }
            applyPanelCatalogCompactFields(m_scene, compactFields,
                                           activePanel);
            applyPanelCatalogCompactFields(m_presentationScene,
                                           compactFields, activePanel);
            if (message.contains(QStringLiteral("commandLine"))) {
                const QVariantMap nextCommandLine = message.value(
                    QStringLiteral("commandLine")).toMap();
                if (nextCommandLine != m_commandLine) {
                    m_commandLine = nextCommandLine;
                    emit commandLineChanged();
                }
            }
            // The bridge acknowledgement precedes QML's visual update so
            // pending Gallery intents observe the authoritative active side.
            emit compactMessageApplying(message);
            emit panelActivationChanged(activePanel, revision);
        }
    } else if (messageType == QStringLiteral("command_line")) {
        const QVariant commandLine = message.value(QStringLiteral("commandLine"));
        QVariantMap shell = m_scene.value(QStringLiteral("shell")).toMap();
        if (!shell.isEmpty() && commandLine.metaType().id() == QMetaType::QVariantMap) {
            const QVariantMap nextCommandLine = commandLine.toMap();
            shell.insert(QStringLiteral("commandLine"), nextCommandLine);
            m_scene.insert(QStringLiteral("shell"), shell);
            QVariantMap presentationShell = m_presentationScene
                                                .value(QStringLiteral("shell"))
                                                .toMap();
            if (!presentationShell.isEmpty()) {
                presentationShell.insert(QStringLiteral("commandLine"),
                                         nextCommandLine);
                m_presentationScene.insert(QStringLiteral("shell"),
                                           presentationShell);
            }
            if (nextCommandLine != m_commandLine) {
                m_commandLine = nextCommandLine;
                emit commandLineChanged();
            }
        }
        const QVariant menus = message.value(QStringLiteral("menus"));
        if (menus.metaType().id() == QMetaType::QVariantList) {
            const QVariantList nextCommandMenus = menus.toList();
            m_scene.insert(QStringLiteral("menus"), nextCommandMenus);
            m_presentationScene.insert(QStringLiteral("menus"),
                                       nextCommandMenus);
            updateCommandMenus(nextCommandMenus, true);
        }
    }
    QElapsedTimer messageSignalTimer;
    qint64 messageSignalStartedNs = 0;
    if (traceEnabled) {
        messageSignalStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        messageSignalTimer.start();
    }
    emit messageReceived(message);
    const qint64 messageSignalDurationNs = traceEnabled
        ? messageSignalTimer.nsecsElapsed() : 0;
    if (traceEnabled) {
        const qint64 messageSignalCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        const qint64 applyDurationNs = applyTimer.nsecsElapsed();
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.socket.frame.received"),
            frame.receivedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("receiveDurationNs"),
                 frame.receiveDurationNs},
            });
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.apply.begin"), applyStartedNs,
            traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
            });
        if (messageType == QStringLiteral("scene")) {
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.presentation.begin"),
                presentationStartedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                });
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.presentation.created"),
                presentationCompletedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                    {QStringLiteral("durationNs"), presentationDurationNs},
                });
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.sceneChanged.begin"),
                sceneSignalStartedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                });
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.scene.sceneChanged.emitted"),
                sceneSignalCompletedNs, traceId, {
                    {QStringLiteral("sequence"), sequence},
                    {QStringLiteral("payloadBytes"), frame.payloadBytes},
                    {QStringLiteral("durationNs"), sceneSignalDurationNs},
                });
        }
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.messageReceived.begin"),
            messageSignalStartedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
            });
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.messageReceived.emitted"),
            messageSignalCompletedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
                {QStringLiteral("durationNs"), messageSignalDurationNs},
            });
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.message.apply.end"),
            messageSignalCompletedNs, traceId, {
                {QStringLiteral("sequence"), sequence},
                {QStringLiteral("payloadBytes"), frame.payloadBytes},
                {QStringLiteral("messageType"), messageType},
                {QStringLiteral("presentationDurationNs"),
                 presentationDurationNs},
                {QStringLiteral("sceneChangedDurationNs"),
                 sceneSignalDurationNs},
                {QStringLiteral("messageReceivedDurationNs"),
                 messageSignalDurationNs},
                {QStringLiteral("catalogValidationDurationNs"),
                 catalogValidationDurationNs},
                {QStringLiteral("catalogScenePatchDurationNs"),
                 catalogScenePatchDurationNs},
                {QStringLiteral("compactApplyingDurationNs"),
                 compactApplyingDurationNs},
                {QStringLiteral("panelCatalogSignalDurationNs"),
                 panelCatalogSignalDurationNs},
                {QStringLiteral("catalogPresentationSignalDurationNs"),
                 catalogPresentationSignalDurationNs},
                {QStringLiteral("scenePatchCoreDurationNs"),
                 scenePatchCoreDurationNs},
                {QStringLiteral("scenePatchCompactApplyingDurationNs"),
                 scenePatchCompactApplyingDurationNs},
                {QStringLiteral("scenePatchCompactRootDurationNs"),
                 scenePatchCompactRootDurationNs},
                {QStringLiteral("scenePatchPresentationSignalDurationNs"),
                 scenePatchPresentationSignalDurationNs},
                {QStringLiteral("durationNs"), applyDurationNs},
            });
    }
}

void QtShellController::onFrameDecodeFailed(quint64 epoch, quint64 sequence,
                                            const QString &message)
{
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    DeferredDecodeResult result{
        epoch, sequence, QVariant(), message, true,
    };
    if (m_applyInProgress || m_deferredDecodeScheduled
        || !m_deferredDecodeResults.isEmpty()) {
        m_deferredDecodeResults.enqueue(std::move(result));
        scheduleDeferredDecodeResult();
        return;
    }
    applyDecodeResult(std::move(result));
}

void QtShellController::applyFrameDecodeFailed(
    quint64 epoch, quint64 sequence, const QString &message)
{
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    if (sequence != m_nextApplySequence) {
        failProtocol(QStringLiteral("Out-of-order IPC decode failure"));
        return;
    }
    failProtocol(QStringLiteral("Failed to decode IPC frame: %1").arg(message));
}

void QtShellController::applyDecodeResult(DeferredDecodeResult result)
{
    if (!m_acceptDecodedFrames || result.epoch != m_decodeEpoch) {
        return;
    }

    m_applyInProgress = true;
    if (result.failed) {
        applyFrameDecodeFailed(result.epoch, result.sequence,
                               result.error);
    } else {
        applyFrameDecoded(result.epoch, result.sequence,
                          result.decoded);
    }
    m_applyingPayloadBytes = 0;
    m_applyInProgress = false;
    processBuffer();
    scheduleDeferredDecodeResult();
}

void QtShellController::scheduleDeferredDecodeResult()
{
    if (!m_acceptDecodedFrames || m_applyInProgress
        || m_deferredDecodeScheduled
        || m_deferredDecodeResults.isEmpty()) {
        return;
    }

    m_deferredDecodeScheduled = true;
    QMetaObject::invokeMethod(this, [this]() {
        m_deferredDecodeScheduled = false;
        if (!m_acceptDecodedFrames
            || m_deferredDecodeResults.isEmpty()) {
            return;
        }
        DeferredDecodeResult result =
            m_deferredDecodeResults.dequeue();
        applyDecodeResult(std::move(result));
    }, Qt::QueuedConnection);
}

void QtShellController::invalidateDecodeSession()
{
    m_acceptDecodedFrames = false;
    ++m_decodeEpoch;
    m_frameHeader.clear();
    m_framePayload.clear();
    m_frameReceiveTimer.invalidate();
    m_expectedFrameSize = 0;
    m_frameBytesRead = 0;
    m_queuedFrames.clear();
    m_queuedPayloadBytes = 0;
    m_applyingPayloadBytes = 0;
    m_deferredDecodeResults.clear();
    m_deferredDecodeScheduled = false;
}

void QtShellController::failProtocol(const QString &message)
{
    if (m_protocolFailed) {
        return;
    }
    m_protocolFailed = true;
    if (!m_initialHandshakeComplete && m_startupError.isEmpty()) {
        m_startupError = message;
    }
    invalidateDecodeSession();
    emit fatalError(message);
    if (m_socket) {
        m_socket->disconnectFromHost();
    }
}

#include "QtShellController.moc"
