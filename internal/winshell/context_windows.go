//go:build windows

package winshell

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/zzl/go-win32api/v2/win32"
)

const contextCommandFirst uint32 = 1

type nativeContextMenu struct {
	context *win32.IContextMenu
	menu    win32.HMENU
}

func (m *nativeContextMenu) close() {
	if m == nil {
		return
	}
	if m.menu != 0 {
		_, _ = win32.DestroyMenu(m.menu)
		m.menu = 0
	}
	if m.context != nil {
		m.context.Release()
		m.context = nil
	}
}

type nativeContextState struct {
	next  uint64
	menus map[uint64]*nativeContextMenu
}

func newNativeContextState() *nativeContextState {
	return &nativeContextState{menus: make(map[uint64]*nativeContextMenu)}
}

func (s *nativeContextState) open(parsingName string) (ContextMenu, error) {
	native, commands, err := createNativeContextMenu(parsingName)
	if err != nil {
		return ContextMenu{}, err
	}
	s.next++
	if s.next == 0 {
		s.next++
	}
	s.menus[s.next] = native
	return ContextMenu{Token: s.next, Commands: commands}, nil
}

func (s *nativeContextState) invoke(token uint64, commandID uint32) error {
	native := s.menus[token]
	if native == nil {
		return fmt.Errorf("Windows Shell context menu has expired")
	}
	delete(s.menus, token)
	defer native.close()
	if commandID < contextCommandFirst {
		return fmt.Errorf("invalid Windows Shell context command")
	}
	info := win32.CMINVOKECOMMANDINFO{
		CbSize: uint32(unsafe.Sizeof(win32.CMINVOKECOMMANDINFO{})),
		LpVerb: (*byte)(unsafe.Pointer(uintptr(commandID - contextCommandFirst))),
		NShow:  int32(win32.SW_SHOWNORMAL),
	}
	hr := native.context.InvokeCommand(&info)
	if win32.FAILED(hr) {
		return shellHRESULT("invoke Windows Shell context command", hr)
	}
	return nil
}

func (s *nativeContextState) dismiss(token uint64) {
	if native := s.menus[token]; native != nil {
		delete(s.menus, token)
		native.close()
	}
}

func (s *nativeContextState) close() {
	for token, native := range s.menus {
		delete(s.menus, token)
		native.close()
	}
}

func createNativeContextMenu(parsingName string) (*nativeContextMenu, []ContextCommand, error) {
	var absolute *win32.ITEMIDLIST
	hr := win32.SHParseDisplayName(win32.StrToPwstr(parsingName), nil, &absolute, 0, nil)
	if win32.FAILED(hr) || absolute == nil {
		return nil, nil, shellHRESULT("parse Windows Shell context item", hr)
	}
	defer win32.CoTaskMemFree(unsafe.Pointer(absolute))

	var parent *win32.IShellFolder
	var relative *win32.ITEMIDLIST
	hr = win32.SHBindToParent(absolute, &win32.IID_IShellFolder, unsafe.Pointer(&parent), &relative)
	if win32.FAILED(hr) || parent == nil || relative == nil {
		return nil, nil, shellHRESULT("bind Windows Shell context parent", hr)
	}
	defer parent.Release()

	var contextMenu *win32.IContextMenu
	hr = parent.GetUIObjectOf(0, 1, &relative, &win32.IID_IContextMenu, nil, unsafe.Pointer(&contextMenu))
	if win32.FAILED(hr) || contextMenu == nil {
		return nil, nil, shellHRESULT("create Windows Shell context menu", hr)
	}
	menu, menuErr := win32.CreatePopupMenu()
	if menu == 0 {
		contextMenu.Release()
		return nil, nil, fmt.Errorf("create Windows Shell popup menu: %v", menuErr)
	}
	native := &nativeContextMenu{context: contextMenu, menu: menu}
	hr = contextMenu.QueryContextMenu(menu, 0, contextCommandFirst, 0x7fff,
		win32.CMF_NORMAL|win32.CMF_EXTENDEDVERBS)
	if win32.FAILED(hr) {
		native.close()
		return nil, nil, shellHRESULT("query Windows Shell context menu", hr)
	}
	commands := readNativeMenu(contextMenu, menu, 0)
	if len(commands) == 0 {
		native.close()
		return nil, nil, fmt.Errorf("Windows Shell item has no context commands")
	}
	return native, commands, nil
}

func readNativeMenu(contextMenu *win32.IContextMenu, menu win32.HMENU, depth int) []ContextCommand {
	if menu == 0 || depth > 8 {
		return nil
	}
	count, _ := win32.GetMenuItemCount(menu)
	if count <= 0 {
		return nil
	}
	commands := make([]ContextCommand, 0, count)
	for index := int32(0); index < count; index++ {
		buffer := make([]uint16, 1024)
		info := win32.MENUITEMINFOW{
			CbSize:     uint32(unsafe.Sizeof(win32.MENUITEMINFOW{})),
			FMask:      win32.MIIM_FTYPE | win32.MIIM_ID | win32.MIIM_STATE | win32.MIIM_STRING | win32.MIIM_SUBMENU,
			DwTypeData: &buffer[0],
			Cch:        uint32(len(buffer) - 1),
		}
		ok, _ := win32.GetMenuItemInfoW(menu, uint32(index), 1, &info)
		if ok == 0 {
			continue
		}
		command := ContextCommand{
			ID:        info.WID,
			Separator: info.FType&win32.MFT_SEPARATOR != 0,
			Enabled:   info.FState&win32.MFS_DISABLED == 0,
			Default:   info.FState&win32.MFS_DEFAULT != 0,
		}
		if command.Separator {
			commands = append(commands, command)
			continue
		}
		command.Text = strings.TrimSpace(syscall.UTF16ToString(buffer))
		if info.HSubMenu != 0 {
			command.Children = readNativeMenu(contextMenu, info.HSubMenu, depth+1)
		}
		if command.Text == "" && command.ID >= contextCommandFirst {
			command.Text = nativeCommandVerb(contextMenu, command.ID-contextCommandFirst)
		}
		if command.Text == "" {
			command.Text = fmt.Sprintf("Command %d", command.ID)
		}
		commands = append(commands, command)
	}
	return trimContextSeparators(commands)
}

func nativeCommandVerb(contextMenu *win32.IContextMenu, offset uint32) string {
	buffer := make([]uint16, 256)
	hr := contextMenu.GetCommandString(uintptr(offset), win32.GCS_VERBW, nil,
		(*byte)(unsafe.Pointer(&buffer[0])), uint32(len(buffer)))
	if win32.FAILED(hr) {
		return ""
	}
	return syscall.UTF16ToString(buffer)
}

func trimContextSeparators(commands []ContextCommand) []ContextCommand {
	for len(commands) > 0 && commands[0].Separator {
		commands = commands[1:]
	}
	for len(commands) > 0 && commands[len(commands)-1].Separator {
		commands = commands[:len(commands)-1]
	}
	return commands
}
