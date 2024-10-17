package fastforward

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const PluginType = "forward"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
	sequence.MustRegExecQuickSetup(PluginType, quickSetup)
}

const (
	maxConcurrentQueries = 10  // 增加并发数
	queryTimeout         = time.Second * 2 // 减少超时时间
)

type Args struct {
	Upstreams  []UpstreamConfig `yaml:"upstreams"`
	Concurrent int              `yaml:"concurrent"`

	// Global options.
	Socks5       string `yaml:"socks5"`
	SoMark       int    `yaml:"so_mark"`
	BindToDevice string `yaml:"bind_to_device"`
	Bootstrap    string `yaml:"bootstrap"`
	BootstrapVer int    `yaml:"bootstrap_version"`
}

type UpstreamConfig struct {
	Tag         string `yaml:"tag"`
	Addr        string `yaml:"addr"` // Required.
	DialAddr    string `yaml:"dial_addr"`
	IdleTimeout int    `yaml:"idle_timeout"`

	// Deprecated: This option has no affect.
	// TODO: (v6) Remove this option.
	MaxConns           int  `yaml:"max_conns"`
	EnablePipeline     bool `yaml:"enable_pipeline"`
	EnableHTTP3        bool `yaml:"enable_http3"`
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`

	Socks5       string `yaml:"socks5"`
	SoMark       int    `yaml:"so_mark"`
	BindToDevice string `yaml:"bind_to_device"`
	Bootstrap    string `yaml:"bootstrap"`
	BootstrapVer int    `yaml:"bootstrap_version"`
}

type Forward struct {
	args          *Args
	upstreamCache map[string]*upstream.Upstream
	logger        *zap.Logger
	mu            sync.Mutex
}

func NewForward(args *Args, opts Opts) (*Forward, error) {
	f := &Forward{
		args:          args,
		upstreamCache: make(map[string]*upstream.Upstream),
		logger:        opts.Logger,
	}
	return f, nil
}

func (f *Forward) Exec(ctx context.Context, qCtx *query_context.QueryContext) error {
	// 获取上游配置
	upstreams := f.args.Upstreams

	// 使用 Goroutine 池
	var wg sync.WaitGroup
	errCh := make(chan error, len(upstreams))
	sem := make(chan struct{}, maxConcurrentQueries)

	for _, us := range upstreams {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		wg.Add(1)
		go func(us UpstreamConfig) {
			defer func() {
				<-sem
				wg.Done()
			}()

			// 发起请求
			err := f.queryUpstream(ctx, us, qCtx)
			if err != nil {
				errCh <- err
			}
		}(us)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	// 等待第一个成功的结果或所有请求完成
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *Forward) queryUpstream(ctx context.Context, us UpstreamConfig, qCtx *query_context.QueryContext) error {
	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	// 获取或创建上游连接
	up, err := f.getUpstream(us)
	if err != nil {
		return err
	}

	// 发起请求
	resp, err := up.Exchange(ctx, qCtx)
	if err != nil {
		return err
	}

	// 处理响应
	return f.handleResponse(resp, qCtx)
}

func (f *Forward) getUpstream(us UpstreamConfig) (*upstream.Upstream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if up, ok := f.upstreamCache[us.Addr]; ok {
		return up, nil
	}

	// 创建新的上游连接
	up, err := upstream.NewUpstream(us.Addr, &upstream.Options{
		DialAddr:    us.DialAddr,
		IdleTimeout: time.Duration(us.IdleTimeout) * time.Second,
		Socks5:      us.Socks5,
		SoMark:      us.SoMark,
		BindToDevice: us.BindToDevice,
		Bootstrap:   us.Bootstrap,
		BootstrapVer: us.BootstrapVer,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: us.InsecureSkipVerify,
		},
	})
	if err != nil {
		return nil, err
	}

	f.upstreamCache[us.Addr] = up
	return up, nil
}

func (f *Forward) handleResponse(resp *dns.Msg, qCtx *query_context.QueryContext) error {
	// 处理 DNS 响应的逻辑
	if len(resp.Answer) == 0 {
		return errors.New("no answer")
	}

	qCtx.SetAnswer(resp.Answer)
	return nil
}