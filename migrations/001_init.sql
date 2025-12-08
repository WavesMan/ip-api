CREATE TABLE IF NOT EXISTS _ip_locations (
  id SERIAL PRIMARY KEY,
  country TEXT NOT NULL,
  region TEXT NOT NULL,
  province TEXT NOT NULL,
  city TEXT NOT NULL,
  isp TEXT NOT NULL,
  UNIQUE(country, region, province, city, isp)
);

CREATE TABLE IF NOT EXISTS _ip_ipv4_ranges (
  id BIGSERIAL PRIMARY KEY,
  start_int BIGINT NOT NULL,
  end_int BIGINT NOT NULL,
  first_octet SMALLINT NOT NULL,
  location_id INTEGER NOT NULL REFERENCES _ip_locations(id),
  CHECK (start_int <= end_int)
);

CREATE INDEX IF NOT EXISTS idx_ipv4_ranges_first_octet ON _ip_ipv4_ranges(first_octet);
CREATE INDEX IF NOT EXISTS idx_ipv4_ranges_span ON _ip_ipv4_ranges(start_int, end_int);

CREATE TABLE IF NOT EXISTS _ip_stats_total (
  id SMALLINT PRIMARY KEY DEFAULT 1,
  total_queries BIGINT NOT NULL DEFAULT 0,
  total_visitors BIGINT NOT NULL DEFAULT 0
);

INSERT INTO _ip_stats_total(id) VALUES(1) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS _ip_stats_daily (
  day DATE PRIMARY KEY,
  queries BIGINT NOT NULL DEFAULT 0,
  visitors BIGINT NOT NULL DEFAULT 0
);

