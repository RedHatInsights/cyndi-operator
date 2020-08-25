package controllers

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/jackc/pgx"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	_, err := i.AppDb.Exec(fmt.Sprintf(`CREATE OR REPLACE view inventory.hosts as select * from inventory.%s`, i.Instance.Status.TableName))
	return err
}

func readHBISecretValue(hbiSecret *corev1.Secret, key string) (string, error) {
	value := hbiSecret.Data[key]
	if value == nil || string(value) == "" {
		errorMsg := fmt.Sprintf("%s missing from host-inventory-db secret", key)
		return "", errors.New(errorMsg)
	} else {
		return string(value), nil
	}
}

func (i *ReconcileIteration) parseHBIDBSecret() error {
	hbiSecret := &corev1.Secret{}
	err := i.Client.Get(context.TODO(), client.ObjectKey{Name: "host-inventory-db", Namespace: i.Instance.Namespace}, hbiSecret)

	host, err := readHBISecretValue(hbiSecret, "db.host")
	if err != nil {
		return err
	}

	user, err := readHBISecretValue(hbiSecret, "db.user")
	if err != nil {
		return err
	}

	password, err := readHBISecretValue(hbiSecret, "db.password")
	if err != nil {
		return err
	}

	name, err := readHBISecretValue(hbiSecret, "db.name")
	if err != nil {
		return err
	}

	port, err := readHBISecretValue(hbiSecret, "db.port")
	if err != nil {
		return err
	}

	i.HBIDBParams = HBIDBParams{
		Host:     host,
		User:     user,
		Password: password,
		Name:     name,
		Port:     port}

	return nil
}

func (i *ReconcileIteration) connectToInventoryDB() (*pgx.Conn, error) {
	connStr := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s",
		i.HBIDBParams.Host,
		i.HBIDBParams.User,
		i.HBIDBParams.Password,
		i.HBIDBParams.Name,
		i.HBIDBParams.Port)
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
