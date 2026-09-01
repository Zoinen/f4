#pragma once

#include <QList>
#include <QSet>
#include <QString>
#include <QVariantList>
#include <QVariantMap>

namespace ExtUiSceneReducer
{
struct AppliedScenePatch
{
    QVariantMap scene;
    QVariantMap presentationScene;
    QList<QVariantMap> catalogPanels;
    QList<QVariantMap> catalogAppends;
    QList<QVariantMap> panelPatches;
    QList<QVariantMap> compactPatches;
    QSet<QString> rootKeys;
    QSet<QString> shellKeys;
    QSet<QString> surfaceKeys;
    qulonglong revision = 0;
};

bool nonNegativeInteger(const QVariant &value,
                        qulonglong *result = nullptr);
bool hasNonEmptyMap(const QVariantMap &container, const QString &key);

QVariantMap makePresentationScene(QVariantMap scene);
QVariantMap makePresentationMessage(
    const QVariantMap &message,
    const QVariantMap &presentationScene);
QVariantMap applyPanelActivationPatch(QVariantMap scene, int activeSide);
QVariantMap withoutNativePanelPayload(QVariantMap panel);
QVariantMap compactPresentationPatch(
    const QVariantMap &message,
    const QVariantMap &presentationPanel = QVariantMap());
bool replaceShellPanel(QVariantMap &scene, int side,
                       const QVariantMap &replacement);
bool validPanelCatalogEnvelope(const QVariantMap &message,
                               const QVariantMap &currentScene,
                               int *sideOut, QVariantMap *panelOut);
bool validPanelChromeEnvelope(const QVariantMap &message,
                              int *activePanelOut);
void applyPanelCatalogCompactFields(QVariantMap &scene,
                                    const QVariantMap &message,
                                    int activePanel);
bool shellPanelAtSide(const QVariantMap &scene, int side,
                      QVariantMap *panelOut);
bool applyScenePatch(const QVariantMap &message,
                     const QVariantMap &currentScene,
                     const QVariantMap &currentPresentationScene,
                     qulonglong currentRevision,
                     AppliedScenePatch *result, QString *error);
bool isAuthoritativePhasedCatalog(const QVariantMap &panel);
bool shellSideIsCovered(const QVariantMap &shell, int side);
QVariantList projectCommandMenuStates(const QVariantList &menus);
bool commandMenuStructuresEqual(const QVariantList &left,
                                const QVariantList &right);
QString snapshotPayloadTypeForStream(const QString &streamId);
bool applyStreamSnapshotPayload(const QString &streamId,
                                const QVariantMap &message,
                                QVariantMap *scene,
                                QVariantMap *catalogPanel,
                                QString *error);
}
