#pragma once

#include <QAbstractListModel>
#include <QVariantList>
#include <QtQmlIntegration/qqmlintegration.h>

// The live children of one semantic node. Changes are announced by identity,
// so a new snapshot does not discard native controls, focus or scroll state.
class SemanticChildrenModel : public QAbstractListModel
{
    Q_OBJECT
    QML_ELEMENT
    Q_PROPERTY(QVariantList widgets READ widgets WRITE setWidgets NOTIFY widgetsChanged)
public:
    explicit SemanticChildrenModel(QObject *parent = nullptr) : QAbstractListModel(parent) {}
    int rowCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;
    QVariantList widgets() const { return m_widgets; }
    void setWidgets(const QVariantList &widgets);
signals:
    void widgetsChanged();
private:
    QVariantList m_widgets;
    QVariantList m_labels;
};
