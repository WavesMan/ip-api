package localdb

import (
    "database/sql"
    "ip-api/internal/logger"
    "net"
    "sort"
)

type Location struct{ Country, Region, Province, City, ISP string }
type Range struct {
    Start uint32
    End   uint32
    LocID int
}

type Cache struct {
    idx  [256][]Range
    locs []Location
}

// 文档注释：内存压缩范围缓存
// 背景：按 IPv4 首字节分片构建轻量索引，极大降低查找范围与内存占用；范围数组按起始值有序以支持二分查找。
// 约束：LocID 从 1 起连续映射到地点切片，外部写入需保证一致性；仅支持 IPv4 查找。

// 文档注释：从数据库构建内存缓存
// 背景：一次性拉取地点与范围，并按首字节分桶与排序；用于高 QPS 本地查询路径，规避频繁数据库访问。
// 返回：构建好的 Cache；异常包含数据库查询失败与扫描错误。
func BuildFromDB(db *sql.DB) (*Cache, error) {
    logger.L().Debug("memcache_build_begin")
    c := &Cache{}
    rows, err := db.Query("SELECT id, country, region, province, city, isp FROM _ip_locations")
    if err != nil {
        return nil, err
    }
	defer rows.Close()
	var maxID int
	for rows.Next() {
		var id int
		var l Location
		if err := rows.Scan(&id, &l.Country, &l.Region, &l.Province, &l.City, &l.ISP); err != nil {
			return nil, err
		}
		if id > maxID {
			maxID = id
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

// 文档注释：IPv4 查找
// 背景：将字符串 IP 解析为整数，依据首字节选择分片并二分定位范围；命中后通过 LocID 返回地点信息。
// 返回：Location 与命中标记；无对应范围或解析失败返回 false。
func (c *Cache) Lookup(ip string) (Location, bool) {
	var zero Location
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

// 文档注释：返回所有地点记录（按 id 顺序）
// 背景：用于外部需要地点全集的场景（如构建文件缓存），避免重复查询数据库。
func (c *Cache) Locations() []Location { return c.locs }
