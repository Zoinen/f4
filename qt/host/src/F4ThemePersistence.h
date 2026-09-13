#pragma once

#include <QObject>
#include <QString>
#include <QStringList>
#include <QVariantMap>

class F4ThemePersistence : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString themeFilePath READ themeFilePath CONSTANT)

public:
    explicit F4ThemePersistence(QObject *parent = nullptr);
    F4ThemePersistence(const QString &filePath, const QString &legacyPath,
                       QObject *parent = nullptr);

    QString themeFilePath() const;

    Q_INVOKABLE QVariantMap loadTheme() const;
    Q_INVOKABLE QStringList modernTerminalColors() const;
    Q_INVOKABLE bool saveTheme(const QVariantMap &colors);
    QString lastError() const { return m_lastError; }

private:
    QString m_themeFilePath;
    QString m_legacyPath;
    mutable QString m_lastError;
};
