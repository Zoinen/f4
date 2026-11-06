#include <QCoreApplication>
#include <QDebug>
#include <QFile>
#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSet>
#include <QStringList>

// Core-only: exercise the compiled resources without provider or GUI state.
int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);
    const QString root = QStringLiteral(":/F4QtHost/icons/");
    auto read = [](const QString &path) {
        QFile file(path);
        return file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray();
    };
    auto equal = [&](const QString &alias, const QString &source) {
        const auto bytes = read(source);
        if (bytes.isEmpty() || read(root + alias) != bytes) {
            qCritical() << "Missing or changed artwork:" << alias << source;
            return false;
        }
        return true;
    };
    const auto zoin = QStringLiteral(
        "bash=shell-scripts c=c-source claude=claude cpp=cpp-source css=stylesheets "
        "database=databases default=files docker=docker folder-up=parent-folder "
        "folder=folders font=fonts git=git go=go-source html=web-pages image=images "
        "javascript=javascript json=json markdown=markdown npm=npm python=python "
        "ruby=ruby rust=rust svg=vector-images swift=swift table=tables text=text-files "
        "typescript=typescript yml=yaml zip=archives react=react").split(' ');
    QSet<QString> expected;
    for (const auto &pair : zoin) {
        const auto parts = pair.split('=');
        const auto canonical = QStringLiteral("zoin/%1.svg").arg(parts.at(1));
        expected.insert(parts.at(1) + QStringLiteral(".svg"));
        const auto source = QStringLiteral(F4_ICON_SOURCE_DIR "/") + canonical;
        if (!equal(canonical, source)
            || !equal(QStringLiteral("zoin/%1.svg").arg(parts.at(0)), source))
            return 1;
    }
    const auto manifest = QJsonDocument::fromJson(read(root + "zoin/manifest.json"));
    QSet<QString> actual;
    for (const auto &entry : manifest.object().value("icons").toArray())
        actual.insert(entry.toObject().value("file").toString());
    if (actual != expected || !equal("zoin/ICON_LICENSES.txt",
                                    QStringLiteral(F4_ICON_SOURCE_DIR "/zoin/ICON_LICENSES.txt")))
        return 2;
    const auto lucide = QStringLiteral(
        "databases=database files=file parent-folder=folder-up folders=folder images=image "
        "text-files=file-text archives=file-archive "
        "executables=file-terminal audio=music video=video source-code=file-code "
        "books=book-open configuration=file-cog locked-files=file-lock "
        "temporary-files=file-clock").split(' ');
    QHash<QString, QString> outputNames;
    for (const auto &pair : lucide) {
        const auto parts = pair.split('=');
        outputNames.insert(parts.at(1), parts.at(0));
    }
    const auto mappingLines = QString::fromUtf8(read(
        QStringLiteral(F4_ICON_SOURCE_DIR "/files/group-names.txt")))
        .split('\n', Qt::SkipEmptyParts);
    if (QSet<QString>(mappingLines.begin(), mappingLines.end())
        != QSet<QString>(lucide.begin(), lucide.end()))
        return 7;
    const auto legacyNames = QString::fromUtf8(read(
        QStringLiteral(F4_ICON_SOURCE_DIR "/files/names.txt")))
        .split('\n', Qt::SkipEmptyParts);
    if (legacyNames.size() != 32)
        return 4;
    for (int step = 1; step <= 8; ++step) {
        if (QFile::exists(root + QStringLiteral("files/%1/shell-scripts.svg").arg(step)))
            return 6;
        for (const auto &name : legacyNames) {
            const auto old = name.trimmed();
            const auto prefix = QStringLiteral("files/%1/").arg(step);
            const auto canonical = prefix + outputNames.value(old, old) + ".svg";
            if (!equal(prefix + old + ".svg", QStringLiteral(F4_ICON_SOURCE_DIR "/") + canonical)
                || !equal(canonical, QStringLiteral(F4_ICON_SOURCE_DIR "/") + canonical))
                return 5;
        }
        for (const auto &pair : lucide) {
            const auto parts = pair.split('=');
            const auto prefix = QStringLiteral("files/%1/").arg(step);
            const auto source = QStringLiteral(F4_ICON_SOURCE_DIR "/")
                + prefix + parts.at(0) + ".svg";
            if (parts.at(0) != parts.at(1)
                && QFile::exists(QStringLiteral(F4_ICON_SOURCE_DIR "/")
                                 + prefix + parts.at(1) + ".svg"))
                return 8;
            if (!equal(prefix + parts.at(0) + ".svg", source)
                || !equal(prefix + parts.at(1) + ".svg", source))
                return 3;
        }
    }
    qInfo() << "Verified 30 canonical Zoin icons, legacy URLs, manifest/licenses "
               "and 15 Lucide group keys at all eight stroke widths.";
    return 0;
}
