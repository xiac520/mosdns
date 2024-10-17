package redirect

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
	"go.ubuntu.com/zap"
)

const PluginType = "redirect"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.RecursiveExecutable = (*Redirect)(nil)

type Args struct {
	Rules []string `yaml:"rules"`
	Files []string `yaml:"files"`
}

type Redirect struct {
	m           *domain.MixMatcher[string]
	cache       map[string]string
	cacheExpire time.Duration
	cacheMutex  sync.RWMutex
}

func Init(bp *coremain.BP, args any) (any, error) {
	r, err := NewRedirect(args.(*Args))
	if err != nil {
		return nil, err
	}
	bp.L().Info("redirect rules loaded", zap.Int("length", r.Len()))
	return r, nil
}

func NewRedirect(args *Args) (*Redirect, error) {
	parseFunc := func(s string) (p, v string, err error) {
		f := strings.Fields(s)
		if len(f) != 2 {
			return "", "", fmt.Errorf("redirect rule must have 2 fields, but got %d", len(f))
		}
		return f[0], dns.Fqdn(f[1]), nil
	}

	m := domain.NewMixMatcher[string]()
	m.SetDefaultMatcher(domain.MatcherFull)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errChan = make(chan error, 1)

	// Load rules
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i, rule := range args.Rules {
			if err := domain.Load[string](m, rule, parseFunc); err != nil {
				mu.Lock()
				errChan <- fmt.Errorf("failed to load rule #%d %s, %w", i, rule, err)
				mu.Unlock()
				return
			}
		}
	}()

	// Load files
	for _, file := range args.Files {
		wg.Add(1)
		go func(file string) {
			defer wg.Done()
			b, err := os.ReadFile(file)
			if err != nil {
				mu.Lock()
				errChan <- fmt.Errorf("failed to read file %s, %w", file, err)
				mu.Unlock()
				return
			}
			if err := domain.LoadFromTextReader[string](m, bytes.NewReader(b), parseFunc); err != nil {
				mu.Lock()
				errChan <- fmt.Errorf("failed to load file %s, %w", file, err)
				mu.Unlock()
				return
			}
		}(file)
	}

	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		return nil, err
	}

	return &Redirect{
		m:           m,
		cache:       make(map[string]string),
		cacheExpire: 5 * time.Minute,
	}, nil
}

func (r *Redirect) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	q := qCtx.Q()
	if len(q.Question) != 1 || q.Question[0].Qclass != dns.ClassINET {
		return next.ExecNext(ctx, qCtx)
	}

	orgQName := q.Question[0].Name
	redirectTarget, ok := r.getCache(orgQName)
	if !ok {
		redirectTarget, ok = r.m.Match(orgQName)
		if ok {
			r.setCache(orgQName, redirectTarget)
		}
	}

	if !ok {
		return next.ExecNext(ctx, qCtx)
	}

	q.Question[0].Name = redirectTarget
	defer func() {
		q.Question[0].Name = orgQName
	}()

	err := next.ExecNext(ctx, qCtx)
	if r := qCtx.R(); r != nil {
		// Restore original query name.
		for i := range r.Question {
			if r.Question[i].Name == redirectTarget {
				r.Question[i].Name = orgQName
			}
		}

		// Insert a CNAME record.
		newAns := make([]dns.RR, 0, len(r.Answer)+1)
		newAns = append(newAns, &dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   orgQName,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    1,
			},
			Target: redirectTarget,
		})
		newAns = append(newAns, r.Answer...)
		r.Answer = newAns
	}
	return err
}

func (r *Redirect) Len() int {
	return r.m.Len()
}

func (r *Redirect) getCache(key string) (string, bool) {
	r.cacheMutex.RLock()
	defer r.cacheMutex.RUnlock()

	target, ok := r.cache[key]
	return target, ok
}

func (r *Redirect) setCache(key, value string) {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	r.cache[key] = value
	go func() {
		time.Sleep(r.cacheExpire)
		r.cacheMutex.Lock()
		delete(r.cache, key)
		r.cacheMutex.Unlock()
	}()
}