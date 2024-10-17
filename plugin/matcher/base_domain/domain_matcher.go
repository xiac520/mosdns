package base_domain

import (
	"context"
	"fmt"
	"sync"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider"
	"github.com/IrineSistiana/mosdns/v5/plugin/data_provider/domain_set"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
)

var _ sequence.Matcher = (*Matcher)(nil)

type Args struct {
	Exps       []string `yaml:"exps"`
	DomainSets []string `yaml:"domain_sets"`
	Files      []string `yaml:"files"`
}

type MatchFunc func(qCtx *query_context.Context, m domain.Matcher[struct{}]) (bool, error)

type Matcher struct {
	match MatchFunc
	mg    []domain.Matcher[struct{}]
}

func (m *Matcher) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var matched bool
	var firstErr error

	for _, matcher := range m.mg {
		wg.Add(1)
		go func(m domain.Matcher[struct{}]) {
			defer wg.Done()
			matched, err := m.match(qCtx, m)
			if err != nil && firstErr == nil {
				mu.Lock()
				firstErr = err
				mu.Unlock()
			}
			if matched {
				mu.Lock()
				matched = true
				mu.Unlock()
			}
		}(matcher)
	}

	wg.Wait()

	if firstErr != nil {
		return false, firstErr
	}

	return matched, nil
}

func NewMatcher(bq sequence.BQ, args *Args, f MatchFunc) (m *Matcher, err error) {
	m = &Matcher{
		match: f,
	}

	// 预先分配 mg 的容量
	totalMatchers := len(args.DomainSets) + (len(args.Exps) + len(args.Files) > 0)
	m.mg = make([]domain.Matcher[struct{}], 0, totalMatchers)

	// Acquire matchers from other plugins.
	for _, tag := range args.DomainSets {
		p := bq.M().GetPlugin(tag)
		dsProvider, _ := p.(data_provider.DomainMatcherProvider)
		if dsProvider == nil {
			return nil, fmt.Errorf("cannot find domain set %s", tag)
		}
		dm := dsProvider.GetDomainMatcher()
		m.mg = append(m.mg, dm)
	}

	// Anonymous set from plugin's args and files.
	if len(args.Exps)+len(args.Files) > 0 {
		anonymousSet := domain.NewDomainMixMatcher()
		if err := domain_set.LoadExpsAndFiles(args.Exps, args.Files, anonymousSet); err != nil {
			return nil, err
		}
		if anonymousSet.Len() > 0 {
			m.mg = append(m.mg, anonymousSet)
		}
	}

	return m, nil
}

// ParseQuickSetupArgs parses expressions and domain set to args.
// Format: "([exp] | [$domain_set_tag] | [&domain_list_file])..."
func ParseQuickSetupArgs(s string) *Args {
	cutPrefix := func(s string, p string) (string, bool) {
		if strings.HasPrefix(s, p) {
			return strings.TrimPrefix(s, p), true
		}
		return s, false
	}

	args := new(Args)
	for _, exp := range strings.Fields(s) {
		if tag, ok := cutPrefix(exp, "$"); ok {
			args.DomainSets = append(args.DomainSets, tag)
		} else if path, ok := cutPrefix(exp, "&"); ok {
			args.Files = append(args.Files, path)
		} else {
			args.Exps = append(args.Exps, exp)
		}
	}
	return args
}