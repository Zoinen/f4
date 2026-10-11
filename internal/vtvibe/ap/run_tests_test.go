package ap

// run_tests_test.go ports the remaining file-fixture half of
// implementation/run_tests.py's TESTS list from github.com/unxed/ap.
//
// The reference test suite has two halves: the declarative one in
// implementation/cases.py (ported verbatim in cases_test.go) and the
// older, file-fixture-driven one in implementation/run_tests.py itself,
// whose cases live as separate files under implementation/{patches,src,
// expected}/. legacy_cases_test.go already ported the directory-oriented
// and tolerant/strict-comment-handling slice of that older half (RENAME,
// bare-directory CREATE, whole-file/whole-directory DELETE, mixed atomic
// delete, 16/17/20/35/36, and the 61-66 tolerant-vs-strict pairs) as hand
// -written Go string literals, the same way cases_test.go does.
//
// The cases here are the rest of run_tests.py's TESTS list: 56 of the 75
// entries, covering everything legacy_cases_test.go does not (basic
// REPLACE mechanics, every *_NOT_FOUND/AMBIGUOUS_MATCH error path, range
// locators, the anchor/heuristic-resolution suite, CRLF/CR line-ending
// handling including explicit `newline` directives, idempotency variants,
// RECREATE, boundary anchors `^`/`$`, and multipass retry). Their patches
// operate on real, syntactically meaningful source files (a C++ snippet,
// several Python modules, a JS file, files with real CRLF/CR content) -
// retyping those into Go string literals would risk a silent
// transcription mistake that a diff against the upstream fixture would
// have caught instantly. So, per the plan recorded in the project's
// tracking notes, these are embedded byte-exact from the upstream
// implementation/{patches,src,expected}/ directories instead (copied
// under testdata/reftests/, unmodified) and run through a Go port of
// run_tests.py's run_positive_test/run_negative_test, not hand-retyped.
//
// 19 (legacy_cases_test.go) + 56 (this file) = all 75 TESTS entries.
//
// Not ported: run_tests.py's own generate_test_patches() (the patches it
// writes at start-up are already committed byte-for-byte identical to its
// output upstream, so there is nothing to regenerate) and its
// --update-expected/JSON failure-report-file machinery (CLI-only
// concerns; ap.Apply is a library call, not the reference's CLI wrapper).
// Negative-test atomicity and the SNIPPET_NOT_FOUND fuzzy_matches check
// are still ported, since those exercise the engine itself.

import (
	"bytes"
	"crypto/md5"
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
)

//go:embed testdata/reftests
var reftestsFS embed.FS

const reftestsRoot = "testdata/reftests"

// refTest is one entry of run_tests.py's TESTS list, plus the handful of
// per-test special cases run_positive_test/run_negative_test hard-code by
// test name (get_paths()'s file_map, the actual_file_rel_path overrides,
// 52_force_success_part's relaxed status check, 54_crlf_preservation's
// LF->CRLF setup).
type refTest struct {
	name string

	negative bool
	errCode  ErrCode // negative only

	// srcNames are the fixture basenames under testdata/reftests/src/,
	// copied verbatim into the temp project dir before applying (mirrors
	// get_paths()'s file_map + run_{positive,negative}_test's copy loop).
	srcNames []string

	// actualRelPath overrides srcNames[0] as the path (relative to the
	// project dir) checked after a positive apply, for patches whose
	// target is a file the patch itself creates rather than the fixture
	// copied in under its own name. Empty means srcNames[0].
	actualRelPath string

	// skipStatusCheck mirrors run_positive_test's is_partial_success_test:
	// 52_force_success_part does not require Status == SUCCESS, only that
	// the surviving modification took effect.
	skipStatusCheck bool

	// crlfConvert mirrors 54_crlf_preservation's setup step: the fixture
	// is stored with LF endings (git-friendly) and turned into CRLF right
	// before applying, so the test actually exercises "existing CRLF is
	// preserved" rather than baking CRLF into the repo.
	crlfConvert bool
}

var refTests = []refTest{
	{name: "01_basic_replace", srcNames: []string{"01_basic.cpp"}},
	{name: "02_sequences", srcNames: []string{"02_sequences.py"}},
	{name: "03_tabs", srcNames: []string{"03_tabs.py"}},
	{name: "04_spaces", srcNames: []string{"04_spaces.py"}},
	{name: "05_crlf", srcNames: []string{"05_crlf.txt"}},
	{name: "06_short_anchor", srcNames: []string{"06_short_anchor.py"}},
	{name: "07_empty_lines", srcNames: []string{"07_empty_lines.py"}},
	{name: "08_error_snippet_not_found", negative: true, errCode: ErrSnippetNotFound, srcNames: []string{"08_error_src.py"}},
	{name: "09_error_anchor_not_found", negative: true, errCode: ErrAnchorNotFound, srcNames: []string{"09_error_src.py"}},
	{name: "10_error_ambiguous", negative: true, errCode: ErrAmbiguousMatch, srcNames: []string{"10_error_src.py"}},
	{name: "11_error_invalid_header", negative: true, errCode: ErrInvalidPatchFile, srcNames: []string{"dummy.txt"}},
	{name: "12_error_invalid_spec", negative: true, errCode: ErrInvalidPatchFile, srcNames: []string{"dummy.txt"}},
	{name: "13_create_file", srcNames: []string{"dummy.txt"}, actualRelPath: "new/created_file.txt"},
	{name: "14_edge_cases", srcNames: []string{"14_edge_cases.py"}},
	{name: "15_robustness", srcNames: []string{"15_robustness.js"}},
	{name: "18_idempotency", srcNames: []string{"18_idempotency.py"}},
	{name: "19_idempotency_noop", srcNames: []string{"19_idempotency_noop.py"}},
	{name: "21_error_atomic_failure", negative: true, errCode: ErrSnippetNotFound, srcNames: []string{"21_atomic_src1.txt", "21_atomic_src2.txt"}},
	{name: "22_range_replace", srcNames: []string{"22_range_replace.py"}},
	{name: "23_error_range_ambiguous", negative: true, errCode: ErrAmbiguousMatch, srcNames: []string{"23_error_range_ambiguous.py"}},
	{name: "24_heuristics", srcNames: []string{"24_heuristics.py"}},
	{name: "25_calculator_example", srcNames: []string{"25_calculator.py"}},
	{name: "26_implicit_create_file", srcNames: []string{"dummy.txt"}, actualRelPath: "26_implicit_create_file.txt"},
	{name: "27_anchor_resolution", srcNames: []string{"27_anchor_resolution.py"}},
	{name: "28_mixed_locators", srcNames: []string{"28_mixed_locators.py"}},
	{name: "29_anchor_overlap", srcNames: []string{"29_anchor_overlap.py"}},
	{name: "30_locality_heuristic", srcNames: []string{"30_locality_heuristic.py"}},
	{name: "31_redundant_snippet", srcNames: []string{"31_redundant_snippet.py"}},
	{name: "32_intersection_resolution", srcNames: []string{"32_intersection_resolution.py"}},
	{name: "33_snippet_locality", srcNames: []string{"33_snippet_locality.py"}},
	{name: "34_range_priority_strict", srcNames: []string{"34_range_priority_strict.py"}},
	{name: "37_heuristic_end_eq_content", srcNames: []string{"37_heuristic_end_eq_content.py"}},
	{name: "38_deep_scope", srcNames: []string{"38_deep_scope.py"}},
	{name: "39_sequential_repeats", srcNames: []string{"39_sequential_repeats.py"}},
	{name: "40_unified_snippet", srcNames: []string{"40_unified_snippet.py"}},
	{name: "41_indent_trailing_newline", srcNames: []string{"41_indent_trailing_newline.py"}},
	{name: "42_strict_cursor", negative: true, errCode: ErrSnippetNotFound, srcNames: []string{"42_strict_cursor.py"}},
	{name: "43_heuristic_implicit_create", srcNames: []string{"dummy.txt"}, actualRelPath: "43_created.txt"},
	{name: "49_sequential_cursor", srcNames: []string{"49_sequential_cursor.py"}},
	{name: "50_identical_snippet_tail", srcNames: []string{"50_identical_snippet_tail.py"}},
	{name: "52_force_success_part", srcNames: []string{"52_atomic_src1.txt", "52_atomic_src2.txt"}, skipStatusCheck: true},
	{name: "53_force_fail_report", negative: true, errCode: ErrSnippetNotFound, srcNames: []string{"52_atomic_src1.txt", "52_atomic_src2.txt"}},
	{name: "54_crlf_preservation", srcNames: []string{"54_crlf.txt"}, crlfConvert: true},
	{name: "56_insert_noop", srcNames: []string{"56_insert_noop.txt"}},
	{name: "57_explicit_lf", srcNames: []string{"57_explicit_lf.txt"}},
	{name: "58_explicit_cr", srcNames: []string{"58_explicit_cr.txt"}},
	{name: "60_idempotent_create", srcNames: []string{"60_idempotent_create.txt"}},
	{name: "67_actions_after_create", srcNames: []string{"dummy.txt"}, actualRelPath: "67_created.txt"},
	{name: "68_idempotency_cursor_desync", srcNames: []string{"68_source.py"}},
	{name: "69_delete_snippet_tail_not_found", negative: true, errCode: ErrSnippetTailNotFound, srcNames: []string{"69_source.py"}},
	{name: "70_recreate_file", srcNames: []string{"70_source.txt"}},
	{name: "71_recreate_idempotency", srcNames: []string{"71_source.txt"}},
	{name: "72_boundary_anchors_range", srcNames: []string{"72_source.txt"}},
	{name: "73_boundary_anchors_single", srcNames: []string{"73_source.txt"}},
	{name: "74_robust_overlap", srcNames: []string{"74_source.txt"}},
	{name: "75_multipass_retry", srcNames: []string{"75_source.py"}},
}

func writeFromEmbed(t *testing.T, embedRelPath, dstPath string) {
	t.Helper()
	data, err := reftestsFS.ReadFile(embedRelPath)
	if err != nil {
		t.Fatalf("reading embedded fixture %q: %v", embedRelPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		t.Fatalf("mkdir for %q: %v", dstPath, err)
	}
	if err := os.WriteFile(dstPath, data, 0o600); err != nil {
		t.Fatalf("writing %q: %v", dstPath, err)
	}
}

func runRefTest(t *testing.T, tc refTest) {
	t.Helper()
	dir := t.TempDir()

	for _, s := range tc.srcNames {
		writeFromEmbed(t, path.Join(reftestsRoot, "src", s), filepath.Join(dir, filepath.FromSlash(s)))
	}

	if tc.crlfConvert {
		p := filepath.Join(dir, filepath.FromSlash(tc.srcNames[0]))
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reading %q for CRLF conversion: %v", p, err)
		}
		b = bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatalf("writing CRLF-converted %q: %v", p, err)
		}
	}

	initialHashes := make(map[string][16]byte, len(tc.srcNames))
	if tc.negative {
		for _, s := range tc.srcNames {
			b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(s)))
			if err != nil {
				t.Fatalf("reading %q before apply: %v", s, err)
			}
			initialHashes[s] = md5.Sum(b) //nolint:gosec // fixture integrity check, not a security hash
		}
	}

	patchBytes, err := reftestsFS.ReadFile(path.Join(reftestsRoot, "patches", tc.name+".ap"))
	if err != nil {
		t.Fatalf("reading embedded patch for %q: %v", tc.name, err)
	}
	patchPath := filepath.Join(dir, "_reftest.ap")
	if err := os.WriteFile(patchPath, patchBytes, 0o600); err != nil {
		t.Fatalf("writing patch file: %v", err)
	}

	result := Apply(patchPath, dir, Options{Strict: tc.negative, Silent: true})

	if tc.negative {
		if result.Status != StatusFailed {
			t.Fatalf("expected status FAILED, got %s", result.Status)
		}
		var actualCode ErrCode
		if result.Error != nil {
			actualCode = result.Error.Code
		}
		if actualCode != tc.errCode {
			detail := ""
			if result.Error != nil {
				detail = ": " + result.Error.Message
			}
			t.Fatalf("expected error %s, got %s%s", tc.errCode, actualCode, detail)
		}

		if tc.errCode == ErrSnippetNotFound {
			var ok bool
			if result.Error != nil && result.Error.Context != nil {
				_, ok = result.Error.Context["fuzzy_matches"]
			}
			if !ok {
				t.Fatalf("expected fuzzy_matches in the error context")
			}
		}

		for _, s := range tc.srcNames {
			b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(s)))
			if err != nil {
				t.Fatalf("re-reading %q after a failed apply: %v", s, err)
			}
			if md5.Sum(b) != initialHashes[s] { //nolint:gosec // fixture integrity check, not a security hash
				t.Fatalf("file %q was modified despite the failed (atomic) apply", s)
			}
		}
		return
	}

	if !tc.skipStatusCheck && result.Status != StatusSuccess {
		detail := ""
		if result.Error != nil {
			detail = fmt.Sprintf(" (%s: %s)", result.Error.Code, result.Error.Message)
		}
		t.Fatalf("expected status SUCCESS, got %s%s", result.Status, detail)
	}

	actualRel := tc.actualRelPath
	if actualRel == "" {
		actualRel = tc.srcNames[0]
	}

	actualPath := filepath.Join(dir, filepath.FromSlash(actualRel))
	actualBytes, err := os.ReadFile(actualPath)
	if err != nil {
		t.Fatalf("expected output file %q not found: %v", actualRel, err)
	}

	expectedBytes, err := reftestsFS.ReadFile(path.Join(reftestsRoot, "expected", actualRel))
	if err != nil {
		t.Fatalf("reading expected fixture for %q: %v", actualRel, err)
	}

	if !bytes.Equal(actualBytes, expectedBytes) {
		t.Fatalf("content of %q differs:\n--- expected ---\n%q\n--- actual ---\n%q", actualRel, expectedBytes, actualBytes)
	}
}

func TestRunTestsFixtureCases(t *testing.T) {
	for _, tc := range refTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runRefTest(t, tc)
		})
	}
}
