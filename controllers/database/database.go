package database

import (
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"strings"

	"github.com/jackc/pgx"

	"github.com/RedHatInsights/cyndi-operator/controllers/config"
	. "github.com/RedHatInsights/cyndi-operator/controllers/config"
)

type BaseDatabase struct {
	Config     *DBParams
	connection *pgx.Conn
	Log        logr.Logger
}

const connectionStringTemplate = "postgresql://%s:%s@%s:%s/%s?sslmode=%s&sslrootcert=%s"

func NewBaseDatabase(config *config.DBParams, log logr.Logger) Database {
	return &BaseDatabase{
		Config: config,
		Log:    log,
	}
}

func (db *BaseDatabase) Connect() (err error) {
	if db.connection, err = GetConnection(db.Config); err != nil {
		return fmt.Errorf("Error connecting to %s:%s/%s as %s : %s", db.Config.Host, db.Config.Port, db.Config.Name, db.Config.User, err)
	}

	return nil
}

func (db *BaseDatabase) Close() error {
	if db.connection != nil {
		return db.connection.Close()
	}

	return nil
}

func (db *BaseDatabase) RunQuery(query string) (*pgx.Rows, error) {
	if db.Log != nil {
		db.Log.Info("DB Query", "query", query)
	}

	if db.connection == nil {
		return nil, errors.New("cannot run query because there is no database connection")
	}

	rows, err := db.connection.Query(query)

	if err != nil {
		return nil, fmt.Errorf("Error executing query %s, %w", query, err)
	}

	return rows, nil
}

func (db *BaseDatabase) Exec(query string) (result pgx.CommandTag, err error) {
	if db.connection == nil {
		return result, errors.New("cannot run query because there is no database connection")
	}

	result, err = db.connection.Exec(query)

	if err != nil {
		return result, fmt.Errorf("Error executing query %s, %w", query, err)
	}

	return result, nil
}

func (db *BaseDatabase) getWhereClause(insightsOnly bool, additionalFilters []map[string]string) string {
	length := len(additionalFilters)
	if insightsOnly {
		length += 1
	}

	where := make([]string, length)

	for i, filter := range additionalFilters {
		where[i] = "(" + filter["where"] + ")"
	}

	if insightsOnly {
		where[length-1] = "(canonical_facts ? 'insights_id')"
	}

	query := strings.Join(where, " AND ")
	if len(query) > 0 {
		return "WHERE " + query
	}

	return ""
}

func (db *BaseDatabase) hostCountQuery(table string, insightsOnly bool, additionalFilters []map[string]string) string {
	return fmt.Sprintf(`SELECT count(*) FROM %s %s`, table, db.getWhereClause(insightsOnly, additionalFilters))
}

func (db *BaseDatabase) CountHosts(table string, insightsOnly bool, additionalFilters []map[string]string) (int64, error) {
	// TODO: add modified_on filter
	// waiting on https://issues.redhat.com/browse/RHCLOUD-9545
	rows, err := db.RunQuery(db.hostCountQuery(table, insightsOnly, additionalFilters))

	if err != nil {
		return -1, err
	}

	defer rows.Close()

	var response int64
	for rows.Next() {
		var count int64
		err = rows.Scan(&count)
		if err != nil {
			return -1, err
		}
		response = count
	}

	if err != nil {
		return -1, err
	}

	return response, err
}

func (db *BaseDatabase) hostIdQuery(table string, insightsOnly bool, additionalFilters []map[string]string) string {
	return fmt.Sprintf(`SELECT id FROM %s %s ORDER BY id`, table, db.getWhereClause(insightsOnly, additionalFilters))
}

func (db *BaseDatabase) GetHostIds(table string, insightsOnly bool, additionalFilters []map[string]string) ([]string, error) {
	// TODO: add modified_on filter
	// waiting on https://issues.redhat.com/browse/RHCLOUD-9545
	rows, err := db.RunQuery(db.hostIdQuery(table, insightsOnly, additionalFilters))

	var ids []string

	if err != nil {
		return ids, err
	}

	defer rows.Close()

	for rows.Next() {
		var id string
		err = rows.Scan(&id)

		if err != nil {
			return ids, err
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func GetConnection(params *DBParams) (connection *pgx.Conn, err error) {
	connStr := fmt.Sprintf(
		connectionStringTemplate,
		params.User,
		params.Password,
		params.Host,
		params.Port,
		params.Name,
		params.SSLMode,
		params.SSLRootCert,
	)

	if config, err := pgx.ParseConnectionString(connStr); err != nil {
		return nil, err
	} else {
		if connection, err = pgx.Connect(config); err != nil {
			return nil, err
		} else {
			return connection, nil
		}
	}
}
