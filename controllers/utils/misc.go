package utils

import (
	cyndi "cyndi-operator/api/v1alpha1"
	"fmt"
)

const inventorySchema = "inventory"

func AppFullTableName(tableName string) string {
	return fmt.Sprintf("%s.%s", inventorySchema, tableName)
}

func AppDefaultDbSecretName(appName string) string {
	return fmt.Sprintf("%s-db", appName)
}

func AppDbSecretName(spec cyndi.CyndiPipelineSpec) string {
	if spec.DbSecret != nil {
		return *spec.DbSecret
	}

	return AppDefaultDbSecretName(spec.AppName)
}
