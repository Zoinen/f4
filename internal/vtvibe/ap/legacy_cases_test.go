package ap

// cases_test.go ports implementation/cases.py verbatim, but that is only
// "the declarative half" of the reference test suite (ap-reference
// implementation/todo.md item 11: "the older half of the suite" - the
// file-fixture-based tests driven by implementation/run_tests.py's TESTS
// list and its patches/, src/, expected/ directories - "was never migrated
// into cases.py"). That older half is where RENAME, whole-file/whole-dir
// DELETE, bare-directory CREATE, the "mixed atomic delete" and "create over
// an existing directory" error paths, and the tolerant-vs-strict handling
// of patch-level comments/a missing header/an anchor used as a snippet
// actually get exercised - none of it is covered by cases_test.go. This
// file ports those specific run_tests.py cases (not a 1:1 port of one
// upstream file the way cases_test.go is; each case names the run_tests.py
// test name and patch/src/expected fixture it mirrors, with the same patch
// text and file contents).

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type legacyCase struct {
	name string

	// files are written with their exact given content; dirs are created
	// empty. Parent directories for both are created automatically.
	files map[string]string

	dirs []string

	patch string

	strict bool

	expectStatus Status

	expectError ErrCode

	// expectFiles maps a relative path to its exact expected content.
	expectFiles map[string]string

	// expectMissing lists relative paths (file or dir) that must not exist.
	expectMissing []string

	// expectDirs lists relative paths that must exist and be directories.
	expectDirs []string
}

func runLegacyCase(t *testing.T, c legacyCase) {
	t.Helper()
	dir := t.TempDir()

	for _, rel := range c.dirs {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatalf("setup: mkdir %q: %v", rel, err)
		}
	}
	for rel, content := range c.files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("setup: mkdir for %q: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("setup: write %q: %v", rel, err)
		}
	}

	patchPath := filepath.Join(dir, "_case.ap")
	if err := os.WriteFile(patchPath, []byte(c.patch), 0o600); err != nil {
		t.Fatalf("setup: write patch: %v", err)
	}

	result := Apply(patchPath, dir, Options{Strict: c.strict, Silent: true})

	expectedStatus := c.expectStatus
	if expectedStatus == "" {
		expectedStatus = StatusSuccess
	}
	if result.Status != expectedStatus {
		detail := ""
		if result.Error != nil {
			detail = fmt.Sprintf(" (%s: %s)", result.Error.Code, result.Error.Message)
		}
		t.Fatalf("expected status %s, got %s%s", expectedStatus, result.Status, detail)
	}

	if c.expectError != "" {
		var actual ErrCode
		if result.Error != nil {
			actual = result.Error.Code
		}
		if actual != c.expectError {
			t.Fatalf("expected error %s, got %s", c.expectError, actual)
		}
	}

	for rel, expected := range c.expectFiles {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected file %q does not exist: %v", rel, err)
		}
		if string(b) != expected {
			t.Fatalf("content of %q differs:\n--- expected ---\n%q\n--- actual ---\n%q", rel, expected, string(b))
		}
	}

	for _, rel := range c.expectMissing {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil {
			t.Fatalf("path %q should not exist", rel)
		}
	}

	for _, rel := range c.expectDirs {
		fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("expected directory %q does not exist: %v", rel, err)
		}
		if !fi.IsDir() {
			t.Fatalf("expected %q to be a directory", rel)
		}
	}
}

var legacyCases = []legacyCase{
	{
		// run_tests.py: 44_rename / patches/44_rename.ap
		name:  "44_rename_moves_both_a_file_and_a_directory",
		files: map[string]string{"44_rename_src.txt": "This file will be renamed."},
		dirs:  []string{"44_rename_dir"},
		patch: "t3st0044 AP 3.1\n\n" +
			"t3st0044 FILE\n44_rename_src.txt\n\n" +
			"t3st0044 RENAME\n44_renamed.txt\n\n" +
			"t3st0044 FILE\n44_rename_dir\n\n" +
			"t3st0044 RENAME\n44_renamed_dir\n",
		expectFiles:   map[string]string{"44_renamed.txt": "This file will be renamed."},
		expectMissing: []string{"44_rename_src.txt", "44_rename_dir"},
		expectDirs:    []string{"44_renamed_dir"},
	},
	{
		// run_tests.py: 45_create_dir / patches/45_create_dir.ap - a FILE
		// path ending in "/" plus a bare CREATE (no content) creates an
		// empty directory instead of a file.
		name:       "45_create_dir_bare_CREATE_with_a_trailing_slash_makes_a_directory",
		patch:      "t3st0045 AP 3.1\n\nt3st0045 FILE\nnew_dir/\n\nt3st0045 CREATE\n",
		expectDirs: []string{"new_dir"},
	},
	{
		// run_tests.py: 46_delete_file_dir / patches/46_delete_file_dir.ap
		name:  "46_delete_file_dir_a_bare_DELETE_removes_a_whole_file_and_a_whole_directory",
		files: map[string]string{"to_be_deleted.txt": "bye\n"},
		dirs:  []string{"to_be_deleted_dir"},
		patch: "t3st0046 AP 3.1\n\n" +
			"t3st0046 FILE\nto_be_deleted.txt\n\n" +
			"t3st0046 DELETE\n\n" +
			"t3st0046 FILE\nto_be_deleted_dir\n\n" +
			"t3st0046 DELETE\n",
		expectMissing: []string{"to_be_deleted.txt", "to_be_deleted_dir"},
	},
	{
		// run_tests.py: 47_error_mixed_atomic_delete / patches/47_error_mixed_atomic_delete.ap
		// A bare DELETE mixed with another modification in the same FILE
		// block is not the whole-file-delete shorthand (that requires
		// DELETE to be the block's ONLY modification): it is a DELETE
		// with no locators, which is INVALID_MODIFICATION, and in strict
		// mode fails the whole patch atomically before anything is written.
		name:   "47_error_mixed_atomic_delete_bare_DELETE_alongside_another_edit_is_invalid_not_a_whole_file_delete",
		files:  map[string]string{"47_atomic_delete_source.txt": "Line to change.\n"},
		strict: true,
		patch: "47a00047 AP 3.1\n\n" +
			"47a00047 FILE\n47_atomic_delete_source.txt\n\n" +
			"47a00047 REPLACE\n47a00047 snippet\nLine to change.\n47a00047 content\nThis change should not be applied.\n\n" +
			"47a00047 DELETE\n",
		expectStatus: StatusFailed,
		expectError:  ErrInvalidModification,
		expectFiles:  map[string]string{"47_atomic_delete_source.txt": "Line to change.\n"},
	},
	{
		// run_tests.py: 48_comprehensive_delete / patches/48_comprehensive_delete.ap
		// All three DELETE variants in one patch: a snippet-scoped delete
		// (with include_leading_blank_lines), a whole-file delete and a
		// whole-directory delete.
		name: "48_comprehensive_delete_snippet_whole_file_and_whole_directory_DELETE_together",
		files: map[string]string{
			"48_source.py": "# This is a source file for a comprehensive DELETE test.\n\n" +
				"def main_function():\n    \"\"\"This function should remain.\"\"\"\n    print(\"Hello from main!\")\n\n\n" +
				"def function_to_delete():\n    \"\"\"This function will be removed.\"\"\"\n    pass",
			"48_file_to_delete.txt": "bye\n",
		},
		dirs: []string{"48_dir_to_delete"},
		patch: "test0048 AP 3.1\n\n" +
			"test0048 FILE\n48_source.py\n\n" +
			"test0048 DELETE\ntest0048 snippet\n" +
			"def function_to_delete():\n    \"\"\"This function will be removed.\"\"\"\n    pass\n" +
			"test0048 include_leading_blank_lines 2\n\n" +
			"test0048 FILE\n48_file_to_delete.txt\n\n" +
			"test0048 DELETE\n\n" +
			"test0048 FILE\n48_dir_to_delete\n\n" +
			"test0048 DELETE\n",
		expectFiles: map[string]string{
			"48_source.py": "# This is a source file for a comprehensive DELETE test.\n\n" +
				"def main_function():\n    \"\"\"This function should remain.\"\"\"\n    print(\"Hello from main!\")\n",
		},
		expectMissing: []string{"48_file_to_delete.txt", "48_dir_to_delete"},
	},
	{
		// run_tests.py: 51_rename_create_dir / patches/51_rename_create_dir.ap
		name:  "51_rename_create_dir_RENAME_creates_missing_parent_directories",
		files: map[string]string{"51_rename_create_dir.txt": "This file will be renamed into a new directory."},
		patch: "test0051 AP 3.1\n\ntest0051 FILE\n51_rename_create_dir.txt\n\ntest0051 RENAME\nnew_parent_dir/renamed.txt\n",
		expectFiles: map[string]string{
			"new_parent_dir/renamed.txt": "This file will be renamed into a new directory.",
		},
		expectMissing: []string{"51_rename_create_dir.txt"},
	},
	{
		// run_tests.py: 55_rename_idempotency / patches/55_rename_idempotency.ap
		// The source is already gone and the destination already exists:
		// treat the RENAME as already done rather than failing.
		name:  "55_rename_idempotency_is_a_no_op_when_the_source_is_gone_and_the_destination_exists",
		files: map[string]string{"55_renamed.txt": "This file exists."},
		patch: "552e4a31 AP 3.1\n\n552e4a31 FILE\n55_rename_src.txt\n\n552e4a31 RENAME\n55_renamed.txt\n",
		expectFiles: map[string]string{
			"55_renamed.txt": "This file exists.",
		},
		expectMissing: []string{"55_rename_src.txt"},
	},
	{
		// run_tests.py: 59_error_create_file_on_dir / patches/59_error_create_file_on_dir.ap
		name:   "59_error_create_file_on_dir_CREATE_fails_when_the_target_path_is_an_existing_directory",
		dirs:   []string{"59_a_directory"},
		strict: true,
		patch: "59e22059 AP 3.1\n\n59e22059 FILE\n59_a_directory\n\n" +
			"59e22059 CREATE\n59e22059 content\nThis content should fail to write.\n",
		expectStatus: StatusFailed,
		expectError:  ErrFileWriteError,
		expectDirs:   []string{"59_a_directory"},
	},
	{
		// run_tests.py: 35_safe_create_empty / patches/35_safe_create_empty.ap
		// CREATE overwrites an existing but EMPTY file - this uses CREATE's
		// "hybrid" form (no FILE block; the path is the bare line right
		// after CREATE, parse.go's CREATE_PATH reading key).
		name:  "35_safe_create_empty_CREATE_overwrites_an_existing_empty_file",
		files: map[string]string{"35_safe_create_empty.txt": ""},
		patch: "# Test overwrite of empty file.\n" +
			"35a00999 AP 3.1\n35a00999 CREATE\n35_safe_create_empty.txt\n35a00999 content\nFilled.\n",
		// The existing file is empty, so there is no line ending to detect:
		// the written content falls back to the OS default (§2.4, see
		// osLineSep and c28 in cases_test.go), "\r\n" on Windows.
		expectFiles: map[string]string{"35_safe_create_empty.txt": "Filled." + osLineSep()},
	},
	{
		// run_tests.py: 36_safe_create_fail / patches/36_safe_create_fail.ap
		// CREATE refuses to overwrite an existing NON-EMPTY file whose
		// content does not already match.
		name:   "36_safe_create_fail_CREATE_refuses_to_overwrite_a_non_empty_file",
		files:  map[string]string{"36_safe_create_fail.txt": "Not empty."},
		strict: true,
		patch: "# Test fail overwrite of non-empty file.\n" +
			"36a00aaa AP 3.1\n\n36a00aaa CREATE\n36_safe_create_fail.txt\n36a00aaa content\nShould fail.\n",
		expectStatus: StatusFailed,
		expectError:  ErrFileExists,
		expectFiles:  map[string]string{"36_safe_create_fail.txt": "Not empty."},
	},
	{
		// run_tests.py: 20_error_path_traversal / patches/20_error_path_traversal.ap
		name:   "20_error_path_traversal_a_leading_..%2F_is_rejected",
		strict: true,
		patch: "20e22027 AP 3.1\n\n20e22027 FILE\n../path_traversal_attack.txt\n\n" +
			"20e22027 CREATE\n20e22027 content\nThis file should not be created.\n",
		expectStatus: StatusFailed,
		expectError:  ErrInvalidFilePath,
	},
	{
		// run_tests.py: 17_error_file_not_found / patches/17_error_file_not_found.ap
		name:   "17_error_file_not_found_REPLACE_on_a_file_that_does_not_exist",
		strict: true,
		patch: "17e22026 AP 3.1\n\n17e22026 FILE\nnon_existent_file.txt\n\n" +
			"17e22026 REPLACE\n17e22026 snippet\nsome text\n17e22026 content\nnew text\n",
		expectStatus: StatusFailed,
		expectError:  ErrFileNotFound,
	},
	{
		// run_tests.py: 16_empty_actions / patches/16_empty_actions.ap - unlike
		// c34 in cases.py (the same shape in non-strict mode, where an empty
		// REPLACE content is tolerated as a DELETE shorthand), strict mode
		// refuses an empty REPLACE content outright.
		name:   "16_empty_actions_strict_mode_rejects_REPLACE_with_empty_content",
		files:  map[string]string{"16_empty_actions.txt": "Line 1 to be replaced with nothing.\nLine 2 to have nothing inserted after.\nLine 3 to be deleted.\n"},
		strict: true,
		patch: "16e3971e AP 3.1\n\n16e3971e FILE\n16_empty_actions.txt\n\n" +
			"16e3971e REPLACE\n16e3971e snippet\nLine 1 to be replaced with nothing.\n16e3971e content\n\n" +
			"16e3971e INSERT_AFTER\n16e3971e snippet\nLine 2 to have nothing inserted after.\n16e3971e content\n\n" +
			"16e3971e DELETE\n16e3971e snippet\nLine 3 to be deleted.\n",
		expectStatus: StatusFailed,
		expectError:  ErrEmptyReplace,
		expectFiles:  map[string]string{"16_empty_actions.txt": "Line 1 to be replaced with nothing.\nLine 2 to have nothing inserted after.\nLine 3 to be deleted.\n"},
	},
	{
		// run_tests.py: 61_tolerant_comments / patches/61_tolerant_comments.ap -
		// non-strict mode strips "#" comment lines wherever they appear,
		// including before the AP header and between directives.
		name: "61_tolerant_comments_non_strict_mode_strips_hash_comments_around_directives",
		patch: "# comment before header\n61a00061 AP 3.1\n# comment between\n" +
			"61a00061 FILE\ntolerant.txt\n# another comment\n61a00061 CREATE\n61a00061 content\nWorks!\n",
		// A brand-new file with no declared newline mode: OS default ending.
		expectFiles: map[string]string{"tolerant.txt": "Works!" + osLineSep()},
	},
	{
		// run_tests.py: 62_tolerant_missing_header / patches/62_tolerant_missing_header.ap
		name:  "62_tolerant_missing_header_non_strict_mode_auto_detects_the_patch_ID_without_an_AP_header",
		patch: "# This patch completely lacks the AP 3.1 header\n62a00062 FILE\ntolerant.txt\n\n62a00062 CREATE\n62a00062 content\nWorks!\n",
		// A brand-new file with no declared newline mode: OS default ending.
		expectFiles: map[string]string{
			"tolerant.txt": "Works!" + osLineSep(),
		},
	},
	{
		// run_tests.py: 63_tolerant_anchor_as_snippet / patches/63_tolerant_anchor_as_snippet.ap -
		// non-strict mode accepts an `anchor` used in place of a missing
		// `snippet` on a REPLACE.
		name:  "63_tolerant_anchor_as_snippet_non_strict_mode_accepts_anchor_as_the_snippet",
		files: map[string]string{"63_source.txt": "Line 1"},
		patch: "63a00063 AP 3.1\n\n63a00063 FILE\n63_source.txt\n\n63a00063 REPLACE\n63a00063 anchor\nLine 1\n63a00063 content\nReplaced Line 1\n",
		// The source has a single line with no terminator, so no line
		// ending is detected and the OS default is used.
		expectFiles: map[string]string{
			"63_source.txt": "Replaced Line 1" + osLineSep(),
		},
	},
	{
		// run_tests.py: 64_strict_rejects_comments / patches/64_strict_rejects_comments.ap -
		// same patch text as 61, but strict mode does not strip comment
		// lines, so they are parsed as unrecognised directives.
		name:   "64_strict_rejects_comments_strict_mode_does_not_tolerate_hash_comments",
		strict: true,
		patch: "# comment before header\n61a00061 AP 3.1\n# comment between\n" +
			"61a00061 FILE\ntolerant.txt\n# another comment\n61a00061 CREATE\n61a00061 content\nWorks!\n",
		expectStatus: StatusFailed,
		expectError:  ErrInvalidPatchFile,
	},
	{
		// run_tests.py: 65_strict_rejects_missing_header / patches/65_strict_rejects_missing_header.ap -
		// same patch text as 62, but strict mode requires an explicit
		// AP header and refuses to auto-detect the patch ID.
		name:         "65_strict_rejects_missing_header_strict_mode_requires_an_explicit_AP_header",
		strict:       true,
		patch:        "# This patch completely lacks the AP 3.1 header\n62a00062 FILE\ntolerant.txt\n\n62a00062 CREATE\n62a00062 content\nWorks!\n",
		expectStatus: StatusFailed,
		expectError:  ErrInvalidPatchFile,
	},
	{
		// run_tests.py: 66_strict_rejects_anchor_as_snippet / patches/66_strict_rejects_anchor_as_snippet.ap -
		// same patch text as 63, but strict mode never substitutes `anchor`
		// for a missing `snippet`, so the REPLACE has no locator at all.
		name:         "66_strict_rejects_anchor_as_snippet_strict_mode_never_uses_anchor_as_the_snippet",
		files:        map[string]string{"63_source.txt": "Line 1"},
		strict:       true,
		patch:        "63a00063 AP 3.1\n\n63a00063 FILE\n63_source.txt\n\n63a00063 REPLACE\n63a00063 anchor\nLine 1\n63a00063 content\nReplaced Line 1\n",
		expectStatus: StatusFailed,
		expectError:  ErrInvalidModification,
		expectFiles:  map[string]string{"63_source.txt": "Line 1"},
	},
}

func TestLegacyCases(t *testing.T) {
	for _, c := range legacyCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			runLegacyCase(t, c)
		})
	}
}
