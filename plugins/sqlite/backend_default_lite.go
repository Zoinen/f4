//go:build lite

package sqlite

import "context"

// openDefaultBackend is the lite build's backend: the host's sqlite3
// command-line tool (backend_cli.go), so the lite build links no SQLite
// engine. backend_driver.go has the regular build's.
func openDefaultBackend(ctx context.Context, path string) (sessionBackend, error) {
	backend, err := openCLIBackend(ctx, path)
	if err != nil {
		return nil, err
	}
	return backend, nil
}
