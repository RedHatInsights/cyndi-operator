package database

import (
	"fmt"

	"github.com/jackc/pgx"

	"cyndi-operator/controllers/config"
	. "cyndi-operator/controllers/config"
)

type Database struct {
	Config     *DBParams
	connection *pgx.Conn
}

const connectionStringTemplate = "host=%s user=%s password=%s dbname=%s port=%s"

func NewDatabase(config *config.DBParams) *Database {
	return &Database{
		Config: config,
	}
}

func (db *Database) Connect() (err error) {
	if db.connection, err = GetConnection(db.Config); err != nil {
		return err
	}

	return nil
}

func (db *Database) Close() error {
	if db.connection != nil {
		return db.connection.Close()
	}

	return nil
}

func (db *Database) runQuery(query string) (*pgx.Rows, error) {
	rows, err := db.connection.Query(query)

	if err != nil {
		return nil, fmt.Errorf("Error executing query %s, %w", query, err)
	}

	return rows, nil
}

func (db *Database) Exec(query string) (result pgx.CommandTag, err error) {
	result, err = db.connection.Exec(query)

	if err != nil {
		return result, fmt.Errorf("Error executing query %s, %w", query, err)
	}

	return result, nil
}

func (db *Database) CountHosts(table string) (int64, error) {
	// TODO: add modified_on filter
	//query := fmt.Sprintf(
	//	"SELECT count(*) FROM %s WHERE modified_on < '%s'", table, i.Now)
	// also add "AND canonical_facts ? 'insights_id'"
	// waiting on https://issues.redhat.com/browse/RHCLOUD-9545

	query := fmt.Sprintf("SELECT count(*) FROM %s", table)

	rows, err := db.runQuery(query)

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

// TODO move to database
func (db *Database) GetHostIds(table string) ([]string, error) {
	// TODO" "AND canonical_facts ? 'insights_id'" when !view and insightsOnly
	// also add "AND canonical_facts ? 'insights_id'"
	// waiting on https://issues.redhat.com/browse/RHCLOUD-9545
	query := fmt.Sprintf("SELECT id FROM %s ORDER BY id", table)
	rows, err := db.runQuery(query)

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
		params.Host,
		params.User,
		params.Password,
		params.Name,
		params.Port)

	if config, err := pgx.ParseDSN(connStr); err != nil {
		return nil, err
	} else {
		if connection, err = pgx.Connect(config); err != nil {
			return nil, err
		} else {
			return connection, nil
		}
	}
}
