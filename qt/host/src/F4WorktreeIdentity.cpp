#include "F4WorktreeIdentity.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>

namespace
{
enum class CheckoutProbe
{
    NotFound,
    NormalRepository,
    Worktree,
};

QString branchFromGitDirectory(const QString &gitDirectory)
{
    QFile headFile(QDir(gitDirectory).filePath(QStringLiteral("HEAD")));
    if (!headFile.open(QIODevice::ReadOnly))
        return {};

    const QString head = QString::fromUtf8(headFile.readAll()).trimmed();
    const QString refPrefix = QStringLiteral("ref: refs/heads/");
    if (!head.startsWith(refPrefix))
        return {};

    const QString branch = head.mid(refPrefix.size()).trimmed();
    return branch.isEmpty() ? QString{} : branch;
}

QString branchFromWorktreeFile(const QFileInfo &gitFile)
{
    QFile file(gitFile.filePath());
    if (!file.open(QIODevice::ReadOnly))
        return {};

    const QString description = QString::fromUtf8(file.readAll()).trimmed();
    const QString prefix = QStringLiteral("gitdir:");
    if (!description.startsWith(prefix, Qt::CaseInsensitive))
        return {};

    QString gitDirectory = description.mid(prefix.size()).trimmed();
    if (gitDirectory.isEmpty())
        return {};
    if (QDir::isRelativePath(gitDirectory))
        gitDirectory = gitFile.dir().absoluteFilePath(gitDirectory);

    return branchFromGitDirectory(QDir::cleanPath(gitDirectory));
}

CheckoutProbe branchForCheckout(const QString &root, QString *branch)
{
    if (root.isEmpty())
        return CheckoutProbe::NotFound;

    QFileInfo currentInfo(root);
    QString current = currentInfo.isDir() ? currentInfo.absoluteFilePath()
                                           : currentInfo.absolutePath();
    current = QDir::cleanPath(current);

    for (;;) {
        const QFileInfo gitEntry(QDir(current).filePath(QStringLiteral(".git")));
        if (gitEntry.isFile()) {
            const QString resolvedBranch = branchFromWorktreeFile(gitEntry);
            if (!resolvedBranch.isEmpty()) {
                *branch = resolvedBranch;
                return CheckoutProbe::Worktree;
            }
            return CheckoutProbe::NormalRepository;
        }
        if (gitEntry.isDir())
            return CheckoutProbe::NormalRepository;

        QDir parent(current);
        if (!parent.cdUp())
            return CheckoutProbe::NotFound;
        const QString parentPath = parent.absolutePath();
        if (parentPath == current)
            return CheckoutProbe::NotFound;
        current = parentPath;
    }
}
}

QString F4WorktreeIdentity::resolveActiveWorktreeBranch(
    const QStringList &searchRoots)
{
    for (const QString &root : searchRoots) {
        QString branch;
        const CheckoutProbe result = branchForCheckout(root, &branch);
        if (result == CheckoutProbe::Worktree)
            return branch;
        if (result == CheckoutProbe::NormalRepository)
            return {};
    }
    return {};
}
