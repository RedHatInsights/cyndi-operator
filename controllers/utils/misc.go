package utils

import "fmt"

const inventorySchema = "inventory"

func AppFullTableName(tableName string) string {
	return fmt.Sprintf("%s.%s", inventorySchema, tableName)
}
