package ingest

import (
	"context"
	"database/sql"
	"ip-api/internal/logger"
)

type Location struct{ Country, Region, Province, City, ISP string }

func upsertLocation(ctx context.Context, db *sql.DB, l Location) (int, error) {
	var id int
	err := db.QueryRowContext(ctx, "INSERT INTO _ip_locations(country,region,province,city,isp) VALUES($1,$2,$3,$4,$5) ON CONFLICT (country,region,province,city,isp) DO UPDATE SET country=EXCLUDED.country RETURNING id",
		l.Country, l.Region, l.Province, l.City, l.ISP).Scan(&id)
	return id, err
}

func WriteExact(ctx context.Context, db *sql.DB, ipInt uint32, l Location, sourceTag string) error {
	logger.L().Debug("ingest_exact_begin", "ip_int", int64(ipInt), "source", sourceTag)
	lid, err := upsertLocation(ctx, db, l)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO _ip_exact(ip_int,location_id,source_tag,updated_at) VALUES($1,$2,$3,now()) ON CONFLICT (ip_int) DO UPDATE SET location_id=EXCLUDED.location_id, source_tag=EXCLUDED.source_tag, updated_at=now()",
		int64(ipInt), lid, sourceTag)
	if err == nil {
		logger.L().Debug("ingest_exact_ok", "ip_int", int64(ipInt), "loc_id", lid)
	} else {
		logger.L().Error("ingest_exact_error", "err", err)
	}
	return err
}
