package muxrpc // import "go.cryptoscope.co/muxrpc"

import (
	"context"
	"sync"

	"go.cryptoscope.co/luigi"
)

// withCloseCtx returns a cancellable context where ctx.Err() is luigi.EOS instead of "context cancelled"
func withCloseCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	ch := make(chan struct{})
	next := &closeCtx{
		ch:      ch,
		Context: ctx,
		once:    &sync.Once{},
	}

	go func() {
		next.l.Lock()
		defer next.l.Unlock()

		select {
		case <-ctx.Done():
		case <-ch:
			next.closed = true
		}
	}()

	return next, func() { next.once.Do(func() { close(ch) }) }
}

// closeCtx is the context that cancels functions and returns a luigi.EOS error
type closeCtx struct {
	ch <-chan struct{}
	context.Context

	l      sync.Mutex
	once   *sync.Once
	closed bool
}

// Done returns a channel that is closed once the context is cancelled.
func (ctx *closeCtx) Done() <-chan struct{} {
	ch := make(chan struct{})

	go func() {
		select {
		case <-ctx.ch:
		case <-ctx.Context.Done():
		}

		close(ch)
	}()

	return ch
}

// Err returns the error that made the context cancel.
// returns luigi.EOS if cancelled using our cancel function or the error
// returned by the context below if that was canceled.
func (ctx *closeCtx) Err() error {
	select {
	case <-ctx.ch:
		return luigi.EOS{}
	case <-ctx.Context.Done():
		return ctx.Context.Err()
	default:
		return nil
	}
}
