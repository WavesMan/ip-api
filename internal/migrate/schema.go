package migrate

import (
    "database/sql"
    "ip-api/internal/logger"
)

// 背景：首次运行自动创建所需表与索引，保障后续导入与查询
// 约束：使用 IF NOT EXISTS 避免与既有结构冲突；仅创建最小必需结构
func EnsureSchema(db *sql.DB) error {
    stmts := []string{
		`CREATE TABLE IF NOT EXISTS _ip_locations (
            id SERIAL PRIMARY KEY,
            country TEXT NOT NULL,
            region TEXT NOT NULL,
            province TEXT NOT NULL,
            city TEXT NOT NULL,
            isp TEXT NOT NULL
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_location ON _ip_locations(country,region,province,city,isp)`,
		`CREATE TABLE IF NOT EXISTS _ip_ipv4_ranges (
            start_int BIGINT NOT NULL,
            end_int BIGINT NOT NULL,
            first_octet INT NOT NULL,
            location_id INT NOT NULL REFERENCES _ip_locations(id)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_ipv4_first_start ON _ip_ipv4_ranges(first_octet, start_int)`,
		`CREATE TABLE IF NOT EXISTS _ip_stats_total (
            id INT PRIMARY KEY,
            total_queries BIGINT NOT NULL DEFAULT 0,
            total_visitors BIGINT NOT NULL DEFAULT 0
        )`,
        `CREATE TABLE IF NOT EXISTS _ip_stats_daily (
            day DATE PRIMARY KEY,
            queries BIGINT NOT NULL DEFAULT 0,
            visitors BIGINT NOT NULL DEFAULT 0
        )`,
        `INSERT INTO _ip_stats_total(id, total_queries, total_visitors)
         VALUES(1, 0, 0)
         ON CONFLICT (id) DO NOTHING`,
        `CREATE TABLE IF NOT EXISTS _ip_overrides (
            ip_int BIGINT PRIMARY KEY,
            location_id INT NOT NULL REFERENCES _ip_locations(id)
        )`,
    }
    for i, s := range stmts {
        logger.L().Debug("schema_exec", "idx", i)
        if _, err := db.Exec(s); err != nil {
            return err
        }
    }
	// 外键调整为可延迟检查，降低并行写入时父子可见性问题
    if _, err := db.Exec(`ALTER TABLE _ip_ipv4_ranges DROP CONSTRAINT IF EXISTS _ip_ipv4_ranges_location_id_fkey`); err != nil {
        return err
    }
    if _, err := db.Exec(`ALTER TABLE _ip_ipv4_ranges ADD CONSTRAINT _ip_ipv4_ranges_location_id_fkey FOREIGN KEY (location_id) REFERENCES _ip_locations(id) DEFERRABLE INITIALLY DEFERRED`); err != nil {
        return err
    }
    logger.L().Debug("schema_done")
    return nil
}
