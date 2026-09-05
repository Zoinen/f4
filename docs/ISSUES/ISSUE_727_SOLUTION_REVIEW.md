# Issue #727 solution review

## Problem

`plugins/cloudfox/provider_s3.go` used the deprecated
`feature/s3/manager` upload API. The current AWS SDK publishes
`feature/s3/transfermanager`; its API is intentionally different and the
old package is being flagged by static analysis.

## Candidate solutions

1. Keep the old manager and suppress the warning. This leaves the deprecated
   dependency and misses future fixes in the replacement.
2. Replace only the constructor and upload call. This is incomplete because
   the replacement changes multipart options, failure cleanup, and the
   multipart error type.
3. Migrate the upload path to `transfermanager` while preserving the existing
   semantics: 8 MiB parts, two workers, conditional no-replace uploads,
   checksum policy, selected bucket region, and bounded cleanup after
   cancellation. Keep the old `manager` import only for its separate bucket
   region discovery helper.

## Chosen solution

Use `transfermanager.New` and `UploadObject` with
`MultipartUploadThreshold`, `PartSizeBytes`, `Concurrency`,
`RequestChecksumCalculation`, and `FailTimeout` configured explicitly. The
new manager owns multipart aborts, so the duplicate outer abort is removed.
For discovered buckets, construct a per-upload S3 client with the selected
region; this replaces the old manager's per-request `ClientOptions` hook.

## Verification

- Run CloudFox tests, including no-replace multipart and cancellation cleanup
  tests against a local HTTP S3 test server.
- Run the full Go test suite and CloudFox vet checks.
- Build and run the native Windows amd64 binary and plugin checks.

— zoin-bot
