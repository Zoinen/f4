#include "MacPlatformServices.h"

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#import <NetFS/NetFS.h>
#import <Network/Network.h>

#include <QCryptographicHash>
#include <QDir>
#include <QFileInfo>
#include <QUrl>
#include <QVariantList>

#include <algorithm>
#include <map>
#include <memory>
#include <set>
#include <utility>

namespace {
constexpr int metadataChunkSize = 100;

QString qString(NSString *value) {
  return value ? QString::fromUtf8([value UTF8String]) : QString();
}

NSString *nsString(const QString &value) {
  return [NSString stringWithUTF8String:value.toUtf8().constData()];
}

QString stableID(const QString &prefix, const QString &value) {
  const QByteArray digest =
      QCryptographicHash::hash(value.toUtf8(), QCryptographicHash::Sha256)
          .toHex()
          .left(20);
  return prefix + QString::fromLatin1(digest);
}

QString localizedURLName(NSURL *url) {
  if (!url) {
    return {};
  }
  id value = nil;
  if ([url getResourceValue:&value forKey:NSURLLocalizedNameKey error:nil] &&
      [value isKindOfClass:[NSString class]]) {
    return qString(static_cast<NSString *>(value));
  }
  return qString([url lastPathComponent]);
}

QVariantMap locationRow(const QString &id, const QString &section,
                        const QString &kind, const QString &label,
                        const QString &icon, int order = 0) {
  QVariantMap row = {
      {QStringLiteral("id"), id},     {QStringLiteral("section"), section},
      {QStringLiteral("kind"), kind}, {QStringLiteral("label"), label},
      {QStringLiteral("icon"), icon},
  };
  if (order > 0) {
    row.insert(QStringLiteral("order"), order);
  }
  return row;
}

QString canonicalPathKey(const QString &path) {
  const QString canonical = QFileInfo(path).canonicalFilePath();
  return canonical.isEmpty() ? QDir::cleanPath(path) : canonical;
}

void appendPathRow(QVariantList &rows, std::set<QString> &seenPaths,
                   const QString &id, const QString &section, NSURL *url,
                   const QString &icon, const QString &fallbackLabel = {},
                   int order = 0) {
  if (!url || ![url isFileURL]) {
    return;
  }
  const QString path = QDir::cleanPath(qString([url path]));
  const QString pathKey = canonicalPathKey(path);
  if (path.isEmpty() || !QFileInfo::exists(path) ||
      seenPaths.contains(pathKey)) {
    return;
  }
  seenPaths.insert(pathKey);
  QString label = localizedURLName(url);
  if (label.isEmpty()) {
    label = fallbackLabel;
  }
  QVariantMap row =
      locationRow(id, section, QStringLiteral("path"), label, icon, order);
  row.insert(QStringLiteral("path"), path);
  rows.append(row);
}

void appendCloudRoots(QVariantList &rows, std::set<QString> &seenPaths,
                      NSFileManager *manager, NSURL *cloudStorage,
                      int order = 240) {
  NSArray<NSURL *> *cloudRoots = [manager
        contentsOfDirectoryAtURL:cloudStorage
      includingPropertiesForKeys:@[ NSURLLocalizedNameKey, NSURLIsDirectoryKey ]
                         options:NSDirectoryEnumerationSkipsHiddenFiles
                           error:nil];
  for (NSURL *cloudRoot in cloudRoots) {
    NSNumber *isDirectory = nil;
    [cloudRoot getResourceValue:&isDirectory
                         forKey:NSURLIsDirectoryKey
                          error:nil];
    if ([isDirectory boolValue]) {
      const QString path = QDir::cleanPath(qString([cloudRoot path]));
      appendPathRow(rows, seenPaths, stableID(QStringLiteral("cloud-"), path),
                    QStringLiteral("locations"), cloudRoot,
                    QStringLiteral("folder"), {}, order);
    }
  }
}

QString finderTagName(NSString *rawTag) {
  QString tag = qString(rawTag);
  const qsizetype separator = tag.indexOf(QLatin1Char('\n'));
  if (separator >= 0) {
    tag.truncate(separator);
  }
  return tag.trimmed();
}

QString finderTagColor(NSString *rawTag) {
  const QString tag = qString(rawTag);
  const qsizetype separator = tag.lastIndexOf(QLatin1Char('\n'));
  if (separator < 0 || separator + 1 >= tag.size()) {
    return QStringLiteral("#b7bac0");
  }
  bool ok = false;
  const int color = tag.mid(separator + 1).toInt(&ok);
  if (!ok) {
    return QStringLiteral("#b7bac0");
  }
  switch (color) {
  case 1:
    return QStringLiteral("#9aa0a8"); // gray
  case 2:
    return QStringLiteral("#3fb950"); // green
  case 3:
    return QStringLiteral("#a371f7"); // purple
  case 4:
    return QStringLiteral("#58a6ff"); // blue
  case 5:
    return QStringLiteral("#ffd33d"); // yellow
  case 6:
    return QStringLiteral("#ff7b72"); // red
  case 7:
    return QStringLiteral("#f0883e"); // orange
  default:
    return QStringLiteral("#b7bac0");
  }
}

QVariantMap finderTagLocationRow(NSString *rawTag) {
  const QString tagName = finderTagName(rawTag);
  if (tagName.isEmpty()) {
    return {};
  }
  QVariantMap row = locationRow(
      stableID(QStringLiteral("tag-"), tagName), QStringLiteral("tags"),
      QStringLiteral("query"), tagName, QStringLiteral("tag-dot"), 300);
  row.insert(QStringLiteral("color"), finderTagColor(rawTag));
  row.insert(QStringLiteral("queryKind"), QStringLiteral("tag"));
  row.insert(QStringLiteral("tag"), tagName);
  row.insert(QStringLiteral("uri"),
             QStringLiteral("macos://tag/") +
                 QString::fromLatin1(QUrl::toPercentEncoding(tagName)));
  return row;
}

QVariantMap metadataItemRow(NSMetadataItem *item) {
  NSString *path = [item valueForAttribute:NSMetadataItemPathKey];
  if (![path isKindOfClass:[NSString class]] || [path length] == 0) {
    return {};
  }
  NSString *display = [item valueForAttribute:NSMetadataItemDisplayNameKey];
  if (![display isKindOfClass:[NSString class]] || [display length] == 0) {
    display = [path lastPathComponent];
  }
  NSArray *typeTree = [item valueForAttribute:NSMetadataItemContentTypeTreeKey];
  const bool isDir = [typeTree isKindOfClass:[NSArray class]] &&
                     ([typeTree containsObject:@"public.folder"] ||
                      [typeTree containsObject:@"public.directory"]);
  NSNumber *size = [item valueForAttribute:NSMetadataItemFSSizeKey];
  NSDate *modified =
      [item valueForAttribute:NSMetadataItemFSContentChangeDateKey];
  const QString pathString = qString(path);
  return {
      {QStringLiteral("id"), stableID(QStringLiteral("item-"), pathString)},
      {QStringLiteral("path"), pathString},
      {QStringLiteral("displayName"), qString(display)},
      {QStringLiteral("isDir"), isDir},
      {QStringLiteral("size"),
       size ? QVariant::fromValue<qulonglong>([size unsignedLongLongValue])
            : QVariant::fromValue<qulonglong>(0)},
      {QStringLiteral("sizeKnown"), size != nil},
      {QStringLiteral("mtimeNanos"),
       modified ? QVariant::fromValue<qlonglong>(static_cast<qlonglong>(
                      [modified timeIntervalSince1970] * 1000000000.0))
                : QVariant::fromValue<qlonglong>(0)},
  };
}

enum class MetadataMode {
  Recents,
  Shared,
  Tag,
  AllTags,
  TagDiscovery,
};
} // namespace

@interface F4MetadataOperation : NSObject {
  NSMetadataQuery *_query;
  MetadataMode _mode;
  NSString *_tag;
  BOOL _finished;
  BOOL _live;
  std::function<void(const QVariantList &, bool, NSError *)> _send;
  std::function<void()> _event;
  std::function<void()> _done;
}

- (instancetype)
    initWithMode:(MetadataMode)mode
             tag:(NSString *)tag
            live:(BOOL)live
            send:
                (std::function<void(const QVariantList &, bool, NSError *)>)send
           event:(std::function<void()>)event
            done:(std::function<void()>)done;
- (void)start;
- (void)cancel;

@end

@implementation F4MetadataOperation

- (instancetype)
    initWithMode:(MetadataMode)mode
             tag:(NSString *)tag
            live:(BOOL)live
            send:
                (std::function<void(const QVariantList &, bool, NSError *)>)send
           event:(std::function<void()>)event
            done:(std::function<void()>)done {
  self = [super init];
  if (self) {
    _mode = mode;
    _tag = [tag copy];
    _live = live;
    _send = std::move(send);
    _event = std::move(event);
    _done = std::move(done);
  }
  return self;
}

- (void)dealloc {
  [self cancel];
#if !__has_feature(objc_arc)
  [_tag release];
  [super dealloc];
#endif
}

- (void)start {
  if (_finished) {
    return;
  }
  @try {
    _query = [[NSMetadataQuery alloc] init];
    _query.searchScopes = @[
      NSMetadataQueryLocalComputerScope,
      NSMetadataQueryNetworkScope,
    ];
    NSString *tagKey = @"kMDItemUserTags";
    switch (_mode) {
    case MetadataMode::Recents:
      _query.predicate = [NSPredicate
          predicateWithFormat:@"%K > %@", NSMetadataItemLastUsedDateKey,
                              [NSDate dateWithTimeIntervalSince1970:0]];
      _query.sortDescriptors = @[
        [NSSortDescriptor sortDescriptorWithKey:NSMetadataItemLastUsedDateKey
                                      ascending:NO],
      ];
      break;
    case MetadataMode::Shared:
      _query.predicate = [NSPredicate
          predicateWithFormat:@"%K == YES",
                              NSMetadataUbiquitousItemIsSharedKey];
      break;
    case MetadataMode::Tag:
      _query.predicate = [NSPredicate
          predicateWithFormat:@"(%K == %@ OR %K LIKE %@)", tagKey, _tag, tagKey,
                              [_tag stringByAppendingString:@"\n*"]];
      break;
    case MetadataMode::AllTags:
    case MetadataMode::TagDiscovery:
      _query.predicate =
          [NSPredicate predicateWithFormat:@"%K LIKE %@", tagKey, @"*"];
      _query.valueListAttributes = @[ tagKey ];
      break;
    }
    [[NSNotificationCenter defaultCenter]
        addObserver:self
           selector:@selector(queryFinished:)
               name:NSMetadataQueryDidFinishGatheringNotification
             object:_query];
    if (_live) {
      [[NSNotificationCenter defaultCenter]
          addObserver:self
             selector:@selector(queryUpdated:)
                 name:NSMetadataQueryDidUpdateNotification
               object:_query];
    }
    if (![_query startQuery]) {
      NSError *error =
          [NSError errorWithDomain:@"org.unxed.f4.platform"
                              code:1
                          userInfo:@{
                            NSLocalizedDescriptionKey :
                                @"Spotlight query could not be started"
                          }];
      [self completeWithError:error];
    }
  } @catch (NSException *exception) {
    NSError *error =
        [NSError errorWithDomain:@"org.unxed.f4.platform"
                            code:2
                        userInfo:@{
                          NSLocalizedDescriptionKey : exception.reason
                              ?: @"Invalid Spotlight query"
                        }];
    [self completeWithError:error];
  }
}

- (void)queryFinished:(NSNotification *)notification {
  (void)notification;
  if (_finished || !_query) {
    return;
  }
  [_query disableUpdates];
  if (_live) {
    _send({}, false, nil);
    [_query enableUpdates];
    return;
  }
  QVariantList rows;
  if (_mode == MetadataMode::AllTags ||
      _mode == MetadataMode::TagDiscovery) {
    NSMutableDictionary<NSString *, NSString *> *tags =
        [NSMutableDictionary dictionary];
    for (NSUInteger index = 0; index < _query.resultCount; ++index) {
      NSMetadataItem *item = [_query resultAtIndex:index];
      NSArray *values = [item valueForAttribute:@"kMDItemUserTags"];
      if (![values isKindOfClass:[NSArray class]]) {
        continue;
      }
      for (NSString *rawTag in values) {
        if (![rawTag isKindOfClass:[NSString class]]) {
          continue;
        }
        const QString name = finderTagName(rawTag);
        if (!name.isEmpty()) {
          tags[nsString(name)] = rawTag;
        }
      }
    }
    NSArray<NSString *> *names = [[tags allKeys]
        sortedArrayUsingSelector:@selector(localizedCaseInsensitiveCompare:)];
    for (NSString *name in names) {
      rows.append(finderTagLocationRow(tags[name]));
    }
  } else {
    std::set<QString> seen;
    for (NSUInteger index = 0; index < _query.resultCount; ++index) {
      QVariantMap row = metadataItemRow([_query resultAtIndex:index]);
      const QString path = row.value(QStringLiteral("path")).toString();
      if (!path.isEmpty() && seen.insert(path).second) {
        rows.append(row);
      }
    }
    std::map<QString, int> displayCounts;
    for (const QVariant &value : rows) {
      ++displayCounts
          [value.toMap().value(QStringLiteral("displayName")).toString()];
    }
    for (qsizetype index = 0; index < rows.size(); ++index) {
      QVariantMap row = rows[index].toMap();
      const QString display =
          row.value(QStringLiteral("displayName")).toString();
      if (displayCounts[display] > 1) {
        row.insert(QStringLiteral("displayName"),
                   display + QStringLiteral(" — ") +
                       QFileInfo(row.value(QStringLiteral("path")).toString())
                           .dir()
                           .dirName());
        rows[index] = row;
      }
    }
  }
  [_query enableUpdates];

  if (rows.isEmpty()) {
    _send({}, true, nil);
  } else {
    for (qsizetype offset = 0; offset < rows.size();
         offset += metadataChunkSize) {
      const qsizetype count =
          std::min<qsizetype>(metadataChunkSize, rows.size() - offset);
      _send(rows.mid(offset, count), offset + count == rows.size(), nil);
    }
  }
  [self finish];
}

- (void)queryUpdated:(NSNotification *)notification {
  (void)notification;
  if (!_finished && _live && _event) {
    _event();
  }
}

- (void)completeWithError:(NSError *)error {
  if (_finished) {
    return;
  }
  _send({}, true, error);
  [self finish];
}

- (void)finish {
  if (_finished) {
    return;
  }
  _finished = YES;
  [[NSNotificationCenter defaultCenter] removeObserver:self];
  if (_query) {
    [_query stopQuery];
#if !__has_feature(objc_arc)
    [_query release];
#endif
    _query = nil;
  }
  if (_done) {
    auto done = std::move(_done);
    done();
  }
}

- (void)cancel {
  if (_finished) {
    return;
  }
  _finished = YES;
  [[NSNotificationCenter defaultCenter] removeObserver:self];
  if (_query) {
    [_query stopQuery];
#if !__has_feature(objc_arc)
    [_query release];
#endif
    _query = nil;
  }
  _send = {};
  _event = {};
  _done = {};
}

@end

@interface F4NetServiceResolver : NSObject <NSNetServiceDelegate> {
  NSNetService *_service;
  std::function<void(NSString *, NSInteger, NSError *)> _completion;
  BOOL _finished;
}

- (instancetype)initWithName:(NSString *)name
                        type:(NSString *)type
                      domain:(NSString *)domain
                  completion:
                      (std::function<void(NSString *, NSInteger, NSError *)>)
                          completion;
- (void)start;
- (void)cancel;

@end

@implementation F4NetServiceResolver

- (instancetype)initWithName:(NSString *)name
                        type:(NSString *)type
                      domain:(NSString *)domain
                  completion:
                      (std::function<void(NSString *, NSInteger, NSError *)>)
                          completion {
  self = [super init];
  if (self) {
    NSString *normalizedType =
        [type hasSuffix:@"."] ? type : [type stringByAppendingString:@"."];
    NSString *normalizedDomain = [domain length] == 0 ? @"local." : domain;
    _service = [[NSNetService alloc] initWithDomain:normalizedDomain
                                               type:normalizedType
                                               name:name];
    _service.delegate = self;
    _completion = std::move(completion);
  }
  return self;
}

- (void)dealloc {
  [self cancel];
#if !__has_feature(objc_arc)
  [_service release];
  [super dealloc];
#endif
}

- (void)start {
  if (_finished || !_service) {
    return;
  }
  [_service scheduleInRunLoop:[NSRunLoop mainRunLoop]
                      forMode:NSRunLoopCommonModes];
  [_service resolveWithTimeout:10.0];
}

- (void)finishWithHost:(NSString *)host
                  port:(NSInteger)port
                 error:(NSError *)error {
  if (_finished) {
    return;
  }
  _finished = YES;
  [_service stop];
  [_service removeFromRunLoop:[NSRunLoop mainRunLoop]
                      forMode:NSRunLoopCommonModes];
#if !__has_feature(objc_arc)
  [self retain];
#endif
  if (_completion) {
    auto completion = std::move(_completion);
    completion(host, port, error);
  }
#if !__has_feature(objc_arc)
  [self release];
#endif
}

- (void)netServiceDidResolveAddress:(NSNetService *)sender {
  NSString *host = sender.hostName;
  if ([host length] == 0) {
    NSError *error =
        [NSError errorWithDomain:@"org.unxed.f4.platform"
                            code:3
                        userInfo:@{
                          NSLocalizedDescriptionKey :
                              @"The Bonjour service returned no host name"
                        }];
    [self finishWithHost:nil port:0 error:error];
    return;
  }
  [self finishWithHost:host port:sender.port error:nil];
}

- (void)netService:(NSNetService *)sender
     didNotResolve:(NSDictionary<NSString *, NSNumber *> *)errorDict {
  (void)sender;
  const NSInteger code = [errorDict[NSNetServicesErrorCode] integerValue];
  NSError *error =
      [NSError errorWithDomain:NSNetServicesErrorDomain
                          code:code
                      userInfo:@{
                        NSLocalizedDescriptionKey :
                            @"Unable to resolve the Bonjour service"
                      }];
  [self finishWithHost:nil port:0 error:error];
}

- (void)cancel {
  if (_finished) {
    return;
  }
  _finished = YES;
  [_service stop];
  [_service removeFromRunLoop:[NSRunLoop mainRunLoop]
                      forMode:NSRunLoopCommonModes];
  _completion = {};
}

@end

@interface F4AirDropDelegate : NSObject <NSSharingServiceDelegate> {
  std::function<void(NSError *)> _completion;
}

- (instancetype)initWithCompletion:(std::function<void(NSError *)>)completion;

@end

@implementation F4AirDropDelegate

- (instancetype)initWithCompletion:(std::function<void(NSError *)>)completion {
  self = [super init];
  if (self) {
    _completion = std::move(completion);
  }
  return self;
}

- (void)sharingService:(NSSharingService *)sharingService
         didShareItems:(NSArray *)items {
  (void)sharingService;
  (void)items;
  if (_completion) {
    auto completion = std::move(_completion);
    completion(nil);
  }
}

- (void)sharingService:(NSSharingService *)sharingService
    didFailToShareItems:(NSArray *)items
                  error:(NSError *)error {
  (void)sharingService;
  (void)items;
  if (_completion) {
    auto completion = std::move(_completion);
    completion(error);
  }
}

@end

struct MacPlatformServices::Impl {
  explicit Impl(SendHandler handler)
      : sendHandler(std::move(handler)),
        lifetimeToken(std::make_shared<int>(0)),
        metadataOperations([[NSMutableDictionary alloc] init]),
        serviceResolvers([[NSMutableDictionary alloc] init]),
        airDropDelegates([[NSMutableDictionary alloc] init]),
        airDropServices([[NSMutableDictionary alloc] init]) {}

  ~Impl() {
    lifetimeToken.reset();
    cancelAll();
#if !__has_feature(objc_arc)
    [metadataOperations release];
    [serviceResolvers release];
    [airDropDelegates release];
    [airDropServices release];
#endif
  }

  void send(const QVariantMap &message) const {
    if (sendHandler) {
      sendHandler(message);
    }
  }

  void sendPayload(const QString &requestID, const QString &operation,
                   const QVariantMap &payload, bool chunk, bool final) const {
    send({
        {QStringLiteral("type"), QStringLiteral("platform_response")},
        {QStringLiteral("requestId"), requestID},
        {QStringLiteral("operation"), operation},
        {QStringLiteral("payload"), payload},
        {QStringLiteral("chunk"), chunk},
        {QStringLiteral("final"), final},
    });
  }

  void sendEvent(const QString &requestID, const QString &operation,
                 const QVariantMap &payload) const {
    send({
        {QStringLiteral("type"), QStringLiteral("platform_event")},
        {QStringLiteral("requestId"), requestID},
        {QStringLiteral("operation"), operation},
        {QStringLiteral("payload"), payload},
        {QStringLiteral("chunk"), false},
        {QStringLiteral("final"), false},
    });
  }

  void sendError(const QString &requestID, const QString &operation,
                 const QString &code, const QString &message,
                 bool cancelled = false) const {
    send({
        {QStringLiteral("type"), QStringLiteral("platform_response")},
        {QStringLiteral("requestId"), requestID},
        {QStringLiteral("operation"), operation},
        {QStringLiteral("chunk"), false},
        {QStringLiteral("final"), true},
        {QStringLiteral("error"),
         QVariantMap{
             {QStringLiteral("code"), code},
             {QStringLiteral("message"), message},
             {QStringLiteral("cancelled"), cancelled},
         }},
    });
  }

  static QVariantList discoverLocations(bool includeMountedVolumes = true) {
    @autoreleasepool {
      QVariantList rows;
      std::set<QString> seenPaths;
      NSFileManager *manager = [NSFileManager defaultManager];

      QVariantMap recents =
          locationRow(QStringLiteral("recents"), QStringLiteral("top"),
                      QStringLiteral("query"), QStringLiteral("Recents"),
                      QStringLiteral("file-clock"), 10);
      recents.insert(QStringLiteral("queryKind"), QStringLiteral("recents"));
      recents.insert(QStringLiteral("uri"), QStringLiteral("macos://recents"));
      rows.append(recents);

      QVariantMap shared =
          locationRow(QStringLiteral("shared"), QStringLiteral("top"),
                      QStringLiteral("query"), QStringLiteral("Shared"),
                      QStringLiteral("folder"), 20);
      shared.insert(QStringLiteral("queryKind"), QStringLiteral("shared"));
      shared.insert(QStringLiteral("uri"), QStringLiteral("macos://shared"));
      rows.append(shared);

      NSArray<NSURL *> *applications =
          [manager URLsForDirectory:NSApplicationDirectory
                          inDomains:NSAllDomainsMask];
      NSURL *applicationURL = nil;
      for (NSURL *candidate in applications) {
        if ([[candidate path] isEqualToString:@"/Applications"]) {
          applicationURL = candidate;
          break;
        }
        if (!applicationURL) {
          applicationURL = candidate;
        }
      }
      appendPathRow(rows, seenPaths, QStringLiteral("applications"),
                    QStringLiteral("favorites"), applicationURL,
                    QStringLiteral("folder"), QStringLiteral("Applications"),
                    100);
      const struct {
        NSSearchPathDirectory directory;
        const char *id;
        const char *fallback;
        int order;
      } favorites[] = {
          {NSDesktopDirectory, "desktop", "Desktop", 110},
          {NSDocumentDirectory, "documents", "Documents", 120},
          {NSDownloadsDirectory, "downloads", "Downloads", 130},
      };
      for (const auto &favorite : favorites) {
        NSURL *url = [[manager URLsForDirectory:favorite.directory
                                      inDomains:NSUserDomainMask] firstObject];
        appendPathRow(rows, seenPaths, QString::fromLatin1(favorite.id),
                      QStringLiteral("favorites"), url,
                      QStringLiteral("folder"),
                      QString::fromLatin1(favorite.fallback), favorite.order);
      }

      NSURL *home = [manager homeDirectoryForCurrentUser];
      NSURL *icloud = [home URLByAppendingPathComponent:
                                @"Library/Mobile Documents/com~apple~CloudDocs"
                                            isDirectory:YES];
      appendPathRow(rows, seenPaths, QStringLiteral("icloud-drive"),
                    QStringLiteral("locations"), icloud,
                    QStringLiteral("folder"), QStringLiteral("iCloud Drive"),
                    200);
      appendPathRow(rows, seenPaths, QStringLiteral("home"),
                    QStringLiteral("locations"), home, QStringLiteral("house"),
                    NSUserName() ? qString(NSUserName()) : QString(), 210);

      NSURL *root = [NSURL fileURLWithPath:@"/" isDirectory:YES];
      appendPathRow(rows, seenPaths, QStringLiteral("root-volume"),
                    QStringLiteral("locations"), root,
                    QStringLiteral("folder-root"),
                    QStringLiteral("Macintosh HD"), 220);
      if (includeMountedVolumes) {
        NSArray<NSURLResourceKey> *volumeKeys = @[
          NSURLLocalizedNameKey,
          NSURLVolumeNameKey,
          NSURLVolumeIsBrowsableKey,
          NSURLVolumeIsInternalKey,
        ];
        NSArray<NSURL *> *volumes = [manager
            mountedVolumeURLsIncludingResourceValuesForKeys:volumeKeys
                                                    options:
                                                        NSVolumeEnumerationSkipHiddenVolumes];
        for (NSURL *volume in volumes) {
          const QString path = QDir::cleanPath(qString([volume path]));
          appendPathRow(rows, seenPaths,
                        stableID(QStringLiteral("volume-"), path),
                        QStringLiteral("locations"), volume,
                        QStringLiteral("hard-drive"), {}, 230);
        }
      }

      NSURL *cloudStorage =
          [home URLByAppendingPathComponent:@"Library/CloudStorage"
                                isDirectory:YES];
      appendCloudRoots(rows, seenPaths, manager, cloudStorage);

      QVariantMap airDrop =
          locationRow(QStringLiteral("airdrop"), QStringLiteral("locations"),
                      QStringLiteral("action"), QStringLiteral("AirDrop"),
                      QStringLiteral("network"), 250);
      airDrop.insert(QStringLiteral("action"), QStringLiteral("airdrop"));
      rows.append(airDrop);

      QVariantMap network =
          locationRow(QStringLiteral("network"), QStringLiteral("locations"),
                      QStringLiteral("query"), QStringLiteral("Network"),
                      QStringLiteral("network"), 260);
      network.insert(QStringLiteral("queryKind"), QStringLiteral("network"));
      network.insert(QStringLiteral("uri"), QStringLiteral("macos://network"));
      rows.append(network);

      NSURL *trash = [[manager URLsForDirectory:NSTrashDirectory
                                      inDomains:NSUserDomainMask] firstObject];
      appendPathRow(rows, seenPaths, QStringLiteral("trash"),
                    QStringLiteral("locations"), trash,
                    QStringLiteral("trash-2"), QStringLiteral("Trash"), 270);
      return rows;
    }
  }

  void startMetadata(const QString &requestID, const QString &operation,
                     MetadataMode mode, const QString &tag = {},
                     const QVariantList &prefixRows = {}, bool live = false) {
    NSString *key = nsString(requestID);
    auto sendChunk = [this, requestID, operation](const QVariantList &items,
                                                  bool final, NSError *error) {
      if (error) {
        sendError(requestID, operation, QStringLiteral("spotlight"),
                  qString([error localizedDescription]));
        return;
      }
      sendPayload(requestID, operation, {{QStringLiteral("items"), items}},
                  !final, final);
    };
    auto done = [this, key]() { [metadataOperations removeObjectForKey:key]; };
    auto event = [this, requestID, operation]() {
      sendEvent(requestID, operation, {{QStringLiteral("refresh"), true}});
    };
    F4MetadataOperation *metadata = [[F4MetadataOperation alloc]
        initWithMode:mode
                 tag:tag.isEmpty() ? nil : nsString(tag)
                live:live
                send:std::move(sendChunk)
               event:std::move(event)
                done:std::move(done)];
    metadataOperations[key] = metadata;
#if !__has_feature(objc_arc)
    [metadata release];
#endif
    if (!prefixRows.isEmpty()) {
      sendPayload(requestID, operation, {{QStringLiteral("items"), prefixRows}},
                  true, false);
    }
    [metadata start];
  }

  void startLocations(const QString &requestID, const QString &operation) {
    const std::weak_ptr<int> lifetime = lifetimeToken;
    const QString requestIDCopy = requestID;
    const QString operationCopy = operation;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
      QVariantList rows = Impl::discoverLocations(false);
      dispatch_async(dispatch_get_main_queue(), ^{
        if (lifetime.expired()) {
          return;
        }
        if (!cancelledRequests.contains(requestIDCopy)) {
          sendPayload(requestIDCopy, operationCopy,
                      {{QStringLiteral("items"), rows}}, true, false);
        }
      });
      QVariantList volumes = Impl::discoverMountedVolumes(rows);
      dispatch_async(dispatch_get_main_queue(), ^{
        if (lifetime.expired()) {
          return;
        }
        if (!cancelledRequests.contains(requestIDCopy) &&
            !metadataOperations[nsString(requestIDCopy)]) {
          startMetadata(requestIDCopy, operationCopy,
                        MetadataMode::TagDiscovery, {}, volumes);
        }
      });
    });
  }

  void startQuery(const QString &requestID, const QString &operation,
                  const QVariantMap &payload) {
    const QString kind = payload.value(QStringLiteral("kind")).toString();
    if (kind == QStringLiteral("recents")) {
      startMetadata(requestID, operation, MetadataMode::Recents);
    } else if (kind == QStringLiteral("shared")) {
      startMetadata(requestID, operation, MetadataMode::Shared);
    } else if (kind == QStringLiteral("tag")) {
      startMetadata(requestID, operation, MetadataMode::Tag,
                    payload.value(QStringLiteral("tag")).toString());
    } else if (kind == QStringLiteral("allTags")) {
      startMetadata(requestID, operation, MetadataMode::AllTags);
    } else if (kind == QStringLiteral("network")) {
      startNetwork(requestID, operation);
    } else {
      sendError(requestID, operation, QStringLiteral("invalid_operation"),
                QStringLiteral("Unsupported macOS query kind"));
    }
  }

  void startWatch(const QString &requestID, const QString &operation,
                  const QVariantMap &payload) {
    const QString kind = payload.value(QStringLiteral("kind")).toString();
    if (kind == QStringLiteral("recents")) {
      startMetadata(requestID, operation, MetadataMode::Recents, {}, {}, true);
    } else if (kind == QStringLiteral("shared")) {
      startMetadata(requestID, operation, MetadataMode::Shared, {}, {}, true);
    } else if (kind == QStringLiteral("tag")) {
      startMetadata(requestID, operation, MetadataMode::Tag,
                    payload.value(QStringLiteral("tag")).toString(), {}, true);
    } else if (kind == QStringLiteral("allTags")) {
      startMetadata(requestID, operation, MetadataMode::AllTags, {}, {}, true);
    } else {
      sendError(requestID, operation, QStringLiteral("invalid_operation"),
                QStringLiteral("Unsupported macOS watch kind"));
    }
  }

  static QVariantList discoverMountedVolumes(const QVariantList &knownRows) {
    @autoreleasepool {
      std::set<QString> seenPaths;
      for (const QVariant &value : knownRows) {
        const QString path =
            value.toMap().value(QStringLiteral("path")).toString();
        if (!path.isEmpty()) {
          seenPaths.insert(canonicalPathKey(path));
        }
      }
      QVariantList rows;
      NSFileManager *manager = [NSFileManager defaultManager];
      NSArray<NSURLResourceKey> *volumeKeys = @[
        NSURLLocalizedNameKey,
        NSURLVolumeNameKey,
        NSURLVolumeIsBrowsableKey,
        NSURLVolumeIsInternalKey,
      ];
      NSArray<NSURL *> *volumes = [manager
          mountedVolumeURLsIncludingResourceValuesForKeys:volumeKeys
                                                  options:
                                                      NSVolumeEnumerationSkipHiddenVolumes];
      for (NSURL *volume in volumes) {
        const QString path = QDir::cleanPath(qString([volume path]));
        appendPathRow(rows, seenPaths,
                      stableID(QStringLiteral("volume-"), path),
                      QStringLiteral("locations"), volume,
                      QStringLiteral("hard-drive"), {}, 230);
      }
      return rows;
    }
  }

  struct NetworkState {
    QString requestID;
    QString operation;
    std::map<QString, QVariantMap> rows;
    std::vector<nw_browser_t> browsers;
    bool finished = false;
  };

  void startNetwork(const QString &requestID, const QString &operation) {
    auto state = std::make_shared<NetworkState>();
    state->requestID = requestID;
    state->operation = operation;
    networkOperations[requestID] = state;
    const struct {
      const char *type;
      const char *scheme;
    } services[] = {
        {"_smb._tcp", "smb"},       {"_afpovertcp._tcp", "afp"},
        {"_nfs._tcp", "nfs"},       {"_webdav._tcp", "http"},
        {"_webdavs._tcp", "https"},
    };
    for (const auto &service : services) {
      nw_browse_descriptor_t descriptor =
          nw_browse_descriptor_create_bonjour_service(service.type, nullptr);
      nw_browser_t browser = nw_browser_create(descriptor, nullptr);
      nw_release(descriptor);
      if (!browser) {
        continue;
      }
      state->browsers.push_back(browser);
      nw_browser_set_queue(browser, dispatch_get_main_queue());
      const QString scheme = QString::fromLatin1(service.scheme);
      std::weak_ptr<NetworkState> weak = state;
      nw_browser_set_browse_results_changed_handler(
          browser, ^(nw_browse_result_t oldResult, nw_browse_result_t newResult,
                     bool batchComplete) {
            (void)batchComplete;
            auto current = weak.lock();
            if (!current || current->finished) {
              return;
            }
            auto eraseResult = [&](nw_browse_result_t result) {
              if (!result)
                return;
              nw_endpoint_t endpoint = nw_browse_result_copy_endpoint(result);
              if (!endpoint)
                return;
              const char *name = nw_endpoint_get_bonjour_service_name(endpoint);
              const char *type = nw_endpoint_get_bonjour_service_type(endpoint);
              const char *domain =
                  nw_endpoint_get_bonjour_service_domain(endpoint);
              const QString key =
                  QString::fromUtf8(name ? name : "") + QLatin1Char('|') +
                  QString::fromUtf8(type ? type : "") + QLatin1Char('|') +
                  QString::fromUtf8(domain ? domain : "");
              current->rows.erase(key);
              nw_release(endpoint);
            };
            eraseResult(oldResult);
            if (newResult) {
              nw_endpoint_t endpoint =
                  nw_browse_result_copy_endpoint(newResult);
              if (!endpoint)
                return;
              const QString name = QString::fromUtf8(
                  nw_endpoint_get_bonjour_service_name(endpoint));
              const QString type = QString::fromUtf8(
                  nw_endpoint_get_bonjour_service_type(endpoint));
              const QString domain = QString::fromUtf8(
                  nw_endpoint_get_bonjour_service_domain(endpoint));
              const QString key =
                  name + QLatin1Char('|') + type + QLatin1Char('|') + domain;
              current->rows[key] = {
                  {QStringLiteral("id"),
                   stableID(QStringLiteral("network-"), key)},
                  {QStringLiteral("displayName"), name},
                  {QStringLiteral("serviceName"), name},
                  {QStringLiteral("serviceType"), type},
                  {QStringLiteral("serviceDomain"), domain},
                  {QStringLiteral("scheme"), scheme},
                  {QStringLiteral("networkService"), true},
                  {QStringLiteral("isDir"), true},
              };
              nw_release(endpoint);
            }
          });
      nw_browser_start(browser);
    }
    const std::weak_ptr<NetworkState> weakState = state;
    const std::weak_ptr<int> lifetime = lifetimeToken;
    dispatch_after(
        dispatch_time(DISPATCH_TIME_NOW,
                      static_cast<int64_t>(900 * NSEC_PER_MSEC)),
        dispatch_get_main_queue(), ^{
          auto current = weakState.lock();
          if (lifetime.expired() || !current || current->finished) {
            return;
          }
          QVariantList items;
          QVariantMap connectRow = {
              {QStringLiteral("id"), QStringLiteral("connect-server")},
              {QStringLiteral("displayName"),
               QStringLiteral("Connect to Server…")},
              {QStringLiteral("action"), QStringLiteral("connectServer")},
              {QStringLiteral("isDir"), false},
          };
          items.append(connectRow);
          for (const auto &entry : current->rows) {
            items.append(entry.second);
          }
          sendPayload(current->requestID, current->operation,
                      {{QStringLiteral("items"), items}}, false, true);
          finishNetwork(current->requestID);
        });
  }

  void finishNetwork(const QString &requestID) {
    auto found = networkOperations.find(requestID);
    if (found == networkOperations.end()) {
      return;
    }
    auto state = found->second;
    state->finished = true;
    for (nw_browser_t browser : state->browsers) {
      nw_browser_cancel(browser);
      nw_release(browser);
    }
    state->browsers.clear();
    networkOperations.erase(found);
  }

  void startMountURL(const QString &requestID, const QString &operation,
                     QUrl parsed) {
    if (parsed.scheme().compare(QStringLiteral("webdav"),
                                Qt::CaseInsensitive) == 0) {
      parsed.setScheme(QStringLiteral("http"));
    } else if (parsed.scheme().compare(QStringLiteral("webdavs"),
                                       Qt::CaseInsensitive) == 0) {
      parsed.setScheme(QStringLiteral("https"));
    }
    NSURL *url =
        [NSURL URLWithString:nsString(parsed.toString(QUrl::FullyEncoded))];
    if (!url) {
      sendError(requestID, operation, QStringLiteral("invalid_url"),
                QStringLiteral("The server URL is invalid"));
      return;
    }
    NSMutableDictionary *openOptions = [NSMutableDictionary
        dictionaryWithObject:(__bridge id)kNAUIOptionAllowUI
                      forKey:(__bridge id)kNAUIOptionKey];
    AsyncRequestID asyncID = nullptr;
    const std::weak_ptr<int> lifetime = lifetimeToken;
    const QString requestIDCopy = requestID;
    const QString operationCopy = operation;
    const int status = NetFSMountURLAsync(
        (__bridge CFURLRef)url, nullptr, nullptr, nullptr,
        (__bridge CFMutableDictionaryRef)openOptions, nullptr, &asyncID,
        dispatch_get_main_queue(),
        ^(int mountStatus, AsyncRequestID completedID, CFArrayRef mountpoints) {
          (void)completedID;
          if (lifetime.expired()) {
            return;
          }
          mountOperations.erase(requestIDCopy);
          if (mountStatus != 0) {
            sendError(requestIDCopy, operationCopy,
                      mountStatus == userCanceledErr
                          ? QStringLiteral("cancelled")
                          : QStringLiteral("mount_failed"),
                      mountStatus == userCanceledErr
                          ? QStringLiteral("Connection cancelled")
                          : QStringLiteral("Unable to mount the server (%1)")
                                .arg(mountStatus),
                      mountStatus == userCanceledErr);
            return;
          }
          QVariantList paths;
          for (NSString *path in (__bridge NSArray *)mountpoints) {
            paths.append(qString(path));
          }
          sendPayload(requestIDCopy, operationCopy,
                      {{QStringLiteral("mountPaths"), paths}}, false, true);
        });
    if (status != 0) {
      sendError(requestID, operation, QStringLiteral("mount_failed"),
                QStringLiteral("Unable to start mounting (%1)").arg(status));
      return;
    }
    mountOperations[requestID] = asyncID;
  }

  void startMount(const QString &requestID, const QString &operation,
                  const QVariantMap &payload) {
    const std::set<QString> schemes = {
        QStringLiteral("smb"),     QStringLiteral("afp"),
        QStringLiteral("nfs"),     QStringLiteral("http"),
        QStringLiteral("https"),   QStringLiteral("webdav"),
        QStringLiteral("webdavs"),
    };
    const QString serviceName =
        payload.value(QStringLiteral("serviceName")).toString();
    if (!serviceName.isEmpty()) {
      const QString serviceType =
          payload.value(QStringLiteral("serviceType")).toString();
      const QString serviceDomain =
          payload.value(QStringLiteral("serviceDomain")).toString();
      const QString scheme =
          payload.value(QStringLiteral("scheme")).toString().toLower();
      if (serviceType.isEmpty() || !schemes.contains(scheme)) {
        sendError(requestID, operation, QStringLiteral("invalid_service"),
                  QStringLiteral("The Bonjour service is incomplete"));
        return;
      }
      const QString requestIDCopy = requestID;
      const QString operationCopy = operation;
      const std::weak_ptr<int> lifetime = lifetimeToken;
      NSString *key = nsString(requestIDCopy);
      F4NetServiceResolver *resolver = [[F4NetServiceResolver alloc]
          initWithName:nsString(serviceName)
                  type:nsString(serviceType)
                domain:nsString(serviceDomain)
            completion:[this, requestIDCopy, operationCopy, scheme, key,
                        lifetime](NSString *host, NSInteger port,
                                  NSError *error) {
              if (lifetime.expired()) {
                return;
              }
              [serviceResolvers removeObjectForKey:key];
              if (error) {
                sendError(requestIDCopy, operationCopy,
                          QStringLiteral("resolve_failed"),
                          qString([error localizedDescription]));
                return;
              }
              QUrl resolved;
              resolved.setScheme(scheme);
              resolved.setHost(qString(host));
              if (port > 0 && port <= 65535) {
                resolved.setPort(static_cast<int>(port));
              }
              startMountURL(requestIDCopy, operationCopy, resolved);
            }];
      serviceResolvers[key] = resolver;
#if !__has_feature(objc_arc)
      [resolver release];
#endif
      [serviceResolvers[key] start];
      return;
    }

    QUrl parsed(payload.value(QStringLiteral("url")).toString());
    if (!parsed.isValid() || parsed.host().isEmpty() ||
        !schemes.contains(parsed.scheme().toLower()) ||
        !parsed.userName().isEmpty() || !parsed.password().isEmpty()) {
      sendError(requestID, operation, QStringLiteral("invalid_url"),
                QStringLiteral("Enter a server URL without credentials"));
      return;
    }
    startMountURL(requestID, operation, parsed);
  }

  void startAirDrop(const QString &requestID, const QString &operation,
                    const QVariantMap &payload) {
    const QVariantList values = payload.value(QStringLiteral("paths")).toList();
    NSMutableArray *items = [NSMutableArray arrayWithCapacity:values.size()];
    for (const QVariant &value : values) {
      const QString path = value.toString();
      if (!path.isEmpty()) {
        [items addObject:[NSURL fileURLWithPath:nsString(path)]];
      }
    }
    if ([items count] == 0) {
      sendError(requestID, operation, QStringLiteral("empty_selection"),
                QStringLiteral("There are no files to share"));
      return;
    }
    NSSharingService *service = [NSSharingService
        sharingServiceNamed:NSSharingServiceNameSendViaAirDrop];
    if (!service || ![service canPerformWithItems:items]) {
      sendError(requestID, operation, QStringLiteral("airdrop_unavailable"),
                QStringLiteral("AirDrop is unavailable for this selection"));
      return;
    }
    NSString *key = nsString(requestID);
    const std::weak_ptr<int> lifetime = lifetimeToken;
    F4AirDropDelegate *delegate = [[F4AirDropDelegate alloc]
        initWithCompletion:[this, requestID, operation, key,
                            lifetime](NSError *error) {
          if (lifetime.expired()) {
            return;
          }
          [airDropDelegates removeObjectForKey:key];
          [airDropServices removeObjectForKey:key];
          if (error) {
            sendError(requestID, operation,
                      error.code == NSUserCancelledError
                          ? QStringLiteral("cancelled")
                          : QStringLiteral("airdrop_failed"),
                      qString([error localizedDescription]),
                      error.code == NSUserCancelledError);
          } else {
            sendPayload(requestID, operation, {}, false, true);
          }
        }];
    airDropDelegates[key] = delegate;
    airDropServices[key] = service;
    service.delegate = delegate;
#if !__has_feature(objc_arc)
    [delegate release];
#endif
    [service performWithItems:items];
  }

  void cancel(const QString &requestID) {
    cancelledRequests.insert(requestID);
    NSString *key = nsString(requestID);
    if (F4MetadataOperation *metadata = metadataOperations[key]) {
      [metadata cancel];
      [metadataOperations removeObjectForKey:key];
    }
    if (F4NetServiceResolver *resolver = serviceResolvers[key]) {
      [resolver cancel];
      [serviceResolvers removeObjectForKey:key];
    }
    finishNetwork(requestID);
    auto mount = mountOperations.find(requestID);
    if (mount != mountOperations.end()) {
      NetFSMountURLCancel(mount->second);
      mountOperations.erase(mount);
    }
    // NSSharingService has no public cancellation method. Dropping the
    // response target makes cancellation safe; its native sheet owns the
    // remaining user interaction and delegate lifetime.
  }

  void cancelAll() {
    NSArray *keys = [metadataOperations allKeys];
    for (NSString *key in keys) {
      [metadataOperations[key] cancel];
    }
    [metadataOperations removeAllObjects];
    for (F4NetServiceResolver *resolver in [serviceResolvers allValues]) {
      [resolver cancel];
    }
    [serviceResolvers removeAllObjects];
    while (!networkOperations.empty()) {
      finishNetwork(networkOperations.begin()->first);
    }
    for (const auto &mount : mountOperations) {
      NetFSMountURLCancel(mount.second);
    }
    mountOperations.clear();
    for (NSSharingService *service in [airDropServices allValues]) {
      service.delegate = nil;
    }
    [airDropServices removeAllObjects];
    [airDropDelegates removeAllObjects];
  }

  void handle(const QVariantMap &message) {
    const QString type = message.value(QStringLiteral("type")).toString();
    const QString requestID =
        message.value(QStringLiteral("requestId")).toString();
    if (requestID.isEmpty()) {
      return;
    }
    if (type == QStringLiteral("platform_cancel")) {
      cancel(requestID);
      return;
    }
    const QString operation =
        message.value(QStringLiteral("operation")).toString();
    cancelledRequests.erase(requestID);
    const QVariantMap payload =
        message.value(QStringLiteral("payload")).toMap();
    if (operation == QStringLiteral("macos.locations")) {
      startLocations(requestID, operation);
    } else if (operation == QStringLiteral("macos.query")) {
      startQuery(requestID, operation, payload);
    } else if (operation == QStringLiteral("macos.watch")) {
      startWatch(requestID, operation, payload);
    } else if (operation == QStringLiteral("macos.mount")) {
      startMount(requestID, operation, payload);
    } else if (operation == QStringLiteral("macos.airdrop")) {
      startAirDrop(requestID, operation, payload);
    } else {
      sendError(requestID, operation, QStringLiteral("unsupported"),
                QStringLiteral("Unsupported macOS platform operation"));
    }
  }

  SendHandler sendHandler;
  std::shared_ptr<int> lifetimeToken;
  NSMutableDictionary<NSString *, F4MetadataOperation *> *metadataOperations;
  NSMutableDictionary<NSString *, F4NetServiceResolver *> *serviceResolvers;
  NSMutableDictionary<NSString *, F4AirDropDelegate *> *airDropDelegates;
  NSMutableDictionary<NSString *, NSSharingService *> *airDropServices;
  std::map<QString, std::shared_ptr<NetworkState>> networkOperations;
  std::map<QString, AsyncRequestID> mountOperations;
  std::set<QString> cancelledRequests;
};

MacPlatformServices::MacPlatformServices(SendHandler sendHandler)
    : m_impl(std::make_unique<Impl>(std::move(sendHandler))) {}

MacPlatformServices::~MacPlatformServices() = default;

void MacPlatformServices::handleMessage(const QVariantMap &message) {
  m_impl->handle(message);
}

void MacPlatformServices::cancelAll() { m_impl->cancelAll(); }
