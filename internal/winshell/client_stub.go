//go:build !windows

package winshell

import (
	"context"
	"io"
)

type Client struct{}

func NewClient(string) *Client                                      { return &Client{} }
func DefaultClient() (*Client, error)                               { return nil, ErrUnavailable }
func ShutdownDefaultClient()                                        {}
func RunBroker(io.Reader, io.Writer) error                          { return ErrUnavailable }
func (c *Client) Ping(context.Context) error                        { return ErrUnavailable }
func (c *Client) Roots(context.Context) ([]Node, error)             { return nil, ErrUnavailable }
func (c *Client) Describe(context.Context, string) (Node, error)    { return Node{}, ErrUnavailable }
func (c *Client) Enumerate(context.Context, string) ([]Node, error) { return nil, ErrUnavailable }
func (c *Client) NavigationChildren(context.Context, string) ([]Node, error) {
	return nil, ErrUnavailable
}
func (c *Client) CreateDir(context.Context, string, string) error { return ErrUnavailable }
func (c *Client) Rename(context.Context, string, string) error    { return ErrUnavailable }
func (c *Client) Delete(context.Context, string, bool) error      { return ErrUnavailable }
func (c *Client) ImportPath(context.Context, string, string, string, bool) error {
	return ErrUnavailable
}
func (c *Client) Transfer(context.Context, string, string, string, bool) error { return ErrUnavailable }
func (c *Client) Materialize(context.Context, string) (MaterializedFile, error) {
	return MaterializedFile{}, ErrUnavailable
}
func (c *Client) ContextMenu(context.Context, string) (ContextMenu, error) {
	return ContextMenu{}, ErrUnavailable
}
func (c *Client) InvokeContextCommand(context.Context, uint64, uint32) error {
	return ErrUnavailable
}
func (c *Client) DismissContextMenu(context.Context, uint64) error { return ErrUnavailable }
func (c *Client) Close() error                                     { return nil }
