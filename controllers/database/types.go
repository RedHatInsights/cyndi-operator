package database

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Database interface {
	Connect() error
	Close() error
	RunQuery(query string) (pgx.Rows, error)
	Exec(query string) (result pgconn.CommandTag, err error)
	CountHosts(table string, insightsOnly bool, additionalFilters []map[string]string) (int64, error)
	GetHostIds(table string, insightsOnly bool, additionalFilters []map[string]string) ([]string, error)
}
