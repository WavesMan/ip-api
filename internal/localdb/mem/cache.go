package mem

import (
    "database/sql"
    "ip-api/internal/localdb"
    "ip-api/internal/logger"
    "net"
    "sort"
)

type Range struct {
    Start uint32
    End   uint32
    LocID int
}

type Cache struct {
    idx  [256][]Range
    locs []localdb.Location
}

func BuildFromDB(db *sql.DB) (*Cache, error) {
    logger.L().Debug("memcache_build_begin")
    c := &Cache{}
    rows, err := db.Query("SELECT id, country, region, province, city, isp FROM _ip_locations")
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    for rows.Next() {
        var id int
        var l localdb.Location
        if err := rows.Scan(&id, &l.Country, &l.Region, &l.Province, &l.City, &l.ISP); err != nil {
            return nil, err
        }
        c.locs = append(c.locs, l)
    }
    rrows, err := db.Query("SELECT start_int, end_int, first_octet, location_id FROM _ip_ipv4_ranges ORDER BY first_octet, start_int")
    if err != nil {
        return nil, err
    }
    defer rrows.Close()
    for rrows.Next() {
        var s, e int64
        var a, lid int
        if err := rrows.Scan(&s, &e, &a, &lid); err != nil {
            return nil, err
        }
        c.idx[a] = append(c.idx[a], Range{Start: uint32(s), End: uint32(e), LocID: lid})
    }
    for i := 0; i < 256; i++ {
        sort.Slice(c.idx[i], func(p, q int) bool { return c.idx[i][p].Start < c.idx[i][q].Start })
    }
    logger.L().Debug("memcache_build_done", "locations", len(c.locs))
    return c, nil
}

func (c *Cache) Lookup(ip string) (localdb.Location, bool) {
    var zero localdb.Location
    p := net.ParseIP(ip)
    if p == nil || p.To4() == nil {
        return zero, false
    }
    v := p.To4()
    val := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
    a := int(v[0])
    arr := c.idx[a]
    if len(arr) == 0 {
        return zero, false
    }
    i := sort.Search(len(arr), func(i int) bool { return arr[i].Start > val })
    if i == 0 {
        return zero, false
    }
    r := arr[i-1]
    if val >= r.Start && val <= r.End {
        lid := r.LocID
        if lid >= 1 && lid <= len(c.locs) {
            return c.locs[lid-1], true
        }
    }
    return zero, false
}

func (c *Cache) Locations() []localdb.Location { return c.locs }
