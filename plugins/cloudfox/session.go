package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type pooledSession struct {
	pool    *sessionPool
	key     string
	ready   chan struct{}
	backend Backend
	err     error
	refs    int
}

type sessionPool struct {
	mu       sync.Mutex
	sessions map[string]*pooledSession
	closed   bool
	ctx      context.Context
	cancel   context.CancelFunc
}

// backendSnapshot returns the backend while holding the same mutex that
// release and close use when invalidating a pooled session.  Callers must not
// read pooledSession.backend directly: doing so races plugin shutdown and can
// turn a successful nil check into a nil interface return.
func (s *pooledSession) backendSnapshot() Backend {
	if s == nil || s.pool == nil {
		return nil
	}
	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	if s.err != nil {
		return nil
	}
	return s.backend
}

func newSessionPool() *sessionPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &sessionPool{sessions: make(map[string]*pooledSession), ctx: ctx, cancel: cancel}
}

func (p *sessionPool) lifetime() context.Context { return p.ctx }

func sessionVersionKey(c Connection) string {
	return c.ID + "\x00" + c.UpdatedAt.UTC().Format("20060102T150405.000000000Z") + "\x00" + c.SecretRef
}

// acquire coalesces concurrent opens for an identical committed profile.
func (p *sessionPool) acquire(ctx context.Context, c Connection, open func(context.Context) (Backend, error)) (*pooledSession, error) {
	key := sessionVersionKey(c)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("cloudfox: session pool is closed")
	}
	if existing := p.sessions[key]; existing != nil {
		existing.refs++
		ready := existing.ready
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.release(existing)
			return nil, ctx.Err()
		case <-p.ctx.Done():
			p.release(existing)
			return nil, errors.New("cloudfox: session pool is closed")
		case <-ready:
			if existing.err != nil {
				p.release(existing)
				return nil, existing.err
			}
			return existing, nil
		}
	}
	session := &pooledSession{pool: p, key: key, ready: make(chan struct{}), refs: 1}
	p.sessions[key] = session
	p.mu.Unlock()

	openCtx, finishOpen := providerOperationContext(ctx, p.ctx)
	backend, err := open(openCtx)
	finishOpen()
	if err != nil && backend != nil {
		_ = backend.Close()
		backend = nil
	}
	var closeBackend Backend
	p.mu.Lock()
	if p.closed && err == nil {
		err = errors.New("cloudfox: session pool was closed while opening")
		closeBackend = backend
		backend = nil
	}
	session.backend, session.err = backend, err
	close(session.ready)
	if err != nil {
		delete(p.sessions, key)
	}
	p.mu.Unlock()
	if closeBackend != nil {
		_ = closeBackend.Close()
	}
	if err != nil {
		p.release(session)
		return nil, err
	}
	return session, nil
}

func (p *sessionPool) retain(session *pooledSession) bool {
	if session == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || session.err != nil || session.backend == nil {
		return false
	}
	session.refs++
	return true
}

func (p *sessionPool) release(session *pooledSession) {
	if session == nil {
		return
	}
	p.mu.Lock()
	if session.refs > 0 {
		session.refs--
	}
	if session.refs != 0 {
		p.mu.Unlock()
		return
	}
	if p.sessions[session.key] == session {
		delete(p.sessions, session.key)
	}
	backend := session.backend
	session.backend = nil
	p.mu.Unlock()
	if backend != nil {
		_ = backend.Close()
	}
}

func (p *sessionPool) close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	cancel := p.cancel
	var backends []Backend
	for _, session := range p.sessions {
		if session.backend != nil {
			backends = append(backends, session.backend)
			session.backend = nil
		}
	}
	p.sessions = make(map[string]*pooledSession)
	p.mu.Unlock()
	cancel()
	var result error
	for _, backend := range backends {
		if err := backend.Close(); err != nil {
			result = errors.Join(result, err)
		}
	}
	if result != nil {
		return fmt.Errorf("cloudfox: close sessions: %w", result)
	}
	return nil
}
