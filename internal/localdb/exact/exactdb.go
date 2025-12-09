package exact

import (
    "database/sql"
    "encoding/binary"
    "ip-api/internal/localdb"
    "ip-api/internal/logger"
    "net"
    "os"
    "path/filepath"
    "sort"
)

type ExactDB struct {
    f     *os.File
    count int
    db    *sql.DB
}

func BuildExactDBFromDB(dir string, db *sql.DB) error {
    logger.L().Info("exactdb_build_begin", "dir", dir)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    fp := filepath.Join(dir, "exact.db")
    tmp := fp + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return err
    }
    defer f.Close()
    rows, err := db.Query("SELECT ip_int, location_id FROM _ip_overrides ORDER BY ip_int")
    if err != nil {
        return err
    }
    defer rows.Close()
    if _, err := f.Write([]byte{'E', 'X', 'D', 'B'}); err != nil {
        return err
    }
    if err := binary.Write(f, binary.BigEndian, uint32(1)); err != nil {
        return err
    }
    var recs [][2]uint32
    m := make(map[uint32]uint32)
    for rows.Next() {
        var v int64
        var lid int
        if err := rows.Scan(&v, &lid); err != nil {
            return err
        }
        m[uint32(v)] = uint32(lid)
    }
    rows2, err := db.Query("SELECT ip_int, country, region, province, city, isp FROM _ip_overrides_kv ORDER BY ip_int")
    if err == nil {
        defer rows2.Close()
        for rows2.Next() {
            var v int64
            var c, r, p, ci, isp string
            if err := rows2.Scan(&v, &c, &r, &p, &ci, &isp); err != nil {
                return err
            }
            var locID int
            row := db.QueryRow("SELECT id FROM _ip_locations WHERE country=$1 AND region=$2 AND province=$3 AND city=$4 AND isp=$5", c, r, p, ci, isp)
            if err := row.Scan(&locID); err != nil {
                err2 := db.QueryRow("INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) RETURNING id", c, r, p, ci, isp).Scan(&locID)
                if err2 != nil {
                    return err2
                }
            }
            m[uint32(v)] = uint32(locID)
        }
    }
    for k, v := range m {
        recs = append(recs, [2]uint32{k, v})
    }
    sort.Slice(recs, func(i, j int) bool { return recs[i][0] < recs[j][0] })
    if err := binary.Write(f, binary.BigEndian, uint32(len(recs))); err != nil {
        return err
    }
    for _, r := range recs {
        if err := binary.Write(f, binary.BigEndian, r[0]); err != nil {
            return err
        }
        if err := binary.Write(f, binary.BigEndian, r[1]); err != nil {
            return err
        }
    }
    if err := f.Sync(); err != nil {
        return err
    }
    if err := os.Rename(tmp, fp); err != nil {
        return err
    }
    logger.L().Info("exactdb_build_done", "count", len(recs))
    return nil
}

func NewExactDB(dir string, db *sql.DB) (*ExactDB, error) {
    fp := filepath.Join(dir, "exact.db")
    f, err := os.Open(fp)
    if err != nil {
        return nil, err
    }
    hdr := make([]byte, 12)
    if _, err := f.ReadAt(hdr, 0); err != nil {
        f.Close()
        return nil, err
    }
    if string(hdr[:4]) != "EXDB" {
        f.Close()
        return nil, os.ErrInvalid
    }
    cnt := int(binary.BigEndian.Uint32(hdr[8:12]))
    logger.L().Debug("exactdb_open", "dir", dir, "count", cnt)
    return &ExactDB{f: f, count: cnt, db: db}, nil
}

func (e *ExactDB) Lookup(ip string) (localdb.Location, bool) {
    var zero localdb.Location
    p := net.ParseIP(ip)
    if p == nil || p.To4() == nil {
        return zero, false
    }
    v := p.To4()
    val := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
    lo, hi := 0, e.count-1
    base := int64(12)
    for lo <= hi {
        mid := (lo + hi) >> 1
        off := base + int64(mid)*8
        buf := make([]byte, 8)
        if _, err := e.f.ReadAt(buf, off); err != nil {
            return zero, false
        }
        k := binary.BigEndian.Uint32(buf[:4])
        if val < k {
            hi = mid - 1
        } else if val > k {
            lo = mid + 1
        } else {
            lid := int(binary.BigEndian.Uint32(buf[4:8]))
            var l localdb.Location
            row := e.db.QueryRow("SELECT country, region, province, city, isp FROM _ip_locations WHERE id=$1", lid)
            if err := row.Scan(&l.Country, &l.Region, &l.Province, &l.City, &l.ISP); err != nil {
                return zero, false
            }
            logger.L().Debug("exactdb_lookup_hit", "ip_val", int64(val), "loc_id", lid)
            return l, true
        }
    }
    return zero, false
}

func (e *ExactDB) Close() error { return e.f.Close() }
