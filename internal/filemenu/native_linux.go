package filemenu

import (
	"os"
	"os/exec"
	"sort"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

type gioList struct {
	Data       uintptr
	Next, Prev *gioList
}

const Platform = "linux"

func showNative(r Request) (result Result) {
	result.Outcome = Unavailable
	if r.Operation == "open" {
		for _, path := range r.Paths {
			if err := exec.Command("xdg-open", path).Run(); err != nil {
				return Result{Outcome: Failed, Error: err.Error()}
			}
		}
		return Result{Outcome: Invoked}
	}
	if r.Operation != "applications" && r.Operation != "open-with" {
		return result
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return result
	}
	lib, err := purego.Dlopen("libgio-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return result
	}
	defer purego.Dlclose(lib)
	// Older/minimal desktop installations may lack desktop-app-info. Missing
	// optional symbols mean no Open With submenu, not a broken file menu.
	defer func() {
		if recover() != nil {
			result = Result{Outcome: Unavailable}
		}
	}()
	var guess func(string, uintptr, uintptr, *int32) uintptr
	var duplicateString func(string) uintptr
	var all func(uintptr) *gioList
	var appID, appName func(uintptr) string
	var unref, free func(uintptr)
	var freeList func(*gioList)
	purego.RegisterLibFunc(&guess, lib, "g_content_type_guess")
	purego.RegisterLibFunc(&duplicateString, lib, "g_strdup")
	purego.RegisterLibFunc(&all, lib, "g_app_info_get_all_for_type")
	purego.RegisterLibFunc(&appID, lib, "g_app_info_get_id")
	purego.RegisterLibFunc(&appName, lib, "g_app_info_get_display_name")
	purego.RegisterLibFunc(&unref, lib, "g_object_unref")
	purego.RegisterLibFunc(&free, lib, "g_free")
	purego.RegisterLibFunc(&freeList, lib, "g_list_free")
	common := map[string]Entry{}
	for index, path := range r.Paths {
		var uncertain int32
		contentType := guess(path, 0, 0, &uncertain)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			free(contentType)
			contentType = duplicateString("inode/directory")
		}
		if contentType == 0 {
			return result
		}
		list := all(contentType)
		free(contentType)
		current := map[string]Entry{}
		for node := list; node != nil; node = node.Next {
			id := appID(node.Data)
			if id != "" {
				current[id] = Entry{ID: "app:" + id, Label: appName(node.Data)}
			}
			unref(node.Data)
		}
		freeList(list)
		if index == 0 {
			common = current
		} else {
			for id := range common {
				if _, ok := current[id]; !ok {
					delete(common, id)
				}
			}
		}
	}
	if r.Operation == "applications" {
		result.Outcome = Selected
		for _, entry := range common {
			result.Entries = append(result.Entries, entry)
		}
		sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Label < result.Entries[j].Label })
		return result
	}
	id := strings.TrimPrefix(r.Application, "app:")
	if _, ok := common[id]; !ok {
		return Result{Outcome: Failed, Error: "Application no longer supports this selection"}
	}
	var create func(string) uintptr
	var file func(string) uintptr
	var appendList func(*gioList, uintptr) *gioList
	var launch func(uintptr, *gioList, uintptr, *uintptr) int32
	var errorFree func(uintptr)
	purego.RegisterLibFunc(&create, lib, "g_desktop_app_info_new")
	purego.RegisterLibFunc(&file, lib, "g_file_new_for_path")
	purego.RegisterLibFunc(&appendList, lib, "g_list_append")
	purego.RegisterLibFunc(&launch, lib, "g_app_info_launch")
	purego.RegisterLibFunc(&errorFree, lib, "g_error_free")
	app := create(id)
	if app == 0 {
		return Result{Outcome: Failed, Error: "Application is unavailable"}
	}
	defer unref(app)
	var files *gioList
	for _, path := range r.Paths {
		files = appendList(files, file(path))
	}
	defer func() {
		for node := files; node != nil; node = node.Next {
			unref(node.Data)
		}
		freeList(files)
	}()
	var launchErr uintptr
	if launch(app, files, 0, &launchErr) == 0 {
		if launchErr != 0 {
			errorFree(launchErr)
		}
		return Result{Outcome: Failed, Error: "Desktop application launch failed"}
	}
	return Result{Outcome: Invoked}
}

// Keep the GList layout tied to pointer-sized fields on every Linux target.
var _ = unsafe.Sizeof(gioList{})
