package database

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/RedHatInsights/cyndi-operator/controllers/config"
	"github.com/RedHatInsights/cyndi-operator/controllers/utils"
)

type AppDatabase struct {
	BaseDatabase
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
	system_profile,
	insights_id,
	reporter,
	per_reporter_staleness,
	org_id
FROM inventory.%[1]s`

const cullingStaleWarningOffset = "7"
const cullingCulledOffset = "14"

const initialPassword = "havefun"

func NewAppDatabase(config *config.DBParams) *AppDatabase {
	return &AppDatabase{
		BaseDatabase: BaseDatabase{
			Config: config,
		},
	}
}

func (db *AppDatabase) Connect() (err error) {
	err = db.BaseDatabase.Connect()

	// auth error may indicate the database user is still using the initial password
	// let's try rotating it
	if err != nil && strings.Contains(err.Error(), "28P01") {
		if err = db.rotatePassword(); err != nil {
			return
		}

		err = db.BaseDatabase.Connect()
	}

	return err
}

func (db *AppDatabase) CheckIfTableExists(tableName string) (bool, error) {
	if tableName == "" {
		return false, nil
	}

	query := fmt.Sprintf(
		"SELECT exists (SELECT FROM information_schema.tables WHERE table_schema = 'inventory' AND table_name = '%s')",
		tableName)
	rows, err := db.RunQuery(query)

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
	_, err = db.Exec(dbSchemaParsed)
	return err
}

func (db *AppDatabase) DeleteTable(tableName string) error {
	tableExists, err := db.CheckIfTableExists(tableName)
	if err != nil {
		return err
	} else if !tableExists {
		return nil
	}

	query := fmt.Sprintf("DROP table %s CASCADE", utils.AppFullTableName(tableName))
	_, err = db.Exec(query)
	return err
}

func (db *AppDatabase) UpdateView(tableName string) error {
	if _, err := db.Exec(fmt.Sprintf(viewTemplate, tableName, cullingStaleWarningOffset, cullingCulledOffset)); err != nil {
		return err
	}

	if _, err := db.Exec(`GRANT SELECT ON inventory.hosts TO cyndi_reader`); err != nil {
		return err
	}

	return nil
}

func (db *AppDatabase) GetCurrentTable() (table *string, err error) {
	query := "SELECT table_name FROM information_schema.view_table_usage WHERE view_schema = 'inventory' AND view_name = 'hosts' LIMIT 1;"
	rows, err := db.RunQuery(query)

	if err != nil {
		return nil, err
	}

	if !rows.Next() {
		return nil, nil
	}

	err = rows.Scan(&table)
	if rows != nil {
		rows.Close()
	}

	return table, err
}

func (db *AppDatabase) GetCyndiTables() (tables []string, err error) {
	query := "SELECT table_name FROM information_schema.tables WHERE table_schema = 'inventory' AND table_type = 'BASE TABLE' AND table_name LIKE 'hosts_%' ORDER BY table_name"
	rows, err := db.RunQuery(query)

	if err != nil {
		return tables, err
	}

	defer rows.Close()

	for rows.Next() {
		var table string
		err = rows.Scan(&table)

		if err != nil {
			return tables, err
		}

		tables = append(tables, table)
	}

	return tables, nil
}

func (db *AppDatabase) rotatePassword() (err error) {
	newPassword := db.Config.Password
	db.Config.Password = initialPassword

	if err = db.BaseDatabase.Connect(); err != nil {
		return
	}

	defer db.BaseDatabase.Close()

	query := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'", db.Config.User, newPassword)
	_, err = db.Exec(query)
	return
}
