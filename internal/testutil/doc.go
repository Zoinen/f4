// Package testutil holds the test scaffolding more than one package needs: the
// frame-manager harness, the process-wide test setup, and the checked
// conversions test fixtures use.
//
// It is a normal package rather than a _test.go file because Go will not let
// one package import another's tests, and 91 call sites across the tree need
// these helpers. That is the price of the split, and it is why nothing here
// may be imported from production code: this package exists for _test.go files
// only.
//
// Nothing here may import another internal package. Anything that needs one is
// not shared scaffolding — it is one package's own test helper, and belongs
// there. Where a helper genuinely needs a caller's seam, it takes it as an
// argument; see SwapFrameManager's drains and PressKey's filter.
package testutil
