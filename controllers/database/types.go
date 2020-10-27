package database

import (
	"github.com/jackc/pgx"
)

type Database interface {
	Connect() error
	Close() error
	RunQuery(query string) (*pgx.Rows, error)
	Exec(query string) (result pgx.CommandTag, err error)
	CountHosts(table string) (int64, error)
	GetHostIds(table string) ([]string, error)
}
