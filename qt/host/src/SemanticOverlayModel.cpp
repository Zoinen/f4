#include "SemanticOverlayModel.h"
#include <QSet>

namespace {
QString frameKey(const QVariantMap &frame)
{
    return frame.value("kind").toString() + "|" + frame.value("role").toString()
        + "|" + frame.value("id").toString();
}
}
int SemanticOverlayModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : m_rows.size();
}
QVariant SemanticOverlayModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() < 0 || index.row() >= m_rows.size()) return {};
    const auto &row = m_rows[index.row()];
    if (role == Qt::UserRole) return row.frame;
    if (role == Qt::UserRole + 1) return row.closing;
    return {};
}
QHash<int, QByteArray> SemanticOverlayModel::roleNames() const
{
    return {{Qt::UserRole, "modelFrame"}, {Qt::UserRole + 1, "isClosing"}};
}
void SemanticOverlayModel::setFrames(const QVariantList &frames)
{
    if (frames == m_frames) return;
    m_frames = frames;
    QSet<QString> wanted;
    for (const auto &frame : frames) wanted.insert(frameKey(frame.toMap()));
    for (int row = m_rows.size() - 1; row >= 0; --row) {
        if (wanted.contains(m_rows[row].key)) continue;
        if (m_rows[row].frame.value("presentation") == "dropdown") {
            if (!m_rows[row].closing) {
                m_rows[row].closing = true;
                emit dataChanged(index(row), index(row), {Qt::UserRole + 1});
            }
        } else {
            beginRemoveRows({}, row, row);
            m_rows.removeAt(row);
            endRemoveRows();
        }
    }
    for (int row = 0; row < frames.size(); ++row) {
        const auto frame = frames[row].toMap();
        const auto key = frameKey(frame);
        int previous = row;
        while (previous < m_rows.size() && m_rows[previous].key != key) ++previous;
        if (previous == m_rows.size()) {
            beginInsertRows({}, row, row);
            m_rows.insert(row, Row{key, frame, false});
            endInsertRows();
            continue;
        }
        if (previous != row) {
            beginMoveRows({}, previous, previous, {}, row);
            m_rows.move(previous, row);
            endMoveRows();
        }
        QList<int> roles;
        if (m_rows[row].frame != frame) {
            m_rows[row].frame = frame;
            roles.append(Qt::UserRole);
        }
        if (m_rows[row].closing) {
            m_rows[row].closing = false;
            roles.append(Qt::UserRole + 1);
        }
        if (!roles.isEmpty()) emit dataChanged(index(row), index(row), roles);
    }
    emit framesChanged();
}
void SemanticOverlayModel::finishExit(const QString &key)
{
    for (int row = 0; row < m_rows.size(); ++row) {
        if (m_rows[row].key != key || !m_rows[row].closing) continue;
        beginRemoveRows({}, row, row);
        m_rows.removeAt(row);
        endRemoveRows();
        return;
    }
}
