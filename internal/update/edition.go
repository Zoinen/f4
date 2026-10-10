//go:build !lite

package update

// liteEdition is false here and true in edition_lite.go. It picks which
// release assets this binary updates to (editionAssetSuffixes).
const liteEdition = false
