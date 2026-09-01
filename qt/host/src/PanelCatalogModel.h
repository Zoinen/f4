#pragma once

#include <QHash>
#include <QList>
#include <QSet>
#include <QStringList>
#include <QVariantList>
#include <QVariantMap>

// Typed state for one panel catalog stream. The logical row count is kept
// separately from the bounded materialized pages, so a 30K-row directory
// never allocates 30K QVariant placeholders in the Qt host.
class PanelCatalogModel
{
public:
    struct MetadataRange
    {
        int begin = 0;
        int end = 0;
    };

    bool rowLoaded(int row) const;
    int entryCount() const;
    QVariantMap entryAt(int row) const;
    bool setEntry(int row, const QVariant &entry);

    bool initialized = false;
    QString panelId;
    qulonglong catalogRevision = 0;
    // Newest semantic revision received for the catalog which is still
    // interactive. It can lead catalogRevision while a bounded source
    // completion is intentionally held out of rendering.
    qulonglong latestSemanticCatalogRevision = 0;
    qulonglong selectionRevision = 0;
    qulonglong highlightRevision = 0;
    qulonglong iconRevision = 0;
    QString currentPath;
    QString sourceKind;
    QString cursorEntryId;
    int cursorIndex = -1;
    bool previewCapable = false;
    bool active = false;
    bool loading = false;
    bool catalogProvisional = false;
    qulonglong provisionalFrameRequiredRenderSyncSerial = 0;
    bool catalogRowsDeferred = false;
    bool catalogRowsRequestInFlight = false;
    int catalogRowsRequestOffset = -1;
    int catalogRowsRequestLimit = 0;
    int catalogRowsVisibleFirst = -1;
    int catalogRowsVisibleLast = -1;
    int totalCount = 0;
    bool metadataDeferred = false;
    bool metadataComplete = true;
    bool metadataRequestInFlight = false;
    bool metadataAwaitingFrame = false;
    qulonglong metadataRequiredRenderSyncSerial = 0;
    qulonglong metadataPacingGeneration = 0;
    qulonglong metadataRevision = 0;
    int metadataRequestOffset = -1;
    int metadataRequestLimit = 0;
    int metadataVisibleFirst = -1;
    int metadataVisibleLast = -1;
    int metadataUrgentBudget = 0;
    int metadataFailureCount = 0;
    QList<MetadataRange> metadataPendingRanges;
    QString galleryLayoutMode;

    // Sparse catalogs keep only materialized viewport pages. rowCount is
    // totalCount; this map resolves a logical row to its compact payload slot.
    QVariantList entries;
    QHash<int, int> entryOffsetByRow;
    QStringList selectedEntryIdList;
    QHash<QString, int> sourceIndexByEntryId;
    QSet<QString> entryIds;
    QSet<QString> selectedEntryIds;
};
