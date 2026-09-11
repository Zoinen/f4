#include "SemanticChildrenModel.h"

#include <QSet>

namespace {
QString identity(const QVariant &value, int position)
{
    const auto widget = value.toMap();
    const auto id = widget.value("id").toString();
    return widget.value("kind").toString() + QChar(0x1f)
        + (id.isEmpty() ? QString::number(position) : id);
}
// Terminal coordinates describe adjacency, but the native field follows the
// measured caption. Project just that relationship, not all siblings into
// every delegate (a sibling can itself contain an entire settings page).
QVariantMap inlineLabel(const QVariantList &widgets, const QVariantMap &widget)
{
    if (widget.value("kind") != "edit" && widget.value("kind") != "comboBox") return {};
    QVariantMap previous;
    const int x = widget.value("x").toInt(), y = widget.value("y").toInt();
    for (const auto &value : widgets) {
        const auto candidate = value.toMap();
        if (candidate.value("visible", true) == false || candidate.value("y").toInt() != y
            || candidate.value("x").toInt() >= x) continue;
        if (previous.isEmpty() || candidate.value("x").toInt() > previous.value("x").toInt())
            previous = candidate;
    }
    const int gap = x - previous.value("x").toInt() - previous.value("w", 1).toInt();
    return previous.value("kind") == "text" && previous.value("h", 1) == 1
        && gap >= 0 && gap <= 3 ? previous : QVariantMap{};
}
}


int SemanticChildrenModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : m_widgets.size();
}

QVariant SemanticChildrenModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() < 0 || index.row() >= m_widgets.size())
        return {};
    if (role == Qt::UserRole) return m_widgets[index.row()];
    if (role == Qt::UserRole + 1) return m_labels[index.row()];
    return {};
}

QHash<int, QByteArray> SemanticChildrenModel::roleNames() const
{
    return {{Qt::UserRole, "widgetData"}, {Qt::UserRole + 1, "labelData"}};
}

void SemanticChildrenModel::setWidgets(const QVariantList &widgets)
{
    if (widgets == m_widgets)
        return;
    QStringList keys;
    QSet<QString> wanted;
    for (int row = 0; row < widgets.size(); ++row)
        wanted.insert(identity(widgets[row], row));
    for (int row = 0; row < m_widgets.size(); ++row)
        keys.append(identity(m_widgets[row], row));
    for (int row = keys.size() - 1; row >= 0; --row) {
        if (!wanted.contains(keys[row])) {
            beginRemoveRows({}, row, row);
            m_widgets.removeAt(row);
            m_labels.removeAt(row);
            keys.removeAt(row);
            endRemoveRows();
        }
    }
    for (int row = 0; row < widgets.size(); ++row) {
        const auto key = identity(widgets[row], row);
        const auto label = inlineLabel(widgets, widgets[row].toMap());
        const int previous = keys.indexOf(key, row);
        if (previous < 0) {
            beginInsertRows({}, row, row);
            m_widgets.insert(row, widgets[row]);
            m_labels.insert(row, label);
            keys.insert(row, key);
            endInsertRows();
        } else {
            if (previous != row) {
                beginMoveRows({}, previous, previous, {}, row);
                m_widgets.move(previous, row);
                m_labels.move(previous, row);
                keys.move(previous, row);
                endMoveRows();
            }
            QList<int> roles;
            if (m_widgets[row] != widgets[row]) {
                m_widgets[row] = widgets[row];
                roles.append(Qt::UserRole);
            }
            if (m_labels[row] != label) {
                m_labels[row] = label;
                roles.append(Qt::UserRole + 1);
            }
            if (!roles.isEmpty()) emit dataChanged(index(row), index(row), roles);
        }
    }
    emit widgetsChanged();
}
