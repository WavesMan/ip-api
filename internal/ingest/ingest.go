// 包 ingest：提供上游数据集拉取与批量导入逻辑，作为离线数据通道
package ingest

import (
    "bufio"
    "database/sql"
    "errors"
    "fmt"
    "ip-api/internal/logger"
    "net/http"
    "strings"
)

// ipToInt：将 IPv4 文本转换为无符号整数
// 为什么：范围匹配基于整数比较，需在导入阶段生成稳定的数值表示
// 约束：非法输入返回错误，不做纠错以确保数据质量
func ipToInt(ip string) (uint32, error) {
	var a, b, c, d int
	n, err := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d)
	if err != nil || n != 4 {
		return 0, errors.New("bad ip")
	}
	if a < 0 || a > 255 || b < 0 || b > 255 || c < 0 || c > 255 || d < 0 || d > 255 {
		return 0, errors.New("bad ip")
	}
	x := uint32(a)<<24 | uint32(b)<<16 | uint32(c)<<8 | uint32(d)
	return x, nil
}

// FetchAndImport：拉取上游并批量写入数据库（字典与范围）
// 背景：按行解析 ip2region 源，5000 行为一批提交，降低锁持有与 WAL 压力
// 异常：网络错误/解析失败/数据库错误直接返回，不做重试（交由调度层处理）
func FetchAndImport(db *sql.DB, srcURL string) error {
    logger.L().Info("ingest_start", "src", srcURL)
    resp, err := http.Get(srcURL)
    if err != nil {
        return err
    }
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("bad status")
	}

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

	rd := bufio.NewScanner(resp.Body)
	rd.Buffer(make([]byte, 1024), 1024*1024)
    count := 0
    for rd.Scan() {
		line := strings.TrimSpace(rd.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}
		s := parts[0]
		e := parts[1]
		country := parts[2]
		region := parts[3]
		province := parts[4]
		city := ""
		isp := ""
		if len(parts) >= 7 {
			city = parts[5]
		}
		if len(parts) >= 8 {
			isp = parts[6]
		}
		start, err := ipToInt(s)
		if err != nil {
			continue
		}
		end, err := ipToInt(e)
		if err != nil {
			continue
		}
		var locID int
		err = stmtLoc.QueryRow(country, region, province, city, isp).Scan(&locID)
		if err != nil {
			return err
		}
		a := int((start >> 24) & 0xff)
		_, err = stmtRange.Exec(int64(start), int64(end), a, locID)
		if err != nil {
			return err
		}
		count++
        if count%5000 == 0 {
            logger.L().Info("ingest_progress", "count", count)
            if err = tx.Commit(); err != nil {
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
    if err = rd.Err(); err != nil {
        return err
    }
    if err = tx.Commit(); err != nil { return err }
    logger.L().Info("ingest_done", "count", count)
    return nil
}

// EnsureInitialized：检查范围表是否为空；为空则执行一次初始化导入
// 为什么：简化部署流程，避免独立手动导入步骤
func EnsureInitialized(db *sql.DB, srcURL string) error {
	var c int64
	row := db.QueryRow("SELECT COUNT(1) FROM _ip_ipv4_ranges")
	_ = row.Scan(&c)
	if c > 0 {
		return nil
	}
	return FetchAndImport(db, srcURL)
}
