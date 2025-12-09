package ip2region

import (
    "ip-api/internal/localdb"
    "strings"
    
    "github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

type IP2RegionCache struct {
    v4 *xdb.Searcher
    v6 *xdb.Searcher
}

func NewIP2RegionCache(v4Path, v6Path string) (*IP2RegionCache, error) {
    var v4s *xdb.Searcher
    var v6s *xdb.Searcher
    var err error
    if v4Path != "" {
        v4s, err = xdb.NewWithFileOnly(xdb.IPv4, v4Path)
        if err != nil { return nil, err }
    }
    if v6Path != "" {
        v6s, err = xdb.NewWithFileOnly(xdb.IPv6, v6Path)
        if err != nil { return nil, err }
    }
    return &IP2RegionCache{ v4: v4s, v6: v6s }, nil
}

func (c *IP2RegionCache) Lookup(ip string) (localdb.Location, bool) {
    var zero localdb.Location
    if ip == "" { return zero, false }
    if c.v4 != nil {
        if region, err := c.v4.SearchByStr(ip); err == nil && region != "" {
            return parseRegion(region), true
        }
    }
    if c.v6 != nil {
        if region, err := c.v6.SearchByStr(ip); err == nil && region != "" {
            return parseRegion(region), true
        }
    }
    return zero, false
}

func parseRegion(s string) localdb.Location {
    parts := strings.Split(s, "|")
    var l localdb.Location
    if len(parts) > 0 { l.Country = safe(parts[0]) }
    if len(parts) > 1 { l.Region = safe(parts[1]) }
    if len(parts) > 2 { l.Province = safe(parts[2]) }
    if len(parts) > 3 { l.City = safe(parts[3]) }
    if len(parts) > 4 { l.ISP = safe(parts[4]) }
    return l
}

func safe(s string) string {
    if s == "0" || s == "" || strings.EqualFold(s, "unknown") { return "" }
    return s
}
