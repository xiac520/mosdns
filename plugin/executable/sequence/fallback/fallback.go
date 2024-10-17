package fallback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"sync"
)

const PluginType = "fallback"

const (
	defaultParallelTimeout   = time.Second * 5
	defaultFallbackThreshold = time.Millisecond * 500
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

type fallback struct {
	logger               *zap.Logger
	primary              sequence.Executable
	secondary            sequence.Executable
	fastFallbackDuration time.Duration
	alwaysStandby        bool
	qCtxPool             *sync.Pool
}

type Args struct {
	// Primary exec sequence.
	Primary string `yaml:"primary"`
	// Secondary exec sequence.
	Secondary string `yaml:"secondary"`

	// Threshold in milliseconds. Default is 500.
	Threshold int `yaml:"threshold"`

	// AlwaysStandby: secondary should always stand by in fallback.
	AlwaysStandby bool `yaml:"always_standby"`
}

func Init(bp *coremain.BP, args any) (any, error) {
	return newFallbackPlugin(bp, args.(*Args))
}

func newFallbackPlugin(bp *coremain.BP, args *Args) (*fallback, error) {
	if len(args.Primary) == 0 || len(args.Secondary) == 0 {
		return nil, errors.New("args missing primary or secondary")
	}

	pe := sequence.ToExecutable(bp.M().GetPlugin(args.Primary))
	if pe == nil {
		return nil, fmt.Errorf("can not find primary executable %s", args.Primary)
	}
	se := sequence.ToExecutable(bp.M().GetPlugin(args.Secondary))
	if se == nil {
		return nil, fmt.Errorf("can not find secondary executable %s", args.Secondary)
	}
	threshold := time.Duration(args.Threshold) * time.Millisecond
	if threshold <= 0 {
		threshold = defaultFallbackThreshold
	}

	s := &fallback{
		logger:               bp.L(),
		primary:              pe,
		secondary:            se,
		fastFallbackDuration: threshold,
		alwaysStandby:        args.AlwaysStandby,
		qCtxPool: &sync.Pool{
			New: func() any {
				return query_context.NewContext()
			},
		},
	}
	return s, nil
}

var (
	ErrFailed = errors.New("no valid response from both primary and secondary")
)

var _ sequence.Executable = (*fallback)(nil)

func (f *fallback) Exec(ctx context.Context, qCtx *query_context.Context) error {
	return f.doFallback(ctx, qCtx)
}

func (f *fallback) doFallback(ctx context.Context, qCtx *query_context.Context) error {
	respChan := make(chan *dns.Msg, 2) // resp could be nil.
	primFailed := make(chan struct{})
	primDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// primary goroutine.
	qCtxP := f.qCtxPool.Get().(*query_context.Context)
	*qCtxP = *qCtx
	go func() {
		defer func() {
			f.qCtxPool.Put(qCtxP)
			wg.Done()
		}()
		ctx, cancel := makeDdlCtx(ctx, defaultParallelTimeout)
		defer cancel()
		err := f.primary.Exec(ctx, qCtxP)
		if err != nil {
			f.logger.Warn("primary error", qCtx.InfoField(), zap.Error(err))
		}

		r := qCtxP.R()
		if err != nil || r == nil {
			close(primFailed)
			respChan <- nil
		} else {
			close(primDone)
			respChan <- r
		}
	}()

	// Secondary goroutine.
	qCtxS := f.qCtxPool.Get().(*query_context.Context)
	*qCtxS = *qCtx
	go func() {
		defer func() {
			f.qCtxPool.Put(qCtxS)
			wg.Done()
		}()
		if !f.alwaysStandby { // not always standby, wait here.
			select {
			case <-primDone: // primary is done, no need to exec this.
				return
			case <-primFailed: // primary failed
			case <-time.After(f.fastFallbackDuration): // timed out
			}
		}

		ctx, cancel := makeDdlCtx(ctx, defaultParallelTimeout)
		defer cancel()
		err := f.secondary.Exec(ctx, qCtxS)
		if err != nil {
			f.logger.Warn("secondary error", qCtx.InfoField(), zap.Error(err))
			respChan <- nil
			return
		}

		r := qCtxS.R()
		// always standby is enabled. Wait until secondary resp is needed.
		if f.alwaysStandby && r != nil {
			select {
			case <-ctx.Done():
			case <-primDone:
			case <-primFailed: // only send secondary result when primary is failed.
			case <-time.After(f.fastFallbackDuration): // or timed out.
			}
		}
		respChan <- r
	}()

	go func() {
		wg.Wait()
		close(respChan)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case r := <-respChan:
			if r == nil { // One of goroutines finished but failed.
				continue
			}
			qCtx.SetResponse(r)
			return nil
		}
	}

	// All goroutines finished but failed.
	return ErrFailed
}

func makeDdlCtx(ctx context.Context, timeout time.Duration) (context.Context, func()) {
	ddl, ok := ctx.Deadline()
	if !ok {
		ddl = time.Now().Add(timeout)
	}
	return context.WithDeadline(ctx, ddl)
}