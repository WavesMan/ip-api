package utils

import (
	"database/sql"
	"os"
	"strconv"

	_ "github.com/lib/pq"
)

func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	return db, nil
}

func BuildPostgresDSNFromEnv() string {
	host := os.Getenv("PG_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("PG_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("PG_USER")
	if user == "" {
		user = "postgres"
	}
	pass := os.Getenv("PG_PASSWORD")
	db := os.Getenv("PG_DB")
	if db == "" {
		db = "ipapi"
	}
	ssl := os.Getenv("PG_SSLMODE")
	if ssl == "" {
		ssl = "disable"
	}
	dsn := "postgres://" + user
	if pass != "" {
		dsn += ":" + pass
	}
	dsn += "@" + host + ":" + port + "/" + db + "?sslmode=" + ssl
	return dsn
}

func OpenPostgresFromEnv() (*sql.DB, error) {
	dsn := BuildPostgresDSNFromEnv()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	maxOpen := 50
	maxIdle := 25
	if v := os.Getenv("PG_MAX_OPEN_CONNS"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			maxOpen = n
		}
	}
	if v := os.Getenv("PG_MAX_IDLE_CONNS"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			maxIdle = n
		}
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	return db, nil
}
