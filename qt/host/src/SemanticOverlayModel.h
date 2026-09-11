#pragma once
#include <QAbstractListModel>
#include <QVariantList>
#include <QtQmlIntegration/qqmlintegration.h>

// Native overlay payloads stay QVariant trees; QML ListModel recursively turns
// nested maps/lists into another model tree on each snapshot assignment.
class SemanticOverlayModel : public QAbstractListModel
{
    Q_OBJECT
    QML_ELEMENT
    Q_PROPERTY(QVariantList frames READ frames WRITE setFrames NOTIFY framesChanged)
public:
    explicit SemanticOverlayModel(QObject *parent = nullptr) : QAbstractListModel(parent) {}
    int rowCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;
    QVariantList frames() const { return m_frames; }
    void setFrames(const QVariantList &frames);
    Q_INVOKABLE void finishExit(const QString &key);
signals:
    void framesChanged();
private:
    struct Row { QString key; QVariantMap frame; bool closing = false; };
    QList<Row> m_rows;
    QVariantList m_frames;
};
