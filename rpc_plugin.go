package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type HighlightReq struct {
	Line string
	Prev any
	Base uint64
}

type HighlightRes struct {
	Attrs []uint64
	Next  any
}

type ProgressTaskReq struct {
	Title    string
	StartMsg string
	Forked   bool
}

type ProgressUpdateReq struct {
	Msg     string
	Percent int
}

type HotkeyReq struct {
	VK   uint16
	Mods uint32
}

type OpenReq struct {
	Drive string
	Path  string
}

type OpenRes struct {
	ID   uint32
	Size int64
}

type ReadAtReq struct {
	ID  uint32
	Len int
	Off int64
}

type WriteReq struct {
	ID   uint32
	Data []byte
}

type CloseReq struct {
	ID uint32
}

type MkDirReq struct {
	Drive string
	Path  string
}

type RemoveReq struct {
	Drive string
	Path  string
}

type RenameReq struct {
	Drive string
	Old   string
	New   string
}
type AskOverwriteReq struct {
	Path string
	Src  vfs.VFSItem
	Dst  vfs.VFSItem
}

type AskOverwriteRes struct {
	Choice   int
	Remember bool
}

type AskErrorReq struct {
	Op  string
	Err string
}
type SetAttrReq struct {
	Drive string
	Path  string
	Item  vfs.VFSItem
}

type InputBoxReq struct {
	Title   string
	Prompt  string
	Default string
}

type MenuReq struct {
	Title string
	Items []string
}

// RPCPlugin manages the lifecycle of an external process plugin.
type RPCPlugin struct {
	path     string
	dir      string
	cmd      *exec.Cmd
	sess     *f4rpc.Session
	api      vfs.HostAPI
	closing  bool
	identity PluginIdentity
}

// SetPermissionIdentity passes on who the manifest says this plugin is.
func (p *RPCPlugin) SetPermissionIdentity(identity PluginIdentity) {
	p.identity = identity
}

// permissionIdentity falls back to the path for a plugin registered by hand.
func (p *RPCPlugin) permissionIdentity() PluginIdentity {
	if p.identity.Key == "" {
		return PermissionIdentityForPath(p.path)
	}
	return p.identity
}

func NewRPCPlugin(path string) *RPCPlugin {
	return &RPCPlugin{path: path}
}

// NewRPCPlugRing creates a plugin running a specific command within a specific directory.
func NewRPCPlugRing(dir string, cmd string) *RPCPlugin {
	return &RPCPlugin{path: cmd, dir: dir}
}

func (p *RPCPlugin) GetName() string {
	return p.path + " (RPC)"
}

func (p *RPCPlugin) Init(api vfs.HostAPI) error {
	p.api = api

	gate := newPluginGate(p.permissionIdentity())
	if err := gate.Allow(PermissionNative, "run as an external process"); err != nil {
		return err
	}

	parts := strings.Fields(p.path)
	if len(parts) == 0 {
		return fmt.Errorf("empty entrypoint")
	}

	bin := parts[0]
	// Fix Go exec.Command trap: if binary path is relative, resolve it against p.dir
	if p.dir != "" && !filepath.IsAbs(bin) {
		localBin := filepath.Join(p.dir, bin)
		if _, err := os.Stat(localBin); err == nil {
			bin = localBin
		}
	}

	p.cmd = exec.Command(bin, parts[1:]...)
	if p.dir != "" {
		p.cmd.Dir = p.dir
	}

	stdin, err := p.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return err
	}

	// Forward stderr to global debug log
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			vtui.DebugLog("PLUGIN_STDERR [%s]: %s", p.path, scanner.Text())
		}
	}()

	if err := p.cmd.Start(); err != nil {
		return err
	}

	p.sess = f4rpc.NewSession(stdout, stdin)

	return startPluginSession(p.sess, api, p.path, nil, func(err error) {
		if !p.closing {
			vtui.DebugLog("RPC Plugin %q terminated unexpectedly: %v", p.path, err)
		}
	})
}

func (p *RPCPlugin) Close() error {
	p.closing = true
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	return nil
}
