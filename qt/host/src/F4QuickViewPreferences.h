#pragma once
#include <QObject>
#include <QSettings>
#include <QVariantMap>

// F4 presentation preferences, deliberately outside ZoinGallery's settings.
class F4QuickViewPreferences final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(QVariantMap values READ values NOTIFY changed)
    Q_PROPERTY(QString error READ error NOTIFY changed)
public:
    explicit F4QuickViewPreferences(QObject *parent = nullptr) : QObject(parent)
    {
        QSettings settings;
        m_values = {{"useBuiltinF4Viewer", settings.value("QuickView/useBuiltinF4Viewer", false).toBool()},
                    {"previewOnHover", settings.value("QuickView/previewOnHover", true).toBool()}};
    }
    QVariantMap values() const { return m_values; }
    QString error() const { return m_error; }
    bool builtin() const { return m_values.value("useBuiltinF4Viewer").toBool(); }
    bool hover() const { return m_values.value("previewOnHover").toBool(); }
    Q_INVOKABLE bool apply(const QVariantMap &values)
    {
        const QVariantMap next{{"useBuiltinF4Viewer", values.value("useBuiltinF4Viewer", false).toBool()},
                               {"previewOnHover", values.value("previewOnHover", true).toBool()}};
        QSettings settings;
        for (auto it = next.cbegin(); it != next.cend(); ++it)
            settings.setValue("QuickView/" + it.key(), it.value());
        settings.sync();
        if (settings.status() != QSettings::NoError) {
            m_error = tr("Could not save Quick View settings.");
            emit changed();
            return false;
        }
        m_error.clear();
        m_values = next;
        emit changed();
        return true;
    }
signals:
    void changed();
private:
    QVariantMap m_values;
    QString m_error;
};
