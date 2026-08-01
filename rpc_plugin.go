package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
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
	path    string
	dir     string
	cmd     *exec.Cmd
	sess    *f4rpc.Session
	api     vfs.HostAPI
	closing bool
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

	// Register Host API methods for the plugin to call
	p.sess.Register("Host.Log", func(data msgpack.RawMessage) (any, error) {
		var msg string
		msgpack.Unmarshal(data, &msg)
		api.Log(msg)
		return nil, nil
	})
	p.sess.Register("Host.Message", func(data msgpack.RawMessage) (any, error) {
		var msg string
		msgpack.Unmarshal(data, &msg)
		api.Message(msg)
		return nil, nil
	})
	p.sess.Register("Host.GetVersion", func(data msgpack.RawMessage) (any, error) {
		return api.GetVersion(), nil
	})
	p.sess.Register("Host.RegisterHighlighter", func(data msgpack.RawMessage) (any, error) {
		api.RegisterHighlighter(&rpcHighlighterProvider{p})
		return nil, nil
	})

	p.sess.Register("Host.RegisterGlobalHotkey", func(data msgpack.RawMessage) (any, error) {
		var req HotkeyReq
		msgpack.Unmarshal(data, &req)
		api.RegisterGlobalHotkey(req.VK, vtinput.ControlKeyState(req.Mods), func(app vfs.App) {
			_ = p.sess.Call("Plugin.OnHotkey", req, nil)
		})
		return nil, nil
	})

	// Session-local state for active progress task

	// Session-local state for active progress task
	var taskUpdate func(string, int)
	var taskCtx context.Context
	var taskAnchor vtui.Frame

	p.sess.Register("Host.RunProgressTask", func(data msgpack.RawMessage) (any, error) {
		var req ProgressTaskReq
		msgpack.Unmarshal(data, &req)
		vtui.FrameManager.PostTask(func() {
			var pf *PanelsFrame
			if len(vtui.FrameManager.Screens) > 0 {
				for _, f := range vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx].Frames {
					if p, ok := f.(*PanelsFrame); ok {
						pf = p
						break
					}
				}
			}
			if pf == nil {
				return
			}

			pf.RunProgressTask(req.Title, req.StartMsg, req.Forked, func(ctx context.Context, update func(msg string, percent int)) error {
				taskUpdate = update
				taskCtx = ctx
				taskAnchor = vtui.FrameManager.GetTopFrame()
				return p.sess.Call("Plugin.OnProgressTask", nil, nil)
			}, func(err error) {
				taskUpdate = nil
				taskCtx = nil
				taskAnchor = nil
			})
		})
		return nil, nil
	})

	p.sess.Register("Host.UpdateProgress", func(data msgpack.RawMessage) (any, error) {
		var req ProgressUpdateReq
		msgpack.Unmarshal(data, &req)
		if taskUpdate != nil {
			taskUpdate(req.Msg, req.Percent)
		}
		return nil, nil
	})

	p.sess.Register("Host.IsProgressCancelled", func(data msgpack.RawMessage) (any, error) {
		if taskCtx != nil {
			return taskCtx.Err() != nil, nil
		}
		return false, nil
	})

	p.sess.Register("Host.AskOverwrite", func(data msgpack.RawMessage) (any, error) {
		var req AskOverwriteReq
		msgpack.Unmarshal(data, &req)
		ctx := taskCtx
		if ctx == nil {
			ctx = context.Background()
		}
		choice, remember := AskOverwrite(ctx, req.Path, req.Src, req.Dst, taskAnchor)
		return AskOverwriteRes{Choice: choice, Remember: remember}, nil
	})

	p.sess.Register("Host.AskError", func(data msgpack.RawMessage) (any, error) {
		var req AskErrorReq
		msgpack.Unmarshal(data, &req)
		ctx := taskCtx
		if ctx == nil {
			ctx = context.Background()
		}
		choice := AskError(ctx, req.Op, fmt.Errorf("%s", req.Err), taskAnchor)
		return choice, nil
	})

	p.sess.Register("Host.InputBox", func(data msgpack.RawMessage) (any, error) {
		var req InputBoxReq
		msgpack.Unmarshal(data, &req)
		resChan := make(chan string, 1)
		vtui.FrameManager.PostTask(func() {
			vtui.InputBox(req.Title, req.Prompt, req.Default, func(s string) {
				resChan <- s
			})
		})
		return <-resChan, nil
	})

	p.sess.Register("Host.Menu", func(data msgpack.RawMessage) (any, error) {
		var req MenuReq
		msgpack.Unmarshal(data, &req)
		resChan := make(chan int, 1)
		vtui.FrameManager.PostTask(func() {
			// Find PanelsFrame for context-aware menu
			var pf *PanelsFrame
			if len(vtui.FrameManager.Screens) > 0 {
				for _, f := range vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx].Frames {
					if p, ok := f.(*PanelsFrame); ok {
						pf = p
						break
					}
				}
			}
			if pf != nil {
				pf.Menu(req.Title, req.Items, func(idx int) { resChan <- idx })
			} else {
				resChan <- -1
			}
		})
		return <-resChan, nil
	})

	go func() {
		err := p.sess.Serve()
		if !p.closing {
			vtui.DebugLog("RPC Plugin %q terminated unexpectedly: %v", p.path, err)
		}
	}()

	// Query plugin for its capabilities (drives)
	type PluginInitRes struct {
		Drives []string
	}
	var res PluginInitRes
	if err := p.sess.Call("Plugin.Init", nil, &res); err != nil {
		return fmt.Errorf("Plugin.Init failed: %v", err)
	}

	for _, drive := range res.Drives {
		driveName := drive // closure capture
		api.RegisterDrive(driveName, func() vfs.VFS {
			return NewRPCVFS(p.sess, driveName)
		})
	}

	return nil
}

type rpcHighlighterProvider struct{ p *RPCPlugin }

func (r *rpcHighlighterProvider) Name() string                        { return r.p.path }
func (r *rpcHighlighterProvider) Match(f, c string) bool              { return true }
func (r *rpcHighlighterProvider) Create(f, c string) vtui.Highlighter { return &rpcHighlighter{r.p} }

type rpcHighlighter struct{ p *RPCPlugin }

func (h *rpcHighlighter) Highlight(line string, prev any, base uint64) ([]uint64, any) {
	var res HighlightRes
	err := h.p.sess.Call("VFS.Highlight", HighlightReq{Line: line, Prev: prev, Base: base}, &res)
	if err != nil {
		return nil, nil
	}
	return res.Attrs, res.Next
}
func (p *RPCPlugin) Close() error {
	p.closing = true
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	return nil
}
