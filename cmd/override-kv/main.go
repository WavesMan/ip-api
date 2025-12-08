package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"ip-api/internal/migrate"
	"ip-api/internal/utils"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func ipToInt(ip string) (uint32, error) {
	var a, b, c, d int
	n, err := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d)
	if err != nil || n != 4 {
		return 0, fmt.Errorf("bad ip")
	}
	if a < 0 || a > 255 || b < 0 || b > 255 || c < 0 || c > 255 || d < 0 || d > 255 {
		return 0, fmt.Errorf("bad ip")
	}
	x := uint32(a)<<24 | uint32(b)<<16 | uint32(c)<<8 | uint32(d)
	return x, nil
}

func ensureSchema(db *sql.DB) error {
	return migrate.EnsureSchema(db)
}

func upsertKV(db *sql.DB, key string, ip string, country, region, province, city, isp string) error {
	v, err := ipToInt(ip)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO _ip_overrides_kv(assoc_key, ip_int, country, region, province, city, isp)
        VALUES($1,$2,$3,$4,$5,$6,$7)
        ON CONFLICT (assoc_key, ip_int) DO UPDATE SET country=EXCLUDED.country, region=EXCLUDED.region, province=EXCLUDED.province, city=EXCLUDED.city, isp=EXCLUDED.isp, updated_at=now()`,
		key, int64(v), country, region, province, city, isp,
	)
	return err
}

func delKV(db *sql.DB, key string, ip string) error {
	v, err := ipToInt(ip)
	if err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM _ip_overrides_kv WHERE assoc_key=$1 AND ip_int=$2`, key, int64(v))
	return err
}

func getKV(db *sql.DB, key string, ip string) (string, error) {
	v, err := ipToInt(ip)
	if err != nil {
		return "", err
	}
	row := db.QueryRow(`SELECT country, region, province, city, isp FROM _ip_overrides_kv WHERE assoc_key=$1 AND ip_int=$2`, key, int64(v))
	var c, r, p, ci, isp string
	if err := row.Scan(&c, &r, &p, &ci, &isp); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s | %s | %s | %s | %s", c, r, p, ci, isp), nil
}

func listKV(db *sql.DB, key string, limit int) ([]string, error) {
	rows, err := db.Query(`SELECT ip_int, country, region, province, city, isp FROM _ip_overrides_kv WHERE assoc_key=$1 ORDER BY updated_at DESC LIMIT $2`, key, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v int64
		var c, r, p, ci, isp string
		if err := rows.Scan(&v, &c, &r, &p, &ci, &isp); err != nil {
			return nil, err
		}
		a := (v >> 24) & 0xff
		b := (v >> 16) & 0xff
		c1 := (v >> 8) & 0xff
		d := v & 0xff
		ip := fmt.Sprintf("%d.%d.%d.%d", a, b, c1, d)
		out = append(out, fmt.Sprintf("%s -> %s | %s | %s | %s | %s", ip, c, r, p, ci, isp))
	}
	return out, nil
}

func findKeys(db *sql.DB, ip string) ([]string, error) {
	v, err := ipToInt(ip)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT DISTINCT assoc_key FROM _ip_overrides_kv WHERE ip_int=$1`, int64(v))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

func printHelp() {
	fmt.Println("commands:")
	fmt.Println("  add <key> <ip> <country> <region> [province] [city] [isp]")
	fmt.Println("  set <key> <ip> <country> <region> [province] [city] [isp]")
	fmt.Println("  del <key> <ip>")
	fmt.Println("  get <key> <ip>")
	fmt.Println("  list <key> [limit]")
	fmt.Println("  keys <ip>")
	fmt.Println("  help")
	fmt.Println("  exit")
}

func prompt(r *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	s, _ := r.ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}

func main() {
	var envFile string
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--env" && i+1 < len(os.Args) {
			envFile = os.Args[i+1]
			i++
		} else if strings.HasSuffix(os.Args[i], ".env") {
			envFile = os.Args[i]
		}
	}
	var db *sql.DB
	var err error
	if envFile != "" {
		_ = godotenv.Load(envFile)
		db, err = utils.OpenPostgresFromEnv()
	} else {
		r := bufio.NewReader(os.Stdin)
		fmt.Println("输入数据库连接参数，回车使用默认值")
		host := prompt(r, "PG_HOST", "127.0.0.1")
		port := prompt(r, "PG_PORT", "5432")
		user := prompt(r, "PG_USER", "postgres")
		pass := prompt(r, "PG_PASSWORD", "")
		name := prompt(r, "PG_DB", "ipapi")
		ssl := prompt(r, "PG_SSLMODE", "disable")
		dsn := "postgres://" + user
		if pass != "" {
			dsn += ":" + pass
		}
		dsn += "@" + host + ":" + port + "/" + name + "?sslmode=" + ssl
		db, err = utils.OpenPostgres(dsn)
	}
	if err != nil {
		fmt.Println("db error:", err)
		os.Exit(1)
	}
	if err := ensureSchema(db); err != nil {
		fmt.Println("schema error:", err)
		os.Exit(1)
	}
	defer db.Close()
	fmt.Println("override kv cli ready")
	printHelp()
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !in.Scan() {
			break
		}
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])
		switch cmd {
		case "exit", "quit":
			return
		case "help":
			printHelp()
		case "add", "set":
			if len(parts) < 5 {
				fmt.Println("usage: add <key> <ip> <country> <region> [province] [city] [isp]")
				continue
			}
			key := parts[1]
			ip := parts[2]
			country := parts[3]
			region := parts[4]
			province := ""
			city := ""
			isp := ""
			if len(parts) >= 6 {
				province = parts[5]
			}
			if len(parts) >= 7 {
				city = parts[6]
			}
			if len(parts) >= 8 {
				isp = parts[7]
			}
			if err := upsertKV(db, key, ip, country, region, province, city, isp); err != nil {
				fmt.Println("error:", err)
			} else {
				fmt.Println("ok")
			}
		case "del":
			if len(parts) < 3 {
				fmt.Println("usage: del <key> <ip>")
				continue
			}
			if err := delKV(db, parts[1], parts[2]); err != nil {
				fmt.Println("error:", err)
			} else {
				fmt.Println("ok")
			}
		case "get":
			if len(parts) < 3 {
				fmt.Println("usage: get <key> <ip>")
				continue
			}
			s, err := getKV(db, parts[1], parts[2])
			if err != nil {
				fmt.Println("error:", err)
			} else {
				fmt.Println(s)
			}
		case "list":
			if len(parts) < 2 {
				fmt.Println("usage: list <key> [limit]")
				continue
			}
			limit := 20
			if len(parts) >= 3 {
				if n, e := strconv.Atoi(parts[2]); e == nil && n > 0 {
					limit = n
				}
			}
			xs, err := listKV(db, parts[1], limit)
			if err != nil {
				fmt.Println("error:", err)
				continue
			}
			for _, s := range xs {
				fmt.Println(s)
			}
		case "keys", "key":
			if len(parts) < 2 {
				fmt.Println("usage: keys <ip>")
				continue
			}
			ks, err := findKeys(db, parts[1])
			if err != nil {
				fmt.Println("error:", err)
				continue
			}
			if len(ks) == 0 {
				fmt.Println("none")
			} else {
				for _, k := range ks {
					fmt.Println(k)
				}
			}
		default:
			fmt.Println("unknown command")
		}
	}
}
