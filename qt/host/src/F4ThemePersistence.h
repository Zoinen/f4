#pragma once

#include <QObject>
#include <QString>
#include <QVariantMap>

class F4ThemePersistence : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString themeFilePath READ themeFilePath CONSTANT)

public:
    explicit F4ThemePersistence(QObject *parent = nullptr);

    QString themeFilePath() const;

    Q_INVOKABLE QVariantMap loadTheme() const;
    Q_INVOKABLE bool saveTheme(const QVariantMap &colors);
    Q_INVOKABLE bool resetTheme();

private:
    QString m_themeFilePath;
};
