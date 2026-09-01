#pragma once

#include <QObject>
#include <QString>

class ViewerCoordinator final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool visible READ visible NOTIFY changed)
    Q_PROPERTY(int side READ side NOTIFY changed)

public:
    struct PendingIntent
    {
        bool active = false;
        int side = -1;
        QString panelId;
        QString entryId;
        qulonglong catalogRevision = 0;
    };

    explicit ViewerCoordinator(QObject *parent = nullptr);

    bool visible() const { return m_visible; }
    int side() const { return m_side; }
    const PendingIntent &pendingIntent() const { return m_pendingIntent; }
    PendingIntent &pendingIntent() { return m_pendingIntent; }

    void beginPending(int side, const QString &panelId,
                      const QString &entryId,
                      qulonglong catalogRevision);
    void clearPending();
    void show(int side);
    void hide();

signals:
    void changed();

private:
    void setVisibleState(bool visible, int side);

    PendingIntent m_pendingIntent;
    bool m_visible = false;
    int m_side = -1;
};
