package utils

import "fmt"

const inventorySchema = "inventory"

func AppFullTableName(tableName string) string {
	return fmt.Sprintf("%s.%s", inventorySchema, tableName)
}

func AppDbSecretName(appName string) string {
	return fmt.Sprintf("%s-db", appName)
}
