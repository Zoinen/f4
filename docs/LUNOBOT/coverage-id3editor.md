# Coverage: `plugins/id3editor`

## Status

This slice adds regression coverage for the plugin's legacy close path, failed
ID3 file opening, and comment updates for ID3v2.2 and ID3v2.3 tags. The package
still has UI-only dialog interaction outside this slice. ID3v2 comment frames
include their language/description prefix, so the regression checks the stored
comment content rather than requiring a presentation-specific prefix.
