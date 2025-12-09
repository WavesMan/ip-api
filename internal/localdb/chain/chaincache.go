package chain

import "ip-api/internal/localdb"

type ChainCache struct {
    list []interface{ Lookup(string) (localdb.Location, bool) }
}

func NewChainCache(list ...interface{ Lookup(string) (localdb.Location, bool) }) *ChainCache {
    return &ChainCache{ list: list }
}

func (c *ChainCache) Lookup(ip string) (localdb.Location, bool) {
    for _, s := range c.list {
        if s == nil { continue }
        if l, ok := s.Lookup(ip); ok { return l, true }
    }
    return localdb.Location{}, false
}
