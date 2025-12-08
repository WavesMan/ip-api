// 包 localdb：多级缓存路由器，按优先级查询不同数据源以平衡准确性与覆盖面
package localdb

// 文档注释：多缓存组合
// 背景：优先查询人工覆盖的精确文件库（准确性），未命中则退回树形只读数据（覆盖面）；避免范围 SQL 导致误判。
type MultiCache struct {
	a interface{ Lookup(string) (Location, bool) }
	b interface{ Lookup(string) (Location, bool) }
}

// 文档注释：构建多缓存组合
// 背景：按“精确 -> 前缀树”的固定顺序；上层可根据数据源可用性选择性传入。
func NewMultiCache(a, b interface{ Lookup(string) (Location, bool) }) *MultiCache {
	return &MultiCache{a: a, b: b}
}

// 文档注释：按优先级查询
// 背景：先查 a（精确），未命中再查 b（前缀树）；保持读路径无锁与最小分支以兼顾并发与可读性。
func (m *MultiCache) Lookup(ip string) (Location, bool) {
	if m.a != nil {
		if l, ok := m.a.Lookup(ip); ok {
			return l, true
		}
	}
	if m.b != nil {
		return m.b.Lookup(ip)
	}
	return Location{}, false
}
