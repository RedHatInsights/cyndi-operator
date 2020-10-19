package database

import (
	"bytes"
	"fmt"
	"text/template"
)

type AppDatabase struct {
	Database
}

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

const cullingStaleWarningOffset = "7"
const cullingCulledOffset = "14"

func (db *AppDatabase) CheckIfTableExists(tableName string) (bool, error) {
	if tableName == "" {
		return false, nil
	}

	query := fmt.Sprintf(
		"SELECT exists (SELECT FROM information_schema.tables WHERE table_schema = 'inventory' AND table_name = '%s')",
		tableName)
	rows, err := db.runQuery(query)

	if err != nil {
		return false, err
	}

	var exists bool
	rows.Next()
	err = rows.Scan(&exists)
	if err != nil {
		return false, err
	}

	if rows != nil {
		rows.Close()
	}

	return exists, err
}

func (db *AppDatabase) CreateTable(tableName string, script string) error {
	m := make(map[string]string)
	m["TableName"] = tableName
	tmpl, err := template.New("dbSchema").Parse(script)
	if err != nil {
		return err
	}

	var dbSchemaBuffer bytes.Buffer
	err = tmpl.Execute(&dbSchemaBuffer, m)
	if err != nil {
		return err
	}

	dbSchemaParsed := dbSchemaBuffer.String()
	_, err = db.connection.Exec(dbSchemaParsed)
	return err
}

func (db *AppDatabase) DeleteTable(tableName string) error {
	tableExists, err := db.CheckIfTableExists(tableName)
	if err != nil {
		return err
	} else if tableExists != true {
		return nil
	}

	query := fmt.Sprintf("DROP table inventory.%s CASCADE", tableName)
	_, err = db.connection.Exec(query)
	return err
}

func (db *AppDatabase) UpdateView(tableName string) error {
	_, err := db.connection.Exec(fmt.Sprintf(viewTemplate, tableName, cullingStaleWarningOffset, cullingCulledOffset))
	return err
}
