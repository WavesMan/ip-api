package localdb

import (
    "sync/atomic"
)

type lookupable interface { Lookup(string) (Location, bool) }

type DynamicCache struct { v atomic.Value }

// 文档注释：动态缓存包装器
// 背景：通过 atomic.Value 提供无锁读写切换（如从内存缓存切换到文件缓存），保障高并发场景下读路径不阻塞。
// 约束：内部存储需实现 Lookup(string)；Set 时类型断言必须一致，否则 Lookup 会触发 panic。

// 文档注释：查找（读路径）
// 背景：原子读取当前缓存实现，未设置时返回未命中；适合热重载场景避免中断服务。
func (d *DynamicCache) Lookup(ip string) (Location, bool) {
    x := d.v.Load()
    if x == nil { return Location{}, false }
    c := x.(lookupable)
    return c.Lookup(ip)
}

// 文档注释：设置当前缓存实现（写路径）
// 背景：用于切换不同实现（内存/文件/远端）；在写入后立即对后续查找生效。
// WARNING: c 为 nil 会导致后续查找均未命中，应在上层保证非空与可用性。
func (d *DynamicCache) Set(c lookupable) { d.v.Store(c) }
