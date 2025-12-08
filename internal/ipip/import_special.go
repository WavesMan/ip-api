package ipip

import (
	"database/sql"
	"ip-api/internal/logger"
)

func ImportIPv4LeavesToSpecial(db *sql.DB, r *Reader, language, sourceTag string) error {
	logger.L().Info("ipip_special_import_start", "language", language, "source", sourceTag)
	ch := make(chan IPv4Leaf, 8192)
	go func() { _ = r.EnumerateIPv4(ch); close(ch) }()
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
	stmtRange, err := tx.Prepare("INSERT INTO _ip_cidr_special(start_int,end_int,first_octet,location_id,source_tag,active) VALUES($1,$2,$3,$4,$5,TRUE)")
	if err != nil {
		return err
	}
	defer stmtRange.Close()
	count := 0
	off := languageOffset(r, language)
	for leaf := range ch {
		fields := string(leaf.Raw)
		parts := make([]string, 0, len(r.meta.Fields))
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
		netw := ipToCIDR(leaf.Prefix<<uint(32-leaf.Length), leaf.Length)
		s, e, a := cidrRange(netw)
		var locID int
		if err := stmtLoc.QueryRow(country, region, province, city, "").Scan(&locID); err != nil {
			return err
		}
		if _, err := stmtRange.Exec(int64(s), int64(e), a, locID, sourceTag); err != nil {
			return err
		}
		count++
		if count%2000 == 0 {
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
			stmtRange, err = tx.Prepare("INSERT INTO _ip_cidr_special(start_int,end_int,first_octet,location_id,source_tag,active) VALUES($1,$2,$3,$4,$5,TRUE)")
			if err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logger.L().Info("ipip_special_import_done", "count", count)
	return nil
}
