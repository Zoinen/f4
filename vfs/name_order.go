package vfs

import "strings"

// FoldNameKey prepares the case-insensitive key once for bulk sorting. ASCII
// lowercase names reuse their original string storage.
func FoldNameKey(value string) string {
	needsASCII := false
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 0x80 {
			return strings.ToLower(value)
		}
		if character >= 'A' && character <= 'Z' {
			needsASCII = true
		}
	}
	if !needsASCII {
		return value
	}
	folded := []byte(value)
	for index, character := range folded {
		if character >= 'A' && character <= 'Z' {
			folded[index] = character + ('a' - 'A')
		}
	}
	return string(folded)
}

// CompareFoldedNames compares names case-insensitively without allocating for
// the overwhelmingly common ASCII path.  Keeping this ordering in the VFS
// package lets a bounded local-directory window prove that it has exactly the
// same order as the panel which will later consume the complete catalog.
func CompareFoldedNames(left, right string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		leftByte, rightByte := left[index], right[index]
		if leftByte >= 0x80 || rightByte >= 0x80 {
			leftFolded, rightFolded := strings.ToLower(left), strings.ToLower(right)
			return strings.Compare(leftFolded, rightFolded)
		}
		if leftByte >= 'A' && leftByte <= 'Z' {
			leftByte += 'a' - 'A'
		}
		if rightByte >= 'A' && rightByte <= 'Z' {
			rightByte += 'a' - 'A'
		}
		if leftByte < rightByte {
			return -1
		}
		if leftByte > rightByte {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

// CompareNames adds a deterministic case-sensitive tie-breaker to the folded
// ordering used by file panels.
func CompareNames(left, right string) int {
	if folded := CompareFoldedNames(left, right); folded != 0 {
		return folded
	}
	return strings.Compare(left, right)
}
