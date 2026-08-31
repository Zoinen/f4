//go:build windows

package winshell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/unxed/f4/sdk/f4rpc"
)

const createNoWindow = 0x08000000

// Client owns one restartable broker process. Calls are serialized because a
// Shell namespace extension is apartment-affine and often non-reentrant.
type Client struct {
	executable string

	callMu sync.Mutex
	mu     sync.Mutex
	cmd    *exec.Cmd
	sess   *f4rpc.Session
	stdin  io.WriteCloser
	stdout io.ReadCloser
	gen    uint64
	closed bool
}

func NewClient(executable string) *Client {
	return &Client{executable: executable}
}

var defaultClientState struct {
	sync.Mutex
	client *Client
}

func DefaultClient() (*Client, error) {
	defaultClientState.Lock()
	defer defaultClientState.Unlock()
	if defaultClientState.client != nil {
		return defaultClientState.client, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate f4 executable for Windows Shell broker: %w", err)
	}
	defaultClientState.client = NewClient(executable)
	return defaultClientState.client, nil
}

func ShutdownDefaultClient() {
	defaultClientState.Lock()
	client := defaultClientState.client
	defaultClientState.client = nil
	defaultClientState.Unlock()
	if client != nil {
		_ = client.Close()
	}
}

func (c *Client) startLocked() (*f4rpc.Session, uint64, error) {
	if c.closed {
		return nil, 0, io.ErrClosedPipe
	}
	if c.sess != nil && c.cmd != nil && c.cmd.Process != nil {
		return c.sess, c.gen, nil
	}
	if c.executable == "" {
		return nil, 0, fmt.Errorf("Windows Shell broker executable is empty")
	}
	cmd := exec.Command(c.executable, BrokerArg)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, 0, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, 0, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, 0, fmt.Errorf("start Windows Shell broker: %w", err)
	}

	c.gen++
	generation := c.gen
	session := f4rpc.NewSession(stdout, stdin)
	c.cmd, c.sess, c.stdin, c.stdout = cmd, session, stdin, stdout
	go c.serve(generation, cmd, session)
	return session, generation, nil
}

func (c *Client) serve(generation uint64, cmd *exec.Cmd, session *f4rpc.Session) {
	_ = session.Serve()
	_ = cmd.Wait()
	c.mu.Lock()
	if c.gen == generation && c.cmd == cmd {
		c.cmd, c.sess, c.stdin, c.stdout = nil, nil, nil, nil
	}
	c.mu.Unlock()
}

func (c *Client) terminate(generation uint64) {
	c.mu.Lock()
	if c.gen != generation {
		c.mu.Unlock()
		return
	}
	cmd, stdin, stdout := c.cmd, c.stdin, c.stdout
	c.cmd, c.sess, c.stdin, c.stdout = nil, nil, nil, nil
	c.gen++
	c.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if stdout != nil {
		_ = stdout.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (c *Client) call(ctx context.Context, method string, request, response any) error {
	c.callMu.Lock()
	defer c.callMu.Unlock()
	c.mu.Lock()
	session, generation, err := c.startLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	err = session.CallContext(ctx, method, request, response)
	if ctx.Err() != nil || f4rpc.ErrClosed(err) {
		c.terminate(generation)
	}
	return err
}

func (c *Client) Ping(ctx context.Context) error {
	var ok bool
	if err := c.call(ctx, methodPing, nil, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("Windows Shell broker did not acknowledge ping")
	}
	return nil
}

func (c *Client) Roots(ctx context.Context) ([]Node, error) {
	var nodes []Node
	err := c.call(ctx, methodRoots, nil, &nodes)
	return nodes, err
}

func (c *Client) Describe(ctx context.Context, parsingName string) (Node, error) {
	var node Node
	err := c.call(ctx, methodDescribe, describeRequest{ParsingName: parsingName}, &node)
	return node, err
}

func (c *Client) Enumerate(ctx context.Context, parsingName string) ([]Node, error) {
	var response enumerateResponse
	err := c.call(ctx, methodEnumerate, enumerateRequest{ParsingName: parsingName}, &response)
	return decodeEnumerationResponse(response, err)
}

func (c *Client) NavigationChildren(ctx context.Context, parsingName string) ([]Node, error) {
	var response enumerateResponse
	err := c.call(ctx, methodNavigationChildren, enumerateRequest{ParsingName: parsingName}, &response)
	return decodeEnumerationResponse(response, err)
}

func decodeEnumerationResponse(response enumerateResponse, err error) ([]Node, error) {
	if err != nil {
		return nil, err
	}
	switch response.Status {
	case enumerateStatusOK:
		return response.Nodes, nil
	case enumerateStatusGalleryIndexingRequired:
		return nil, ErrGalleryIndexingRequired
	default:
		return nil, fmt.Errorf("unknown Windows Shell enumeration status %d", response.Status)
	}
}

func (c *Client) CreateDir(ctx context.Context, parentParsingName, name string) error {
	var ok bool
	return c.call(ctx, methodCreateDir, newItemRequest{ParentParsingName: parentParsingName, Name: name}, &ok)
}

func (c *Client) Rename(ctx context.Context, parsingName, newName string) error {
	var ok bool
	return c.call(ctx, methodRename, renameRequest{ParsingName: parsingName, NewName: newName}, &ok)
}

func (c *Client) Delete(ctx context.Context, parsingName string, recycle bool) error {
	var ok bool
	return c.call(ctx, methodDelete, deleteRequest{ParsingName: parsingName, Recycle: recycle}, &ok)
}

func (c *Client) ImportPath(ctx context.Context, sourcePath, parentParsingName, name string, move bool) error {
	var ok bool
	return c.call(ctx, methodImport, importRequest{
		SourcePath: sourcePath, ParentParsingName: parentParsingName, Name: name, Move: move,
	}, &ok)
}

func (c *Client) Transfer(ctx context.Context, sourceParsingName, destinationParsingName, name string, move bool) error {
	var ok bool
	return c.call(ctx, methodTransfer, transferRequest{
		SourceParsingName: sourceParsingName, DestinationParsingName: destinationParsingName,
		Name: name, Move: move,
	}, &ok)
}

func (c *Client) Materialize(ctx context.Context, parsingName string) (MaterializedFile, error) {
	var file MaterializedFile
	err := c.call(ctx, methodMaterialize, describeRequest{ParsingName: parsingName}, &file)
	return file, err
}

func (c *Client) ContextMenu(ctx context.Context, parsingName string) (ContextMenu, error) {
	var menu ContextMenu
	err := c.call(ctx, methodContextMenu, contextMenuRequest{ParsingName: parsingName}, &menu)
	return menu, err
}

func (c *Client) InvokeContextCommand(ctx context.Context, token uint64, commandID uint32) error {
	var ok bool
	return c.call(ctx, methodContextInvoke, contextInvokeRequest{Token: token, CommandID: commandID}, &ok)
}

func (c *Client) DismissContextMenu(ctx context.Context, token uint64) error {
	var ok bool
	return c.call(ctx, methodContextDismiss, contextDismissRequest{Token: token}, &ok)
}

func (c *Client) Close() error {
	c.callMu.Lock()
	defer c.callMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	generation := c.gen
	c.mu.Unlock()
	c.terminate(generation)
	return nil
}
