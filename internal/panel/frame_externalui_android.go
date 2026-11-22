package panel

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func AssociatedFileCommand(path string) (string, []string, bool) {
	// Android parses the broadcast data as a URI, so encode path characters
	// such as '#' and '%' before handing the file to Termux's provider.
	fileURL := url.URL{Scheme: "file", Path: path}
	return "termux-open", []string{fileURL.String()}, true
}

func openAssociatedPathOS(path, dir string) error {
	item, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !item.IsDir() {
		command, args, _ := AssociatedFileCommand(path)
		return runExternalUICommandOS(command, args, dir)
	}

	// termux-open shares regular files. Use the Android documents provider
	// for directories in primary shared storage, including Termux's symlinks.
	shared := os.Getenv("EXTERNAL_STORAGE")
	if shared == "" {
		shared = "/sdcard"
	}
	shared, err = filepath.EvalSymlinks(shared)
	if err != nil {
		return err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	uri, err := androidSharedDirectoryURI(shared, path)
	if err != nil {
		return err
	}
	return runExternalUICommandOS("am", []string{
		"start", "-a", "android.intent.action.VIEW", "-d", uri,
		"-t", "vnd.android.document/directory",
	}, dir)
}

func androidSharedDirectoryURI(shared, path string) (string, error) {
	relative, err := filepath.Rel(shared, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("Android can only open directories in primary shared storage from Termux")
	}
	if relative == "." {
		relative = ""
	}
	id := url.PathEscape("primary:" + filepath.ToSlash(relative))
	return "content://com.android.externalstorage.documents/document/" + id, nil
}
