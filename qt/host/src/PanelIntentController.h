#pragma once

#include <QObject>
#include <QString>
#include <QVariantList>
#include <QVariantMap>

// Internal typed representation of panel actions. QVariantMap exists only at
// the Go transport boundary; bridge and coordinators build one of these fixed
// intents instead of spelling wire keys throughout the implementation.
struct PanelIntent
{
    enum class Kind
    {
        Activate,
        Cursor,
        Open,
        SetSelection,
        SetGalleryLayout,
        SetGalleryDensity,
        Sort,
        SortMenu,
    };

    Kind kind = Kind::Activate;
    int side = -1;
    QString entryId;
    int index = -1;
    qulonglong catalogRevision = 0;
    bool includeCatalogRevision = false;
    bool activate = false;

    QString mode;
    QVariantList entryIds;
    QVariantList changes;
    QString cursorEntryId;
    int cursorIndex = -1;
    qulonglong selectionRevision = 0;

    QString layoutMode;
    int columnCount = 0;
    int density = 0;
};

Q_DECLARE_METATYPE(PanelIntent)

class PanelIntentController final : public QObject
{
    Q_OBJECT

public:
    explicit PanelIntentController(QObject *parent = nullptr);

    void dispatch(const PanelIntent &intent);
    static QVariantMap toWireMap(const PanelIntent &intent);

signals:
    void intentRequested(const PanelIntent &intent);
};
