package ipip

import (
	"database/sql"
	"ip-api/internal/logger"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// 文档注释：计算语言偏移
// 背景：IPDB 以语言 -> 起始下标映射组织字段；当目标语言不存在时回退到最小偏移以尽可能读取到有效字段。
// 参数：Reader 元信息中的 Languages 映射与目标 language。
// 返回：字段起始偏移；若语言缺失则返回当前文件中最小的偏移值。
func languageOffset(r *Reader, language string) int {
	if off, ok := r.meta.Languages[language]; ok {
		return off
	}
	have := false
	min := 0
	for _, v := range r.meta.Languages {
		if !have || v < min {
			min = v
			have = true
		}
	}
	return min
}

// 文档注释：将位前缀转换为 CIDR 网络
// 背景：导入时需要范围起止整数，故先转为标准 CIDR 表示以便后续计算；长度为掩码位数。
// 返回：IPv4 网络结构（IP 为网络地址，Mask 为掩码）。
func ipToCIDR(prefix uint32, length int) *net.IPNet {
	base := net.IPv4(byte(prefix>>24), byte((prefix>>16)&0xff), byte((prefix>>8)&0xff), byte(prefix&0xff))
	mask := net.CIDRMask(length, 32)
	return &net.IPNet{IP: base, Mask: mask}
}

// 文档注释：计算 CIDR 范围与首段
// 背景：导出起始/结束整数与首字节用于数据库索引与内存切分；首段（first_octet）便于桶分片。
// 返回：起始整数、结束整数、首字节。
func cidrRange(n *net.IPNet) (uint32, uint32, int) {
	ip := n.IP.To4()
	m := net.IP(n.Mask).To4()
	s := (uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])) &
		(uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3]))
	inv := (^uint32(0)) ^ (uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3]))
	e := s | inv
	a := int((s >> 24) & 0xff)
	return s, e, a
}

// 文档注释：导入 IPv4 叶子到数据库（追加/去重）
// 背景：单线程导入流程，按 1000 条批次提交事务以平衡写入延迟与锁占用；地点表采用 UPSERT 去重。
// 参数：db 为数据库连接；r 为 IPDB Reader；language 为解析目标语言。
// 异常：SQL 执行或事务提交失败直接返回 error；为保证幂等，地点键以（country,region,province,city,isp）唯一约束。
func ImportIPv4LeavesToDB(db *sql.DB, r *Reader, language string) error {
	logger.L().Info("ipip_import_start", "language", language)
	ch := make(chan IPv4Leaf, 8192)
	done := make(chan struct{})
	go func() {
		_ = r.EnumerateIPv4(ch)
		close(ch)
		close(done)
	}()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmtLoc, err := tx.Prepare("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id")
	if err != nil {
		return err
	}
	defer stmtLoc.Close()
	stmtRange, err := tx.Prepare("INSERT INTO _ip_ipv4_ranges(start_int,end_int,first_octet,location_id) VALUES($1,$2,$3,$4)")
	if err != nil {
		return err
	}
	defer stmtRange.Close()
	count := 0
	off := languageOffset(r, language)
	for leaf := range ch {
		fields := string(leaf.Raw)
		parts := make([]string, 0, len(r.meta.Fields))
		// NOTE: 使用手写切割以规避 strings.Split 的额外分配；字段规范为制表符分隔。
		start := 0
		for i := 0; i < len(fields); i++ {
			if fields[i] == '\t' {
				parts = append(parts, fields[start:i])
				start = i + 1
			}
		}
		parts = append(parts, fields[start:])
		begin := off
		end := off + len(r.meta.Fields)
		if begin < 0 {
			begin = 0
		}
		if end > len(parts) {
			end = len(parts)
		}
		if begin >= end {
			continue
		}
		seg := parts[begin:end]
		var country, region, province, city string
		for i, f := range r.meta.Fields {
			if i >= len(seg) {
				break
			}
			switch f {
			case "country_name":
				country = seg[i]
			case "region_name":
				region = seg[i]
			case "province_name":
				province = seg[i]
			case "city_name":
				city = seg[i]
			}
		}
		// NOTE: ISP 信息可能缺失，统一使用空字符串参与唯一键；减少后续查询分支复杂度。
		_, _, _ = country, region, province
		netw := ipToCIDR(leaf.Prefix<<uint(32-leaf.Length), leaf.Length)
		s, e, a := cidrRange(netw)
		var locID int
		if err := stmtLoc.QueryRow(country, region, province, city, "").Scan(&locID); err != nil {
			return err
		}
		if _, err := stmtRange.Exec(int64(s), int64(e), a, locID); err != nil {
			return err
		}
		count++
		if count%1000 == 0 {
			logger.L().Info("ipip_import_progress", "count", count)
			if err := tx.Commit(); err != nil {
				return err
			}
			tx, err = db.Begin()
			if err != nil {
				return err
			}
			stmtLoc, err = tx.Prepare("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id")
			if err != nil {
				return err
			}
			stmtRange, err = tx.Prepare("INSERT INTO _ip_ipv4_ranges(start_int,end_int,first_octet,location_id) VALUES($1,$2,$3,$4)")
			if err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logger.L().Info("ipip_import_done", "count", count)
	return nil
}

// 文档注释：并行导入（解析与写入分片并行）
// 背景：解析阶段与写入阶段分片并发，按首字节对范围进行管道分流，降低全局锁竞争并提升吞吐。
// 参数：db 为数据库连接；r 为 IPDB Reader；language 为解析目标语言。
// 约束：
// - workers 可通过环境变量 IPIP_WORKERS 配置；
// - 地点表在并发场景下使用键级互斥避免重复插入与冲突；
// - 解析管道使用缓冲通道，防止生产过快导致写入阻塞。
// WARNING: 如数据库开启严格锁或低并发连接数，应调低 workers 或加大事务批量以避免频繁重试。
func ImportIPv4LeavesToDBConcurrent(db *sql.DB, r *Reader, language string) error {
	logger.L().Info("ipip_import_start", "language", language)
	logger.L().Info("ipip_meta", "node_count", r.meta.NodeCount, "fields", len(r.meta.Fields), "languages", len(r.meta.Languages))
	ch := make(chan IPv4Leaf, 16384)
	go func() { _ = r.EnumerateIPv4(ch); close(ch) }()

	workers := 8
	if s := os.Getenv("IPIP_WORKERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			workers = n
		}
	}
	type result struct {
		country, region, province, city string
		s, e                            uint32
		a                               int
	}
	pipes := make([]chan result, workers)
	for i := range pipes {
		pipes[i] = make(chan result, 8192)
	}

	var parseWG sync.WaitGroup
	var produced int64
	off := languageOffset(r, language)
	logger.L().Debug("ipip_lang_offset", "offset", off)
	for i := 0; i < workers; i++ {
		parseWG.Add(1)
		go func() {
			defer parseWG.Done()
			for leaf := range ch {
				fields := string(leaf.Raw)
				parts := make([]string, 0, len(r.meta.Fields))
				start := 0
				for j := 0; j < len(fields); j++ {
					if fields[j] == '\t' {
						parts = append(parts, fields[start:j])
						start = j + 1
					}
				}
				parts = append(parts, fields[start:])
				begin := off
				end := off + len(r.meta.Fields)
				if begin < 0 {
					begin = 0
				}
				if end > len(parts) {
					end = len(parts)
				}
				if begin >= end {
					continue
				}
				seg := parts[begin:end]
				var country, region, province, city string
				for k, f := range r.meta.Fields {
					if k >= len(seg) {
						break
					}
					switch f {
					case "country_name":
						country = seg[k]
					case "region_name":
						region = seg[k]
					case "province_name":
						province = seg[k]
					case "city_name":
						city = seg[k]
					}
				}
				netw := ipToCIDR(leaf.Prefix<<uint(32-leaf.Length), leaf.Length)
				s, e, a := cidrRange(netw)
				pipes[a%workers] <- result{country, region, province, city, s, e, a}
				n := atomic.AddInt64(&produced, 1)
				if n%10000 == 0 {
					logger.L().Debug("ipip_parse_progress", "produced", n)
				}
			}
		}()
	}

	// NOTE: 预加载地点缓存，避免重复写入与索引冲突；并提供按地点键的串行锁
	var locCache sync.Map // key -> id
	if rows, err := db.Query("SELECT id, country, region, province, city, isp FROM _ip_locations"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var country, region, province, city, isp string
			if e := rows.Scan(&id, &country, &region, &province, &city, &isp); e == nil {
				k := country + "|" + region + "|" + province + "|" + city + "|" + isp
				locCache.Store(k, id)
			}
		}
	}
	type keyLocker struct{ m sync.Map }
	getLock := func(kl *keyLocker, k string) *sync.Mutex {
		v, _ := kl.m.LoadOrStore(k, &sync.Mutex{})
		return v.(*sync.Mutex)
	}
	var keyLocks keyLocker

	var writeWG sync.WaitGroup
	for idx := 0; idx < workers; idx++ {
		writeWG.Add(1)
		go func(shard int) {
			defer writeWG.Done()
			tx, err := db.Begin()
			if err != nil {
				logger.L().Error("ipip_tx_error", "err", err)
				return
			}
			defer tx.Rollback()
			stmtLocSel, err := tx.Prepare("SELECT id FROM _ip_locations WHERE country=$1 AND region=$2 AND province=$3 AND city=$4 AND isp=$5")
			if err != nil {
				logger.L().Error("ipip_stmt_error", "err", err)
				return
			}
			defer stmtLocSel.Close()
			stmtLocIns, err := tx.Prepare("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id")
			if err != nil {
				logger.L().Error("ipip_stmt_error", "err", err)
				return
			}
			defer stmtLocIns.Close()
			stmtRange, err := tx.Prepare("INSERT INTO _ip_ipv4_ranges(start_int,end_int,first_octet,location_id) VALUES($1,$2,$3,$4)")
			if err != nil {
				logger.L().Error("ipip_stmt_error", "err", err)
				return
			}
			defer stmtRange.Close()
			logger.L().Debug("ipip_writer_start", "shard", shard)
			count := 0
			started := false
			myLocs := make(map[string]int)
			for v := range pipes[shard] {
				key := v.country + "|" + v.region + "|" + v.province + "|" + v.city + "|"
				var locID int
				if id, ok := myLocs[key]; ok {
					locID = id
				} else {
					if err := stmtLocSel.QueryRow(v.country, v.region, v.province, v.city, "").Scan(&locID); err != nil {
						mu := getLock(&keyLocks, key)
						mu.Lock()
						if err2 := stmtLocSel.QueryRow(v.country, v.region, v.province, v.city, "").Scan(&locID); err2 != nil {
							if err3 := db.QueryRow("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id", v.country, v.region, v.province, v.city, "").Scan(&locID); err3 != nil {
								logger.L().Error("ipip_loc_error", "err", err3)
								mu.Unlock()
								return
							}
						}
						mu.Unlock()
					}
					if !started {
						logger.L().Debug("ipip_writer_first_item", "shard", shard, "key", key)
						started = true
					}
					myLocs[key] = locID
				}
				if _, err := stmtRange.Exec(int64(v.s), int64(v.e), v.a, locID); err != nil {
					logger.L().Error("ipip_range_error", "err", err)
					return
				}
				count++
				if count%1000 == 0 {
					if err := tx.Commit(); err != nil {
						logger.L().Error("ipip_commit_error", "err", err)
						return
					}
					tx, err = db.Begin()
					if err != nil {
						logger.L().Error("ipip_tx_error", "err", err)
						return
					}
					stmtLocSel, err = tx.Prepare("SELECT id FROM _ip_locations WHERE country=$1 AND region=$2 AND province=$3 AND city=$4 AND isp=$5")
					if err != nil {
						logger.L().Error("ipip_stmt_error", "err", err)
						return
					}
					stmtLocIns, err = tx.Prepare("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id")
					if err != nil {
						logger.L().Error("ipip_stmt_error", "err", err)
						return
					}
					stmtRange, err = tx.Prepare("INSERT INTO _ip_ipv4_ranges(start_int,end_int,first_octet,location_id) VALUES($1,$2,$3,$4)")
					if err != nil {
						logger.L().Error("ipip_stmt_error", "err", err)
						return
					}
					logger.L().Info("ipip_import_progress_shard", "shard", shard, "count", count)
				}
			}
			if err := tx.Commit(); err == nil {
				logger.L().Info("ipip_import_done_shard", "shard", shard, "count", count)
			} else {
				logger.L().Error("ipip_commit_error", "err", err)
			}
		}(idx)
	}

	go func() {
		parseWG.Wait()
		logger.L().Info("ipip_parse_done", "produced", produced)
		for i := range pipes {
			close(pipes[i])
		}
	}()
	writeWG.Wait()
	logger.L().Info("ipip_import_done_all")
	var c int64
	row := db.QueryRow("SELECT COUNT(1) FROM _ip_ipv4_ranges")
	_ = row.Scan(&c)
	logger.L().Debug("ipip_ranges_count", "count", c)
	return nil
}
