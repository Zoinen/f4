#pragma once

#include <QObject>
#include <QString>
#include <QVariantList>

class F4TextRenderingPolicy final : public QObject {
    Q_OBJECT
    Q_PROPERTY(int renderType READ renderType WRITE setRenderType NOTIFY renderTypeChanged)
    Q_PROPERTY(QString renderTypeName READ renderTypeName NOTIFY renderTypeChanged)
    Q_PROPERTY(QVariantList options READ options CONSTANT)

public:
    explicit F4TextRenderingPolicy(QObject *parent = nullptr);

    int renderType() const;
    void setRenderType(int renderType);
    QString renderTypeName() const;
    QVariantList options() const;

    Q_INVOKABLE bool setRenderTypeByName(const QString &value);

signals:
    void renderTypeChanged();

private:
    void applyToExistingItems() const;
    bool isSupported(int renderType) const;

    int m_renderType;
};
