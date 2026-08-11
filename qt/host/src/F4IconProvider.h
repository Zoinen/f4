#pragma once

#include <QIcon>
#include <QImage>
#include <QList>
#include <QObject>
#include <QPair>
#include <QQuickImageProvider>
#include <QString>
#include <QStringView>
#include <QUrl>

#include <memory>
#include <mutex>

// The native icon lookup is kept behind this small interface so the image
// provider can be tested without depending on the host desktop's icon theme.
class F4IconProviderBackend
{
public:
    virtual ~F4IconProviderBackend() = default;

    virtual QIcon iconForName(const QString &name) const = 0;
    virtual QIcon iconForFile(const QString &localPath,
                              const QString &fileName,
                              bool directory) const = 0;
};

class F4IconProvider final : public QQuickImageProvider
{
public:
    explicit F4IconProvider(
        std::unique_ptr<F4IconProviderBackend> backend = {});
    ~F4IconProvider() override;

    QImage requestImage(const QString &id,
                        QSize *size,
                        const QSize &requestedSize) override;

    // Public helpers keep URL construction/parsing and high-DPI behavior
    // deterministic in unit tests and in future non-QML callers.
    static QString encodeRouteValue(QStringView value);
    static QString decodeRouteValue(QStringView value, bool *ok = nullptr);
    static QString routeId(const QUrl &url);
    static int normalizedLogicalSize(int logicalSize);
    static qreal normalizedDevicePixelRatio(qreal devicePixelRatio);
    static QSize physicalSize(int logicalSize, qreal devicePixelRatio);
    static QString normalizedIconName(QStringView name);
    static QString lucideFileIconName(QStringView fileName, bool directory);
    static QUrl lucideSource(QStringView name, int logicalSize);

private:
    std::unique_ptr<F4IconProviderBackend> m_backend;
    std::mutex m_mutex;
};

class F4IconSet final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(IconSet iconSet READ iconSet WRITE setIconSet NOTIFY iconSetChanged)
    Q_PROPERTY(QString name READ name WRITE setName NOTIFY iconSetChanged)
    Q_PROPERTY(bool system READ system NOTIFY iconSetChanged)
    Q_PROPERTY(bool fileIconsAreFullColor READ fileIconsAreFullColor
               NOTIFY iconSetChanged)
    Q_PROPERTY(qulonglong revision READ revision NOTIFY revisionChanged)

public:
    enum IconSet {
        Lucide,
        System,
    };
    Q_ENUM(IconSet)

    explicit F4IconSet(QObject *parent = nullptr);
    explicit F4IconSet(QString providerId, QObject *parent = nullptr);

    IconSet iconSet() const { return m_iconSet; }
    QString name() const;
    bool system() const { return m_iconSet == System; }
    // Lucide SVGs are masks and are intentionally tinted by QML. Native file
    // icons may contain several colors and must be displayed by an Image
    // without IconLabel.icon.color/SourceIn recoloring.
    bool fileIconsAreFullColor() const { return system(); }
    qulonglong revision() const { return m_revision; }
    QString providerId() const { return m_providerId; }

    void setIconSet(IconSet iconSet);
    void setName(const QString &name);

    Q_INVOKABLE QUrl iconSource(const QString &name,
                                int logicalSize,
                                qreal devicePixelRatio) const;
    Q_INVOKABLE QUrl fileIconSource(const QString &localPath,
                                    const QString &fileName,
                                    bool directory,
                                    int logicalSize,
                                    qreal devicePixelRatio,
                                    qulonglong version = 0) const;
    Q_INVOKABLE void refresh();

    static QString defaultProviderId();
    static QString normalizedSetName(QStringView name);

signals:
    void iconSetChanged();
    void revisionChanged();

protected:
    bool eventFilter(QObject *watched, QEvent *event) override;

private:
    QUrl systemIconSource(const QString &route,
                          const QString &encodedValue,
                          int logicalSize,
                          qreal devicePixelRatio,
                          qulonglong version,
                          const QList<QPair<QString, QString>> &extraQuery = {}) const;
    void advanceRevision();

    QString m_providerId;
    IconSet m_iconSet = Lucide;
    qulonglong m_revision = 1;
};
