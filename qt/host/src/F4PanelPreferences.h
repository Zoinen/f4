#pragma once

#include <QObject>
#include <QSettings>
#include <QVariantMap>

// F4-owned per-panel preferences; they intentionally do not cross ExtUI.
class F4PanelPreferences final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(QVariantMap values READ values NOTIFY changed)
    Q_PROPERTY(QString error READ error NOTIFY changed)

public:
    explicit F4PanelPreferences(QObject *parent = nullptr)
        : QObject(parent)
    {
        QSettings settings;
        m_values = {
            {QStringLiteral("leftThumbnailsEnabled"),
             settings.value(QStringLiteral("Panels/leftThumbnailsEnabled"),
                            true).toBool()},
            {QStringLiteral("rightThumbnailsEnabled"),
             settings.value(QStringLiteral("Panels/rightThumbnailsEnabled"),
                            true).toBool()},
        };
    }

    QVariantMap values() const { return m_values; }
    QString error() const { return m_error; }

    bool thumbnailsEnabled(int side) const
    {
        const QString key = keyForSide(side);
        return !key.isEmpty() && m_values.value(key, true).toBool();
    }

    bool setThumbnailsEnabled(int side, bool enabled)
    {
        const QString key = keyForSide(side);
        if (key.isEmpty()) {
            m_error = tr("Could not save panel settings: invalid panel.");
            emit changed();
            return false;
        }

        QSettings settings;
        const QString settingsKey = QStringLiteral("Panels/") + key;
        settings.setValue(settingsKey, enabled);
        settings.sync();
        if (settings.status() != QSettings::NoError) {
            m_error = tr("Could not save the Thumbnails preference.");
            emit changed();
            return false;
        }

        m_error.clear();
        m_values.insert(key, enabled);
        emit changed();
        return true;
    }

signals:
    void changed();

private:
    static QString keyForSide(int side)
    {
        if (side == 0)
            return QStringLiteral("leftThumbnailsEnabled");
        if (side == 1)
            return QStringLiteral("rightThumbnailsEnabled");
        return {};
    }

    QVariantMap m_values;
    QString m_error;
};
