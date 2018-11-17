package luigi // import "go.cryptoscope.co/luigi"

import (
	"context"
	"sync"
)

type Broadcast interface {
	Register(Sink) func()
}

func NewBroadcast() (Sink, Broadcast) {
	bcst := broadcast{sinks: make(map[*Sink]struct{})}

	return (*broadcastSink)(&bcst), &bcst
}

type broadcast struct {
	sync.Mutex
	sinks map[*Sink]struct{}
}

func (bcst *broadcast) Register(sink Sink) func() {
	bcst.Lock()
	defer bcst.Unlock()
	bcst.sinks[&sink] = struct{}{}

	return func() {
		bcst.Lock()
		defer bcst.Unlock()
		delete(bcst.sinks, &sink)
		sink.Close()
	}
}

type broadcastSink broadcast

func (bcst *broadcastSink) Pour(ctx context.Context, v interface{}) error {
	errCh := make(chan error)
	errOut := make(chan error, 1)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bcst.Lock()
	defer bcst.Unlock()

	go func() {
		defer close(errOut)

		if err, ok := <-errCh; ok {
			cancel()
			errOut <- err
		}

		for range errCh {
			// drain
		}
	}()

	func() {
		sinks := make([]Sink, 0, len(bcst.sinks))

		for sink := range bcst.sinks {
			sinks = append(sinks, *sink)
		}

		// release lock while broadcasting
		// they might want to take it, e.g. to call cancel()
		bcst.Unlock()
		defer bcst.Lock()

		var wg sync.WaitGroup

		wg.Add(len(sinks))
		for _, sink_ := range sinks {
			go func(sink Sink) {
				defer wg.Done()

				err := sink.Pour(ctx, v)
				if err != nil {
					errCh <- err
					return
				}
			}(sink_)
		}

		wg.Wait()
		close(errCh)
	}()

	return <-errOut
}

func (bcst *broadcastSink) Close() error { return nil }
