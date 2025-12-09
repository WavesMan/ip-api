package chain

import "ip-api/internal/localdb"

type MultiCache struct {
    a interface{ Lookup(string) (localdb.Location, bool) }
    b interface{ Lookup(string) (localdb.Location, bool) }
}

func NewMultiCache(a, b interface{ Lookup(string) (localdb.Location, bool) }) *MultiCache {
    return &MultiCache{a: a, b: b}
}

func (m *MultiCache) Lookup(ip string) (localdb.Location, bool) {
    if m.a != nil {
        if l, ok := m.a.Lookup(ip); ok {
            return l, true
        }
    }
    if m.b != nil {
        return m.b.Lookup(ip)
    }
    return localdb.Location{}, false
}
