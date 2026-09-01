#pragma once

#include <QObject>
#include <QString>
#include <QVariantMap>

class ShellStateStore final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qulonglong revision READ revision NOTIFY revisionChanged)
    Q_PROPERTY(QString id READ id NOTIFY identityChanged)
    Q_PROPERTY(QString title READ title NOTIFY titleChanged)
    Q_PROPERTY(QString mode READ mode NOTIFY modeChanged)
    Q_PROPERTY(int activePanel READ activePanel NOTIFY activePanelChanged)
    Q_PROPERTY(bool showPanels READ showPanels NOTIFY layoutChanged)
    Q_PROPERTY(bool showLeftPanel READ showLeftPanel NOTIFY layoutChanged)
    Q_PROPERTY(bool showRightPanel READ showRightPanel NOTIFY layoutChanged)
    Q_PROPERTY(bool wide READ wide NOTIFY layoutChanged)
    Q_PROPERTY(int widePanel READ widePanel NOTIFY layoutChanged)
    Q_PROPERTY(bool terminalActive READ terminalActive NOTIFY terminalChanged)
    Q_PROPERTY(bool terminalBusy READ terminalBusy NOTIFY terminalChanged)
    Q_PROPERTY(bool fallback READ fallback NOTIFY fallbackChanged)
    Q_PROPERTY(QString fallbackReason READ fallbackReason NOTIFY fallbackChanged)
    Q_PROPERTY(QVariantMap commandLine READ commandLine NOTIFY commandLineChanged)

public:
    explicit ShellStateStore(QObject *parent = nullptr);

    qulonglong revision() const { return m_revision; }
    QString id() const { return m_id; }
    QString title() const { return m_title; }
    QString mode() const { return m_mode; }
    int activePanel() const { return m_activePanel; }
    bool showPanels() const { return m_showPanels; }
    bool showLeftPanel() const { return m_showLeftPanel; }
    bool showRightPanel() const { return m_showRightPanel; }
    bool wide() const { return m_wide; }
    int widePanel() const { return m_widePanel; }
    bool terminalActive() const { return m_terminalActive; }
    bool terminalBusy() const { return m_terminalBusy; }
    bool fallback() const { return m_fallback; }
    QString fallbackReason() const { return m_fallbackReason; }
    QVariantMap commandLine() const { return m_commandLine; }

    // Extracts only fixed shell roles. The panels list and its catalog rows are
    // deliberately never traversed or retained by this store.
    void applyShell(const QVariantMap &shell, qulonglong revision);
    void reset();

signals:
    void revisionChanged();
    void identityChanged();
    void titleChanged();
    void modeChanged();
    void activePanelChanged();
    void layoutChanged();
    void terminalChanged();
    void fallbackChanged();
    void commandLineChanged();

private:
    qulonglong m_revision = 0;
    QString m_id;
    QString m_title;
    QString m_mode;
    int m_activePanel = -1;
    bool m_showPanels = true;
    bool m_showLeftPanel = true;
    bool m_showRightPanel = true;
    bool m_wide = false;
    int m_widePanel = -1;
    bool m_terminalActive = false;
    bool m_terminalBusy = false;
    bool m_fallback = false;
    QString m_fallbackReason;
    QVariantMap m_commandLine;
};
