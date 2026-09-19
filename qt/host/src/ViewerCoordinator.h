#pragma once

#include <QObject>
#include <QString>

class ViewerCoordinator final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool visible READ visible NOTIFY changed)
    Q_PROPERTY(int side READ side NOTIFY changed)

public:
    enum State { Closed, Docked, Expanding, Full, Collapsing };
    Q_ENUM(State)
    struct PendingIntent
    {
        bool active = false;
        int side = -1;
        QString panelId;
        QString entryId;
        qulonglong catalogRevision = 0;
    };

    explicit ViewerCoordinator(QObject *parent = nullptr);

    bool visible() const { return m_state == Full || m_state == Expanding || m_state == Collapsing; }
    bool mounted() const { return m_state != Closed; }
    State state() const { return m_state; }
    int dockSide() const { return m_dockSide; }
    int side() const { return m_side; }
    const PendingIntent &pendingIntent() const { return m_pendingIntent; }
    PendingIntent &pendingIntent() { return m_pendingIntent; }

    void beginPending(int side, const QString &panelId,
                      const QString &entryId,
                      qulonglong catalogRevision);
    void clearPending();
    void show(int side);
    void hide();
    void dock(int sourceSide, int destinationSide);
    void removeDock();
    void collapse();
    void settle();

signals:
    void changed();

private:
    void setState(State state, int side);

    PendingIntent m_pendingIntent;
    State m_state = Closed;
    int m_dockSide = -1;
    int m_side = -1;
};
