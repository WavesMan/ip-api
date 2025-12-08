// 数据导入工具：从上游数据集拉取并批量写入 PostgreSQL（字典与范围）
package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"ip-api/internal/utils"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

// 将 IPv4 文本转换为无符号整数，非法输入返回错误
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

// 读取上游、按行解析并批量 UPSERT 字典与范围；定期提交降低锁与日志压力
func main() {
	dsn := utils.BuildPostgresDSNFromEnv()
	url := os.Getenv("SRC_URL")
	if url == "" {
		url = "https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ipv4_source.txt"
	}
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatal("bad status")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	// 字典 UPSERT，避免重复并返回主键 id
	stmtLoc, err := tx.Prepare("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id")
	if err != nil {
		log.Fatal(err)
	}
	defer stmtLoc.Close()
	// 范围插入，包含首段分桶 first_octet 以支持分区与查询路由
	stmtRange, err := tx.Prepare("INSERT INTO _ip_ipv4_ranges(start_int,end_int,first_octet,location_id) VALUES($1,$2,$3,$4)")
	if err != nil {
		log.Fatal(err)
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
			log.Fatal(err)
		}
		a := int((start >> 24) & 0xff)
		_, err = stmtRange.Exec(int64(start), int64(end), a, locID)
		if err != nil {
			log.Fatal(err)
		}
		count++
		if count%5000 == 0 {
			err = tx.Commit()
			if err != nil {
				log.Fatal(err)
			}
			tx, err = db.Begin()
			if err != nil {
				log.Fatal(err)
			}
			stmtLoc, err = tx.Prepare("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id")
			if err != nil {
				log.Fatal(err)
			}
			stmtRange, err = tx.Prepare("INSERT INTO _ip_ipv4_ranges(start_int,end_int,first_octet,location_id) VALUES($1,$2,$3,$4)")
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	err = rd.Err()
	if err != nil {
		log.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("imported", count)
}
