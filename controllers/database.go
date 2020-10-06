package controllers

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/jackc/pgx"
)

const connectionStringTemplate = "host=%s user=%s password=%s dbname=%s port=%s"

const viewTemplate = `CREATE OR REPLACE VIEW inventory.hosts AS SELECT
	id,
	account,
	display_name,
	created,
	updated,
	stale_timestamp,
	stale_timestamp + INTERVAL '1' DAY * '%[2]s' AS stale_warning_timestamp,
	stale_timestamp + INTERVAL '1' DAY * '%[3]s' AS culled_timestamp,
	tags,
	system_profile
FROM inventory.%[1]s`

// TODO: make configurable
const cullingStaleWarningOffset = "7"
const cullingCulledOffset = "14"

func (i *ReconcileIteration) checkIfTableExists(tableName string) (bool, error) {
	if tableName == "" {
		return false, nil
	}

	query := fmt.Sprintf(
		"SELECT exists (SELECT FROM information_schema.tables WHERE table_schema = 'inventory' AND table_name = '%s')",
		tableName)
	rows, err := i.runQuery(i.AppDb, query)

	if err != nil {
		return false, err
	}

	var exists bool
	rows.Next()
	err = rows.Scan(&exists)
	if err != nil {
		return false, err
	}
	rows.Close()

	return exists, err
}

func (i *ReconcileIteration) deleteTable(tableName string) error {
	tableExists, err := i.checkIfTableExists(tableName)
	if err != nil {
		return err
	} else if tableExists != true {
		return nil
	}

	query := fmt.Sprintf(
		"DROP table inventory.%s CASCADE", tableName)
	_, err = i.runQuery(i.AppDb, query)
	return err
}

func (i *ReconcileIteration) createTable(tableName string) error {
	m := make(map[string]string)
	m["TableName"] = tableName
	tmpl, err := template.New("dbSchema").Parse(i.DBSchema)
	if err != nil {
		return err
	}

	var dbSchemaBuffer bytes.Buffer
	err = tmpl.Execute(&dbSchemaBuffer, m)
	if err != nil {
		return err
	}
	dbSchemaParsed := dbSchemaBuffer.String()
	_, err = i.AppDb.Exec(dbSchemaParsed)
	return err
}

func (i *ReconcileIteration) updateView() error {
	i.Log.Info("Updating view", "table", i.Instance.Status.TableName)
	_, err := i.AppDb.Exec(fmt.Sprintf(viewTemplate, i.Instance.Status.TableName, cullingStaleWarningOffset, cullingCulledOffset))
	return err
}

func connectToDB(params DBParams) (*pgx.Conn, error) {
	connStr := fmt.Sprintf(
		connectionStringTemplate,
		params.Host,
		params.User,
		params.Password,
		params.Name,
		params.Port)

	if config, err := pgx.ParseDSN(connStr); err == nil {
		return pgx.Connect(config)
	} else {
		return nil, err
	}
}

func (i *ReconcileIteration) closeDB(db *pgx.Conn) {
	if db != nil {
		if err := db.Close(); err != nil {
			i.Log.Error(err, "Failed to close App DB")
		}
	}
}

func (i *ReconcileIteration) runQuery(db *pgx.Conn, query string) (*pgx.Rows, error) {
	rows, err := db.Query(query)

	if err != nil {
		i.Log.Error(err, "Error executing query", "query", query)
		return nil, err
	}

	return rows, nil
}
