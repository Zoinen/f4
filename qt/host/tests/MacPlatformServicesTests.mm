#include <QtTest>

#include <QDir>
#include <QFile>
#include <QSet>
#include <QTemporaryDir>

// Keep the Foundation discovery helpers in the same Objective-C++ translation
// unit so deterministic fixtures exercise the exact production code rather
// than a second test-only implementation.
#include "../src/MacPlatformServices.mm"

class MacPlatformServicesTests final : public QObject {
  Q_OBJECT

private slots:
  void pathCandidatesDeduplicateAndIgnoreMissingEntries();
  void cloudCandidatesSkipHiddenFilesAndNonDirectories();
  void finderTagMetadataUsesStableNamesAndColors();
  void finderTagRowsOpenDedicatedVirtualDirectories();
  void metadataQueriesUseSpotlightCompatiblePredicates();
  void unsupportedRequestReturnsStructuredFinalError();
  void mountRejectsCredentialsBeforeOpeningNativeUI();
  void liveLocationRequestStartsWithOrderedDynamicSnapshot();
};

void MacPlatformServicesTests::
    pathCandidatesDeduplicateAndIgnoreMissingEntries() {
  QTemporaryDir fixture;
  QVERIFY(fixture.isValid());
  const QString existing =
      QDir(fixture.path()).filePath(QStringLiteral("Volume"));
  QVERIFY(QDir().mkpath(existing));

  QVariantList rows;
  std::set<QString> seen;
  @autoreleasepool {
    NSURL *url = [NSURL fileURLWithPath:nsString(existing) isDirectory:YES];
    appendPathRow(rows, seen, QStringLiteral("first"),
                  QStringLiteral("locations"), url,
                  QStringLiteral("hard-drive"));
    appendPathRow(rows, seen, QStringLiteral("duplicate"),
                  QStringLiteral("locations"), url,
                  QStringLiteral("hard-drive"));
    NSURL *missing = [NSURL
        fileURLWithPath:nsString(QDir(fixture.path())
                                     .filePath(QStringLiteral("Disconnected")))
            isDirectory:YES];
    appendPathRow(rows, seen, QStringLiteral("missing"),
                  QStringLiteral("locations"), missing,
                  QStringLiteral("hard-drive"));
  }

  QCOMPARE(rows.size(), 1);
  const QVariantMap row = rows.constFirst().toMap();
  QCOMPARE(row.value(QStringLiteral("id")).toString(), QStringLiteral("first"));
  QCOMPARE(row.value(QStringLiteral("path")).toString(),
           QDir::cleanPath(existing));
}

void MacPlatformServicesTests::
    cloudCandidatesSkipHiddenFilesAndNonDirectories() {
  QTemporaryDir fixture;
  QVERIFY(fixture.isValid());
  const QString visible =
      QDir(fixture.path()).filePath(QStringLiteral("Provider"));
  const QString hidden =
      QDir(fixture.path()).filePath(QStringLiteral(".Hidden"));
  const QString plainFile =
      QDir(fixture.path()).filePath(QStringLiteral("README"));
  QVERIFY(QDir().mkpath(visible));
  QVERIFY(QDir().mkpath(hidden));
  QFile file(plainFile);
  QVERIFY(file.open(QIODevice::WriteOnly));
  file.write("not a provider");
  file.close();

  QVariantList rows;
  std::set<QString> seen;
  @autoreleasepool {
    appendCloudRoots(rows, seen, [NSFileManager defaultManager],
                     [NSURL fileURLWithPath:nsString(fixture.path())
                                isDirectory:YES]);
  }
  QCOMPARE(rows.size(), 1);
  QCOMPARE(
      canonicalPathKey(
          rows.constFirst().toMap().value(QStringLiteral("path")).toString()),
      canonicalPathKey(visible));

  // A mounted-volume candidate wins path deduplication over a cloud alias.
  rows.clear();
  seen.insert(canonicalPathKey(visible));
  @autoreleasepool {
    appendCloudRoots(rows, seen, [NSFileManager defaultManager],
                     [NSURL fileURLWithPath:nsString(fixture.path())
                                isDirectory:YES]);
  }
  QVERIFY(rows.isEmpty());
}

void MacPlatformServicesTests::finderTagMetadataUsesStableNamesAndColors() {
  QCOMPARE(finderTagName(@"Project\n6"), QStringLiteral("Project"));
  QCOMPARE(finderTagColor(@"Red\n6"), QStringLiteral("#ff7b72"));
  QCOMPARE(finderTagColor(@"Orange\n7"), QStringLiteral("#f0883e"));
  QCOMPARE(finderTagColor(@"No color"), QStringLiteral("#b7bac0"));
}

void MacPlatformServicesTests::
    finderTagRowsOpenDedicatedVirtualDirectories() {
  const QVariantMap row = finderTagLocationRow(@"Project Files\n6");
  QCOMPARE(row.value(QStringLiteral("section")).toString(),
           QStringLiteral("tags"));
  QCOMPARE(row.value(QStringLiteral("kind")).toString(),
           QStringLiteral("query"));
  QCOMPARE(row.value(QStringLiteral("label")).toString(),
           QStringLiteral("Project Files"));
  QCOMPARE(row.value(QStringLiteral("queryKind")).toString(),
           QStringLiteral("tag"));
  QCOMPARE(row.value(QStringLiteral("tag")).toString(),
           QStringLiteral("Project Files"));
  QCOMPARE(row.value(QStringLiteral("uri")).toString(),
           QStringLiteral("macos://tag/Project%20Files"));
  QCOMPARE(row.value(QStringLiteral("color")).toString(),
           QStringLiteral("#ff7b72"));
  QVERIFY(!row.value(QStringLiteral("id")).toString().isEmpty());
}

void MacPlatformServicesTests::
    metadataQueriesUseSpotlightCompatiblePredicates() {
  const MetadataMode modes[] = {
      MetadataMode::Recents, MetadataMode::Shared,       MetadataMode::Tag,
      MetadataMode::AllTags, MetadataMode::TagDiscovery,
  };
  for (const MetadataMode mode : modes) {
    bool receivedError = false;
    NSException *caughtException = nil;
    QString predicateFormat;
    QStringList searchScopes;
    @autoreleasepool {
      F4MetadataOperation *operation = [[F4MetadataOperation alloc]
          initWithMode:mode
          tag:mode == MetadataMode::Tag ? @"Red" : nil
          live:NO
          send:[&receivedError](const QVariantList &, bool, NSError *error) {
            receivedError = receivedError || error != nil;
          }
          event:[]() {}
          done:[]() {}];
      @try {
        [operation start];
        NSMetadataQuery *query = [operation valueForKey:@"query"];
        predicateFormat = qString(query.predicate.predicateFormat);
        for (NSString *scope in query.searchScopes) {
          searchScopes.append(qString(scope));
        }
      } @catch (NSException *exception) {
        caughtException = exception;
      }
      [operation cancel];
#if !__has_feature(objc_arc)
      [operation release];
#endif
    }
    QVERIFY2(caughtException == nil,
             qPrintable(caughtException ? qString(caughtException.reason)
                                        : QString()));
    QVERIFY(!receivedError);
    QVERIFY(!predicateFormat.contains(qString(NSMetadataItemPathKey)));
    QVERIFY(!predicateFormat.contains(QStringLiteral("nil")));
    if (mode == MetadataMode::Shared) {
      QVERIFY(predicateFormat.contains(
          qString(NSMetadataUbiquitousItemIsSharedKey)));
      QVERIFY(!predicateFormat.contains(QStringLiteral("kMDItemUserShared")));
    }
    QCOMPARE(searchScopes,
             QStringList({qString(NSMetadataQueryLocalComputerScope),
                          qString(NSMetadataQueryNetworkScope)}));
  }
}

void MacPlatformServicesTests::unsupportedRequestReturnsStructuredFinalError() {
  QList<QVariantMap> messages;
  MacPlatformServices services(
      [&messages](const QVariantMap &message) { messages.append(message); });
  services.handleMessage({
      {QStringLiteral("type"), QStringLiteral("platform_request")},
      {QStringLiteral("requestId"), QStringLiteral("unsupported-1")},
      {QStringLiteral("operation"), QStringLiteral("macos.private")},
  });

  QCOMPARE(messages.size(), 1);
  const QVariantMap response = messages.constFirst();
  QCOMPARE(response.value(QStringLiteral("type")).toString(),
           QStringLiteral("platform_response"));
  QCOMPARE(response.value(QStringLiteral("requestId")).toString(),
           QStringLiteral("unsupported-1"));
  QVERIFY(response.value(QStringLiteral("final")).toBool());
  QCOMPARE(response.value(QStringLiteral("error"))
               .toMap()
               .value(QStringLiteral("code"))
               .toString(),
           QStringLiteral("unsupported"));
}

void MacPlatformServicesTests::mountRejectsCredentialsBeforeOpeningNativeUI() {
  QList<QVariantMap> messages;
  MacPlatformServices services(
      [&messages](const QVariantMap &message) { messages.append(message); });
  services.handleMessage({
      {QStringLiteral("type"), QStringLiteral("platform_request")},
      {QStringLiteral("requestId"), QStringLiteral("mount-secret")},
      {QStringLiteral("operation"), QStringLiteral("macos.mount")},
      {QStringLiteral("payload"),
       QVariantMap{
           {QStringLiteral("url"),
            QStringLiteral("smb://alice:secret@example.invalid/share")},
       }},
  });

  QCOMPARE(messages.size(), 1);
  const QVariantMap response = messages.constFirst();
  QCOMPARE(response.value(QStringLiteral("error"))
               .toMap()
               .value(QStringLiteral("code"))
               .toString(),
           QStringLiteral("invalid_url"));
  QVERIFY(!response.value(QStringLiteral("error"))
               .toMap()
               .value(QStringLiteral("message"))
               .toString()
               .contains(QStringLiteral("secret")));
}

void MacPlatformServicesTests::
    liveLocationRequestStartsWithOrderedDynamicSnapshot() {
  QList<QVariantMap> messages;
  MacPlatformServices services(
      [&messages](const QVariantMap &message) { messages.append(message); });
  services.handleMessage({
      {QStringLiteral("type"), QStringLiteral("platform_request")},
      {QStringLiteral("requestId"), QStringLiteral("locations-1")},
      {QStringLiteral("operation"), QStringLiteral("macos.locations")},
  });

  QElapsedTimer waitTimer;
  waitTimer.start();
  while (messages.isEmpty() && waitTimer.elapsed() < 5000) {
    @autoreleasepool {
      [[NSRunLoop currentRunLoop]
             runMode:NSDefaultRunLoopMode
          beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.02]];
    }
  }
  QVERIFY(!messages.isEmpty());
  const QVariantMap response = messages.constFirst();
  QCOMPARE(response.value(QStringLiteral("type")).toString(),
           QStringLiteral("platform_response"));
  QCOMPARE(response.value(QStringLiteral("requestId")).toString(),
           QStringLiteral("locations-1"));
  QVERIFY(response.value(QStringLiteral("chunk")).toBool());
  QVERIFY(!response.value(QStringLiteral("final")).toBool());
  const QVariantList rows = response.value(QStringLiteral("payload"))
                                .toMap()
                                .value(QStringLiteral("items"))
                                .toList();
  QVERIFY(rows.size() >= 6);
  QCOMPARE(rows.at(0).toMap().value(QStringLiteral("id")).toString(),
           QStringLiteral("recents"));
  QCOMPARE(rows.at(1).toMap().value(QStringLiteral("id")).toString(),
           QStringLiteral("shared"));

  const QHash<QString, int> sectionRank = {
      {QStringLiteral("top"), 0},
      {QStringLiteral("favorites"), 1},
      {QStringLiteral("locations"), 2},
  };
  int previousRank = -1;
  QSet<QString> paths;
  bool sawHome = false;
  bool sawRoot = false;
  bool sawNetwork = false;
  bool sawAirDrop = false;
  for (const QVariant &value : rows) {
    const QVariantMap row = value.toMap();
    const int rank = sectionRank.value(
        row.value(QStringLiteral("section")).toString(), previousRank);
    QVERIFY(rank >= previousRank);
    previousRank = rank;
    const QString path = row.value(QStringLiteral("path")).toString();
    if (!path.isEmpty()) {
      QVERIFY2(!paths.contains(path), qPrintable(path));
      paths.insert(path);
      QVERIFY(QFileInfo::exists(path));
    }
    const QString id = row.value(QStringLiteral("id")).toString();
    sawHome = sawHome || id == QStringLiteral("home");
    sawRoot = sawRoot || id == QStringLiteral("root-volume");
    sawNetwork = sawNetwork || id == QStringLiteral("network");
    sawAirDrop = sawAirDrop || id == QStringLiteral("airdrop");
  }
  QVERIFY(sawHome);
  QVERIFY(sawRoot);
  QVERIFY(sawNetwork);
  QVERIFY(sawAirDrop);

  services.handleMessage({
      {QStringLiteral("type"), QStringLiteral("platform_cancel")},
      {QStringLiteral("requestId"), QStringLiteral("locations-1")},
      {QStringLiteral("operation"), QStringLiteral("macos.locations")},
  });
}

QTEST_GUILESS_MAIN(MacPlatformServicesTests)

#include "MacPlatformServicesTests.moc"
