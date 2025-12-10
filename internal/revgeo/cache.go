package revgeo

import (
    "container/list"
    "sync"
    "time"
)

// 文档注释：本地 LRU 缓存（geohash/量化坐标为键）
// 背景：热点坐标在短周期内重复查询，使用进程内缓存降低索引与判定开销；TTL 可调。
// 约束：仅用于 /reverse_geo；键由调用方构造，建议采用 geohash(prec≈6) 或四舍五入到 0.001°。
type LRU struct {
    mu   sync.Mutex
    cap  int
    ttl  time.Duration
    lst  *list.List
    dict map[string]*list.Element
}

type kv struct { k string; v AdminUnit; exp time.Time }

func NewLRU(capacity int, ttlSec int) *LRU {
    return &LRU{cap: capacity, ttl: time.Duration(ttlSec)*time.Second, lst: list.New(), dict: make(map[string]*list.Element)}
}

func (c *LRU) Get(k string) (AdminUnit, bool) {
    c.mu.Lock(); defer c.mu.Unlock()
    if e, ok := c.dict[k]; ok {
        it := e.Value.(kv)
        if time.Now().Before(it.exp) {
            c.lst.MoveToFront(e)
            return it.v, true
        }
        c.lst.Remove(e)
        delete(c.dict, k)
    }
    return AdminUnit{}, false
}

func (c *LRU) Set(k string, v AdminUnit) {
    c.mu.Lock(); defer c.mu.Unlock()
    if e, ok := c.dict[k]; ok {
        e.Value = kv{k: k, v: v, exp: time.Now().Add(c.ttl)}
        c.lst.MoveToFront(e)
        return
    }
    e := c.lst.PushFront(kv{k: k, v: v, exp: time.Now().Add(c.ttl)})
    c.dict[k] = e
    for c.lst.Len() > c.cap {
        back := c.lst.Back()
        if back != nil {
            it := back.Value.(kv)
            delete(c.dict, it.k)
            c.lst.Remove(back)
        }
    }
}

