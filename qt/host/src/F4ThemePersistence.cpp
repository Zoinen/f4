#include "F4ThemePersistence.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QSettings>

F4ThemePersistence::F4ThemePersistence(QObject *parent)
    : QObject(parent)
{
    const QDir appDir(QCoreApplication::applicationDirPath());
    m_themeFilePath = appDir.filePath(QStringLiteral("gui_theme.ini"));
}

QString F4ThemePersistence::themeFilePath() const
{
    return m_themeFilePath;
}

QVariantMap F4ThemePersistence::loadTheme() const
{
    QVariantMap result;
    if (!QFile::exists(m_themeFilePath)) {
        return result;
    }
    QSettings settings(m_themeFilePath, QSettings::IniFormat);
    settings.beginGroup(QStringLiteral("gui_theme"));
    const QStringList keys = settings.childKeys();
    for (const QString &key : keys) {
        result.insert(key, settings.value(key).toString());
    }
    settings.endGroup();
    return result;
}

bool F4ThemePersistence::saveTheme(const QVariantMap &colors)
{
    QSettings settings(m_themeFilePath, QSettings::IniFormat);
    settings.beginGroup(QStringLiteral("gui_theme"));
    for (auto it = colors.constBegin(); it != colors.constEnd(); ++it) {
        settings.setValue(it.key(), it.value().toString());
    }
    settings.endGroup();
    settings.sync();
    return settings.status() == QSettings::NoError;
}

bool F4ThemePersistence::resetTheme()
{
    if (QFile::exists(m_themeFilePath)) {
        QSettings settings(m_themeFilePath, QSettings::IniFormat);
        settings.remove(QStringLiteral("gui_theme"));
        settings.sync();
    }
    return true;
}
