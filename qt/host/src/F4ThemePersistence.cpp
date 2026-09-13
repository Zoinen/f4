#include "F4ThemePersistence.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QSettings>
#include <QFileInfo>
#include <QSaveFile>
#include <QTemporaryDir>
#include <QColor>

QStringList F4ThemePersistence::modernTerminalColors() const
{
    // Snapshot of the configured console palette; COLORREF stores BGR.
    QStringList colors{"#000000", "#212421", "#68a318", "#00508c",
        "#800000", "#aaf046", "#ffdc00", "#ece9d8", "#8c8c8c",
        "#0082ff", "#00ff00", "#aff7ff", "#ff8c00", "#c864ff", "#beb9ff", "#ffffff"};
#ifdef Q_OS_WIN
    QSettings console(QStringLiteral("HKEY_CURRENT_USER\\Console"), QSettings::NativeFormat);
    for (int index = 0; index < 16; ++index) {
        const QString key = QStringLiteral("ColorTable%1").arg(index, 2, 10, QLatin1Char('0'));
        bool ok = false;
        const auto value = console.value(key).toUInt(&ok);
        if (ok) colors[index] = QColor(value & 255, (value >> 8) & 255,
                                      (value >> 16) & 255).name();
    }
#endif
    return colors;
}

F4ThemePersistence::F4ThemePersistence(QObject *parent)
    : QObject(parent)
{
    const QDir appDir(QCoreApplication::applicationDirPath());
    m_themeFilePath = appDir.filePath(QStringLiteral("gui_theme.ini"));
}

F4ThemePersistence::F4ThemePersistence(const QString &filePath,
                                     const QString &legacyPath, QObject *parent)
    : QObject(parent), m_themeFilePath(filePath), m_legacyPath(legacyPath)
{
}

QString F4ThemePersistence::themeFilePath() const
{
    return m_themeFilePath;
}

QVariantMap F4ThemePersistence::loadTheme() const
{
    QVariantMap result;
    QString source = m_themeFilePath;
    if (!QFile::exists(source) && QFile::exists(m_legacyPath)) {
        source = m_legacyPath;
        QFile legacy(source);
        if (legacy.open(QIODevice::ReadOnly)
            && QDir().mkpath(QFileInfo(m_themeFilePath).absolutePath())) {
            QSaveFile target(m_themeFilePath);
            const QByteArray data = legacy.readAll();
            if (target.open(QIODevice::WriteOnly)
                && target.write(data) == data.size() && target.commit()) {
                source = m_themeFilePath;
            } else {
                m_lastError = QStringLiteral("Unable to import legacy GUI preferences: ")
                    + target.errorString();
            }
        } else {
            m_lastError = QStringLiteral("Unable to import legacy GUI preferences");
        }
    }
    if (!QFile::exists(source)) {
        return result;
    }
    QSettings settings(source, QSettings::IniFormat);
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
    m_lastError.clear();
    const QString directory = QFileInfo(m_themeFilePath).absolutePath();
    if (!QDir().mkpath(directory)) {
        m_lastError = QStringLiteral("Cannot create GUI preferences directory: ") + directory;
        return false;
    }
    QTemporaryDir staging(QDir(directory).filePath(QStringLiteral(".gui-theme-XXXXXX")));
    if (!staging.isValid()) { m_lastError = staging.errorString(); return false; }
    const QString stagingFile = staging.filePath("preferences.ini");
    // Preserve unrelated sections and unknown forward-compatible keys.
    if (QFile::exists(m_themeFilePath) && !QFile::copy(m_themeFilePath, stagingFile)) {
        m_lastError = QStringLiteral("Cannot read existing GUI preferences");
        return false;
    }
    {
        QSettings settings(stagingFile, QSettings::IniFormat);
        settings.beginGroup(QStringLiteral("gui_theme"));
        for (auto it = colors.constBegin(); it != colors.constEnd(); ++it)
            settings.setValue(it.key(), it.value().toString());
        settings.endGroup();
        settings.sync();
        if (settings.status() != QSettings::NoError) {
            m_lastError = QStringLiteral("Cannot encode GUI preferences"); return false;
        }
    }
    QFile encoded(stagingFile);
    if (!encoded.open(QIODevice::ReadOnly)) { m_lastError = encoded.errorString(); return false; }
    const QByteArray data = encoded.readAll();
    QSaveFile target(m_themeFilePath);
    if (!target.open(QIODevice::WriteOnly) || target.write(data) != data.size() || !target.commit()) {
        m_lastError = target.errorString(); return false;
    }
    return true;
}
