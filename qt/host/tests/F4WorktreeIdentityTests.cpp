#include "F4WorktreeIdentity.h"

#include <QDir>
#include <QFile>
#include <QTemporaryDir>
#include <QtTest>

namespace
{
bool writeFile(const QString &path, const QByteArray &contents)
{
    QFile file(path);
    if (!file.open(QIODevice::WriteOnly))
        return false;
    return file.write(contents) == contents.size();
}

class F4WorktreeIdentityTests final : public QObject
{
    Q_OBJECT

private slots:
    void resolvesLinkedWorktreeBranch()
    {
        QTemporaryDir fixture;
        QVERIFY(fixture.isValid());

        const QString checkout = fixture.filePath(QStringLiteral("checkout"));
        const QString nested = QDir(checkout).filePath(QStringLiteral("qt/host"));
        const QString gitDirectory = QDir(fixture.path()).filePath(
            QStringLiteral("worktree-metadata"));
        QVERIFY(QDir().mkpath(nested));
        QVERIFY(QDir().mkpath(gitDirectory));
        QVERIFY(writeFile(QDir(checkout).filePath(QStringLiteral(".git")),
                          QByteArrayLiteral("gitdir: ../worktree-metadata\n")));
        QVERIFY(writeFile(QDir(gitDirectory).filePath(QStringLiteral("HEAD")),
                          QByteArrayLiteral("ref: refs/heads/feature/worktree-test\n")));

        QCOMPARE(F4WorktreeIdentity::resolveActiveWorktreeBranch({nested}),
                 QStringLiteral("feature/worktree-test"));
    }

    void ignoresDetachedAndNormalRepositories()
    {
        QTemporaryDir fixture;
        QVERIFY(fixture.isValid());

        const QString normal = fixture.filePath(QStringLiteral("normal"));
        QVERIFY(QDir().mkpath(QDir(normal).filePath(QStringLiteral(".git"))));
        QVERIFY(QDir().mkpath(QDir(normal).filePath(QStringLiteral("src"))));
        QCOMPARE(F4WorktreeIdentity::resolveActiveWorktreeBranch(
                     {QDir(normal).filePath(QStringLiteral("src"))}),
                 QString{});

        const QString detached = fixture.filePath(QStringLiteral("detached"));
        const QString gitDirectory = QDir(detached).filePath(QStringLiteral("metadata"));
        QVERIFY(QDir().mkpath(gitDirectory));
        QVERIFY(writeFile(QDir(detached).filePath(QStringLiteral(".git")),
                          QByteArrayLiteral("gitdir: metadata\n")));
        QVERIFY(writeFile(QDir(gitDirectory).filePath(QStringLiteral("HEAD")),
                          QByteArrayLiteral("0123456789abcdef\n")));
        QCOMPARE(F4WorktreeIdentity::resolveActiveWorktreeBranch({detached}),
                 QString{});
    }

    void stopsAtTheActiveNormalRepository()
    {
        QTemporaryDir fixture;
        QVERIFY(fixture.isValid());

        const QString normal = fixture.filePath(QStringLiteral("normal"));
        const QString linked = fixture.filePath(QStringLiteral("linked"));
        QVERIFY(QDir().mkpath(QDir(normal).filePath(QStringLiteral(".git"))));
        QVERIFY(QDir().mkpath(linked));
        const QString gitDirectory = QDir(fixture.path()).filePath(
            QStringLiteral("linked-metadata"));
        QVERIFY(QDir().mkpath(gitDirectory));
        QVERIFY(writeFile(QDir(linked).filePath(QStringLiteral(".git")),
                          QByteArrayLiteral("gitdir: ../linked-metadata\n")));
        QVERIFY(writeFile(QDir(gitDirectory).filePath(QStringLiteral("HEAD")),
                          QByteArrayLiteral("ref: refs/heads/other\n")));

        QCOMPARE(F4WorktreeIdentity::resolveActiveWorktreeBranch(
                     {QDir(normal).filePath(QStringLiteral("src")), linked}),
                 QString{});
    }
};
}

QTEST_MAIN(F4WorktreeIdentityTests)
#include "F4WorktreeIdentityTests.moc"
