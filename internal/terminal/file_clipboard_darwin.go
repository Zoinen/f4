//go:build darwin

package terminal

import (
	"context"
	"os/exec"
)

// darwinFileClipboardScript lists the files on the general pasteboard as a JSON
// array of absolute paths. Finder offers a copied selection as file URLs next
// to the plain names, so the text on the pasteboard alone cannot tell "these
// files" from "this string"; AppKit reads the URLs themselves. An empty
// pasteboard, text or an image give "[]".
const darwinFileClipboardScript = `ObjC.import('AppKit');
var out = [];
try {
	var pb = $.NSPasteboard.generalPasteboard;
	var opts = $.NSDictionary.dictionaryWithObjectForKey($(true), $.NSPasteboardURLReadingFileURLsOnlyKey);
	var urls = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), opts);
	var n = urls.count;
	for (var i = 0; i < n; i++) {
		out.push(ObjC.unwrap(urls.objectAtIndex(i).path));
	}
} catch (e) {}
JSON.stringify(out);`

// readDarwinFileClipboard reads the files Finder (or any other application)
// copied. Finder has no cut: its move is a paste with Option, so the second
// result is always false.
func readDarwinFileClipboard(ctx context.Context) ([]string, bool, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil, false, errFileClipboardUnavailable
	}
	out, err := exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", darwinFileClipboardScript).Output() // #nosec G204 -- fixed script, no input is interpolated.
	if err != nil {
		return nil, false, errFileClipboardUnavailable
	}
	paths := parseFileClipboardJSON(out)
	if len(paths) == 0 {
		return nil, false, errFileClipboardUnavailable
	}
	return paths, false, nil
}
