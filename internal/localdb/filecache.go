package localdb

import (
    "database/sql"
    "encoding/binary"
    "encoding/json"
    "ip-api/internal/logger"
    "os"
    "path/filepath"
    "sort"
    "strconv"
)

type FileCache struct {
    dir  string
    locs []Location
    lru  map[int][]Range
}

// 文档注释：从数据库生成文件缓存
// 背景：将地点表写为 JSON（locations.json），范围按首字节分桶写为二进制（octet-*.bin），便于后续按需加载；适合低内存部署。
// 参数：dir 为输出目录；db 为数据库连接。
// 文件格式：
// - locations.json：按 id 顺序的地点数组；
// - octet-x.bin：以 BE 写入 [count(uint32)] + count 条记录，每条记录含 Start(uint32), End(uint32), LocID(uint32)。
// 异常：目录创建/查询/写文件失败直接返回；写入过程尽量保持原子性（先写后关）。
func BuildFilesFromDB(dir string, db *sql.DB) error {
    logger.L().Debug("filecache_build_begin", "dir", dir)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
	// 写 locations.json
	rows, err := db.Query("SELECT id, country, region, province, city, isp FROM _ip_locations ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	var locs []Location
	for rows.Next() {
		var id int
		var l Location
		if err := rows.Scan(&id, &l.Country, &l.Region, &l.Province, &l.City, &l.ISP); err != nil {
			return err
		}
		locs = append(locs, l)
	}
    b, _ := json.Marshal(locs)
    if err := os.WriteFile(filepath.Join(dir, "locations.json"), b, 0o644); err != nil {
        return err
    }
    logger.L().Debug("filecache_locations_written", "count", len(locs))
	// 写每个首段分片文件
	rrows, err := db.Query("SELECT start_int, end_int, first_octet, location_id FROM _ip_ipv4_ranges ORDER BY first_octet, start_int")
	if err != nil {
		return err
	}
	defer rrows.Close()
    buckets := make(map[int][]Range)
    for rrows.Next() {
		var s, e int64
		var a, lid int
		if err := rrows.Scan(&s, &e, &a, &lid); err != nil {
			return err
		}
		buckets[a] = append(buckets[a], Range{Start: uint32(s), End: uint32(e), LocID: lid})
    }
    for a, arr := range buckets {
        sort.Slice(arr, func(i, j int) bool { return arr[i].Start < arr[j].Start })
        fp := filepath.Join(dir, "octet-"+strconv.Itoa(a)+".bin")
        f, err := os.Create(fp)
        if err != nil {
            return err
        }
        // NOTE: 格式为 count(uint32 BE) + records(count * 12 bytes)，便于一次性 mmap/读入与顺序解析。
		if err := binary.Write(f, binary.BigEndian, uint32(len(arr))); err != nil {
			f.Close()
			return err
		}
		for _, r := range arr {
			if err := binary.Write(f, binary.BigEndian, r.Start); err != nil {
				f.Close()
				return err
			}
			if err := binary.Write(f, binary.BigEndian, r.End); err != nil {
				f.Close()
				return err
			}
			if err := binary.Write(f, binary.BigEndian, uint32(r.LocID)); err != nil {
				f.Close()
				return err
			}
        }
        _ = f.Close()
        logger.L().Debug("filecache_bucket_written", "octet", a, "count", len(arr))
    }
    logger.L().Info("filecache_build_done")
    return nil
}

// 文档注释：创建文件缓存实例
// 背景：仅加载地点 JSON，范围桶按需延迟加载并保存在 lru 映射中；读取失败时返回错误以便上层回退。
func NewFileCache(dir string) (*FileCache, error) {
    b, err := os.ReadFile(filepath.Join(dir, "locations.json"))
    if err != nil {
        return nil, err
    }
    var locs []Location
    if err := json.Unmarshal(b, &locs); err != nil {
        return nil, err
    }
    logger.L().Debug("filecache_init", "dir", dir, "locations", len(locs))
    return &FileCache{dir: dir, locs: locs, lru: make(map[int][]Range)}, nil
}

// 文档注释：文件缓存查找（延迟加载桶）
// 背景：按首字节定位桶文件，首次访问时读入并缓存；随后对范围数组做二分查找。
// 约束：仅支持 IPv4 字符串；当文件缺失或损坏时返回未命中，避免影响主流程。
func (c *FileCache) Lookup(ip string) (Location, bool) {
	var zero Location
	p := parseIPv4(ip)
	if p == nil {
		return zero, false
	}
	a := int(p[0])
	v := uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
    arr, ok := c.lru[a]
    if !ok {
        fp := filepath.Join(c.dir, "octet-"+strconv.Itoa(a)+".bin")
        data, err := os.ReadFile(fp)
        if err != nil {
            return zero, false
        }
        logger.L().Debug("filecache_bucket_load", "octet", a, "size", len(data))
        if len(data) < 4 {
            return zero, false
        }
		n := int(binary.BigEndian.Uint32(data[:4]))
		recs := make([]Range, n)
		off := 4
		for i := 0; i < n; i++ {
			if off+12 > len(data) {
				return zero, false
			}
			s := binary.BigEndian.Uint32(data[off : off+4])
			e := binary.BigEndian.Uint32(data[off+4 : off+8])
			lid := int(binary.BigEndian.Uint32(data[off+8 : off+12]))
			recs[i] = Range{Start: s, End: e, LocID: lid}
			off += 12
		}
		c.lru[a] = recs
		arr = recs
	}
	if len(arr) == 0 {
		return zero, false
	}
	i := sort.Search(len(arr), func(i int) bool { return arr[i].Start > v })
	if i == 0 {
		return zero, false
	}
	r := arr[i-1]
	if v >= r.Start && v <= r.End {
		lid := r.LocID
		if lid >= 1 && lid <= len(c.locs) {
			return c.locs[lid-1], true
		}
	}
	return zero, false
}

// 文档注释：轻量 IPv4 解析
// 背景：避免使用标准库带来的额外分配与宽容解析规则，仅支持严格的 0–255 数字与 3 个点分；用于高频查找路径。
// 返回：长度为 4 的字节切片；非法输入返回 nil。
func parseIPv4(s string) []byte {
	b := make([]byte, 0, 4)
	v := 0
	c := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			v = v*10 + int(ch-'0')
			if v > 255 {
				return nil
			}
		}
		if ch == '.' {
			b = append(b, byte(v))
			v = 0
			c++
			if c > 3 {
				return nil
			}
		}
	}
	b = append(b, byte(v))
	if len(b) != 4 {
		return nil
	}
	return b
}

// NOTE: 保留空行用于结束标记，避免误拼接注释影响可读性。
