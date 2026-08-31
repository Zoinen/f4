//go:build windows

package winshell

import (
	"errors"
	"fmt"
	"io"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	methodPing               = "Shell.Ping"
	methodRoots              = "Shell.Roots"
	methodDescribe           = "Shell.Describe"
	methodEnumerate          = "Shell.Enumerate"
	methodNavigationChildren = "Shell.NavigationChildren"
	methodCreateDir          = "Shell.CreateDir"
	methodRename             = "Shell.Rename"
	methodDelete             = "Shell.Delete"
	methodImport             = "Shell.Import"
	methodTransfer           = "Shell.Transfer"
	methodMaterialize        = "Shell.Materialize"
	methodContextMenu        = "Shell.ContextMenu"
	methodContextInvoke      = "Shell.ContextInvoke"
	methodContextDismiss     = "Shell.ContextDismiss"
)

func decodeRequest[T any](raw msgpack.RawMessage) (T, error) {
	var request T
	if len(raw) == 0 {
		return request, nil
	}
	if err := msgpack.Unmarshal(raw, &request); err != nil {
		return request, fmt.Errorf("decode Windows Shell request: %w", err)
	}
	return request, nil
}

func encodeEnumerationResponse(nodes []Node, err error) (any, error) {
	if errors.Is(err, ErrGalleryIndexingRequired) {
		return enumerateResponse{Status: enumerateStatusGalleryIndexingRequired}, nil
	}
	return enumerateResponse{Nodes: nodes, Status: enumerateStatusOK}, err
}

// RunBroker serves the private Shell RPC protocol over the supplied streams.
// Every COM operation is dispatched to one dedicated STA thread.
func RunBroker(reader io.Reader, writer io.Writer) error {
	apartment, err := newShellApartment()
	if err != nil {
		return err
	}
	defer apartment.close()
	contextMenus := newNativeContextState()
	defer func() {
		_, _ = apartment.call(func() (any, error) {
			contextMenus.close()
			return nil, nil
		})
	}()

	session := f4rpc.NewSession(reader, writer)
	session.Register(methodPing, func(msgpack.RawMessage) (any, error) { return true, nil })
	session.Register(methodRoots, func(msgpack.RawMessage) (any, error) {
		return apartment.call(func() (any, error) { return buildRootModel() })
	})
	session.Register(methodDescribe, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[describeRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) { return describeParsingName(request.ParsingName) })
	})
	session.Register(methodEnumerate, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[enumerateRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			nodes, enumerateErr := enumerateParsingName(request.ParsingName)
			return encodeEnumerationResponse(nodes, enumerateErr)
		})
	})
	session.Register(methodNavigationChildren, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[enumerateRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			nodes, enumerateErr := enumerateNavigationChildren(request.ParsingName)
			return encodeEnumerationResponse(nodes, enumerateErr)
		})
	})
	session.Register(methodCreateDir, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[newItemRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			return true, createFolder(request.ParentParsingName, request.Name)
		})
	})
	session.Register(methodRename, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[renameRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			return true, renameItem(request.ParsingName, request.NewName)
		})
	})
	session.Register(methodDelete, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[deleteRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			return true, deleteItem(request.ParsingName, request.Recycle)
		})
	})
	session.Register(methodImport, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[importRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			return true, importPath(request.SourcePath, request.ParentParsingName, request.Name, request.Move)
		})
	})
	session.Register(methodTransfer, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[transferRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			return true, transferShellItem(request.SourceParsingName, request.DestinationParsingName, request.Name, request.Move)
		})
	})
	session.Register(methodMaterialize, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[describeRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) { return materializeItem(request.ParsingName) })
	})
	session.Register(methodContextMenu, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[contextMenuRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) { return contextMenus.open(request.ParsingName) })
	})
	session.Register(methodContextInvoke, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[contextInvokeRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			return true, contextMenus.invoke(request.Token, request.CommandID)
		})
	})
	session.Register(methodContextDismiss, func(raw msgpack.RawMessage) (any, error) {
		request, err := decodeRequest[contextDismissRequest](raw)
		if err != nil {
			return nil, err
		}
		return apartment.call(func() (any, error) {
			contextMenus.dismiss(request.Token)
			return true, nil
		})
	})
	return session.Serve()
}
