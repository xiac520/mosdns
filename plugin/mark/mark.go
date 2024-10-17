package mark

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strconv"
	"strings"
	"sync"
)

const PluginType = "mark"

var markPool = sync.Pool{
	New: func() interface{} {
		return &mark{m: make([]uint32, 0, 8)}
	},
}

func init() {
	sequence.MustRegExecQuickSetup(PluginType, func(_ sequence.BQ, args string) (any, error) {
		return newMarker(args)
	})
	sequence.MustRegMatchQuickSetup(PluginType, func(_ sequence.BQ, args string) (sequence.Matcher, error) {
		return newMarker(args)
	})
}

var _ sequence.Executable = (*mark)(nil)
var _ sequence.Matcher = (*mark)(nil)

type mark struct {
	m []uint32
}

func (m *mark) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	for _, u := range m.m {
		if qCtx.HasMark(u) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mark) Exec(_ context.Context, qCtx *query_context.Context) error {
	for _, u := range m.m {
		qCtx.SetMark(u)
	}
	return nil
}

// newMarker format: [uint32_mark]...
// "uint32_mark" is an uint32 defined as Go syntax for integer literals.
// e.g. "111", "0b111", "0o111", "0xfff".
func newMarker(s string) (*mark, error) {
	// 从对象池中获取一个 mark 实例
	m := markPool.Get().(*mark)
	m.m = m.m[:0] // 重置切片

	// 使用 strings.FieldsFunc 分割字符串
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' '
	})
	for _, ms := range fields {
		// 尝试解析不同进制的整数
		n, err := strconv.ParseUint(ms, 0, 32)
		if err != nil {
			markPool.Put(m) // 将 mark 实例放回对象池
			return nil, err
		}
		m.m = append(m.m, uint32(n))
	}
	return m, nil
}

// Release mark instance back to the pool
func (m *mark) Release() {
	m.m = m.m[:0]
	markPool.Put(m)
}