package controllers

import (
	"bytes"
	"fmt"
	"github.com/jackc/pgx"
	"strconv"
	"text/template"
)

func (i *ReconcileIteration) checkIfTableExists(tableName string) (bool, error) {
	if tableName == "" {
		return false, nil
	}

	query := fmt.Sprintf(
		"SELECT exists (SELECT FROM information_schema.tables WHERE table_schema = 'inventory' AND table_name = '%s')",
		tableName)
	rows, err := i.AppDb.Query(query)

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
	_, err = i.AppDb.Query(query)
	return err
}

func (i *ReconcileIteration) createTable(tableName string, dbSchema string) error {
	m := make(map[string]string)
	m["TableName"] = tableName
	tmpl, err := template.New("dbSchema").Parse(dbSchema)
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
	_, err := i.AppDb.Exec(fmt.Sprintf(`CREATE OR REPLACE view inventory.hosts as select * from inventory.%s`, i.Instance.Status.TableName))
	return err
}

func (i *ReconcileIteration) connectToInventoryDB() (*pgx.Conn, error) {
	connStr := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s sslmode=%s port=%s",
		i.Instance.Spec.InventoryDBHostname,
		i.Instance.Spec.InventoryDBUser,
		i.Instance.Spec.InventoryDBPassword,
		i.Instance.Spec.InventoryDBName,
		i.Instance.Spec.InventoryDBSSLMode,
		strconv.FormatInt(i.Instance.Spec.InventoryDBPort, 10))
	config, err := pgx.ParseDSN(connStr)
	db, err := pgx.Connect(config)
	return db, err
}

func (i *ReconcileIteration) connectToAppDB() error {
	connStr := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s sslmode=%s port=%s",
		i.Instance.Spec.AppDBHostname,
		i.Instance.Spec.AppDBUser,
		i.Instance.Spec.AppDBPassword,
		i.Instance.Spec.AppDBName,
		i.Instance.Spec.AppDBSSLMode,
		strconv.FormatInt(i.Instance.Spec.AppDBPort, 10))
	config, err := pgx.ParseDSN(connStr)
	db, err := pgx.Connect(config)
	i.AppDb = db
	return err
}

func (i *ReconcileIteration) closeAppDB() {
	err := i.AppDb.Close()
	if err != nil {
		i.Log.Error(err, "Failed to close App DB")
	}
}
