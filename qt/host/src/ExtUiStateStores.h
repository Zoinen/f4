#pragma once

#include <QObject>
#include <QString>
#include <QVariantList>
#include <QVariantMap>

class ChromeStateStore final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qulonglong revision READ revision NOTIFY revisionChanged)
    Q_PROPERTY(QString schema READ schema NOTIFY identityChanged)
    Q_PROPERTY(int version READ version NOTIFY identityChanged)
    Q_PROPERTY(int width READ width NOTIFY geometryChanged)
    Q_PROPERTY(int height READ height NOTIFY geometryChanged)
    Q_PROPERTY(QString presentation READ presentation NOTIFY presentationChanged)
    Q_PROPERTY(QString qmlIconSet READ qmlIconSet NOTIFY qmlIconSetChanged)
    Q_PROPERTY(QVariantMap keyBar READ keyBar NOTIFY keyBarChanged)
    Q_PROPERTY(QVariantMap toast READ toast NOTIFY toastChanged)

public:
    explicit ChromeStateStore(QObject *parent = nullptr);

    qulonglong revision() const { return m_revision; }
    QString schema() const { return m_schema; }
    int version() const { return m_version; }
    int width() const { return m_width; }
    int height() const { return m_height; }
    QString presentation() const { return m_presentation; }
    QString qmlIconSet() const { return m_qmlIconSet; }
    QVariantMap keyBar() const { return m_keyBar; }
    QVariantMap toast() const { return m_toast; }

    void applyState(const QVariantMap &state, qulonglong revision);
    void reset();

signals:
    void revisionChanged();
    void identityChanged();
    void geometryChanged();
    void presentationChanged();
    void qmlIconSetChanged(const QString &name);
    void keyBarChanged();
    void toastChanged();

private:
    qulonglong m_revision = 0;
    QString m_schema;
    int m_version = 0;
    int m_width = 0;
    int m_height = 0;
    QString m_presentation;
    QString m_qmlIconSet;
    QVariantMap m_keyBar;
    QVariantMap m_toast;
};

class WorkspaceStateStore final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qulonglong revision READ revision NOTIFY revisionChanged)
    Q_PROPERTY(int activeScreen READ activeScreen NOTIFY activeScreenChanged)
    Q_PROPERTY(int workspaceCount READ workspaceCount NOTIFY workspaceCountChanged)
    Q_PROPERTY(QVariantMap tabs READ tabs NOTIFY tabsChanged)

public:
    explicit WorkspaceStateStore(QObject *parent = nullptr);

    qulonglong revision() const { return m_revision; }
    int activeScreen() const { return m_activeScreen; }
    int workspaceCount() const { return m_workspaceCount; }
    QVariantMap tabs() const { return m_tabs; }

    void applyState(const QVariantMap &state, qulonglong revision);
    void reset();

signals:
    void revisionChanged();
    void activeScreenChanged();
    void workspaceCountChanged();
    void tabsChanged();

private:
    qulonglong m_revision = 0;
    int m_activeScreen = 0;
    int m_workspaceCount = 0;
    QVariantMap m_tabs;
};

class OverlayStateStore final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qulonglong menuRevision READ menuRevision NOTIFY menuRevisionChanged)
    Q_PROPERTY(qulonglong dialogRevision READ dialogRevision NOTIFY dialogRevisionChanged)
    Q_PROPERTY(QVariantMap menuBar READ menuBar NOTIFY menuBarChanged)
    Q_PROPERTY(QVariantList commandMenus READ commandMenus NOTIFY commandMenusChanged)
    Q_PROPERTY(QVariantList commandMenuStates READ commandMenuStates NOTIFY commandMenuStatesChanged)
    Q_PROPERTY(QVariantList dialogs READ dialogs NOTIFY dialogsChanged)

public:
    explicit OverlayStateStore(QObject *parent = nullptr);

    qulonglong menuRevision() const { return m_menuRevision; }
    qulonglong dialogRevision() const { return m_dialogRevision; }
    QVariantMap menuBar() const { return m_menuBar; }
    QVariantList commandMenus() const { return m_commandMenus; }
    QVariantList commandMenuStates() const { return m_commandMenuStates; }
    QVariantList dialogs() const { return m_dialogs; }

    void applyMenuState(const QVariantMap &state, qulonglong revision,
                        bool allowStateOnlyUpdate);
    void applyDialogsState(const QVariantMap &state, qulonglong revision);
    void reset();

signals:
    void menuRevisionChanged();
    void dialogRevisionChanged();
    void menuBarChanged();
    void commandMenusChanged();
    void commandMenuStatesChanged(const QVariantList &states);
    void dialogsChanged();

private:
    qulonglong m_menuRevision = 0;
    qulonglong m_dialogRevision = 0;
    QVariantMap m_menuBar;
    QVariantList m_commandMenus;
    QVariantList m_commandMenuStates;
    QVariantList m_dialogs;
};

class CommandLineStateStore final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qulonglong revision READ revision NOTIFY revisionChanged)
    Q_PROPERTY(QVariantMap frame READ frame NOTIFY frameChanged)

public:
    explicit CommandLineStateStore(QObject *parent = nullptr);

    qulonglong revision() const { return m_revision; }
    QVariantMap frame() const { return m_frame; }

    void applyFrame(const QVariantMap &frame, qulonglong revision);
    void reset();

signals:
    void revisionChanged();
    void frameChanged();

private:
    qulonglong m_revision = 0;
    QVariantMap m_frame;
};

class SurfaceRegistry final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qulonglong shellRevision READ shellRevision NOTIFY shellRevisionChanged)
    Q_PROPERTY(qulonglong documentRevision READ documentRevision NOTIFY documentRevisionChanged)
    Q_PROPERTY(qulonglong operationsRevision READ operationsRevision NOTIFY operationsRevisionChanged)
    Q_PROPERTY(QVariantMap shell READ shell NOTIFY shellChanged)
    Q_PROPERTY(QVariantMap document READ document NOTIFY documentChanged)
    Q_PROPERTY(QVariantMap operationsQueue READ operationsQueue NOTIFY operationsQueueChanged)
    Q_PROPERTY(bool hasShell READ hasShell NOTIFY shellChanged)
    Q_PROPERTY(bool hasDocument READ hasDocument NOTIFY documentChanged)
    Q_PROPERTY(bool hasOperationsQueue READ hasOperationsQueue NOTIFY operationsQueueChanged)

public:
    explicit SurfaceRegistry(QObject *parent = nullptr);

    qulonglong shellRevision() const { return m_shellRevision; }
    qulonglong documentRevision() const { return m_documentRevision; }
    qulonglong operationsRevision() const { return m_operationsRevision; }
    QVariantMap shell() const { return m_shell; }
    QVariantMap document() const { return m_document; }
    QVariantMap operationsQueue() const { return m_operationsQueue; }
    bool hasShell() const { return !m_shell.isEmpty(); }
    bool hasDocument() const { return !m_document.isEmpty(); }
    bool hasOperationsQueue() const { return !m_operationsQueue.isEmpty(); }

    void applyShell(const QVariantMap &shell, qulonglong revision);
    void applyDocument(const QVariantMap &document, qulonglong revision);
    void applyOperationsQueue(const QVariantMap &queue,
                              qulonglong revision);
    void reset();

    static QVariantMap withoutCatalogPayload(const QVariantMap &shell);

signals:
    void shellRevisionChanged();
    void documentRevisionChanged();
    void operationsRevisionChanged();
    void shellChanged();
    void documentChanged();
    void operationsQueueChanged();

private:
    qulonglong m_shellRevision = 0;
    qulonglong m_documentRevision = 0;
    qulonglong m_operationsRevision = 0;
    QVariantMap m_shell;
    QVariantMap m_document;
    QVariantMap m_operationsQueue;
};
