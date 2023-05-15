package utils

import (
	"bytes"
	"fmt"
	"text/template"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
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

func AppSubstituteTable(script string, tableName string) (string, error) {
	m := make(map[string]string)
	m["TableName"] = tableName
	tmpl, err := template.New("query").Parse(script)
	if err != nil {
		return "", err
	}

	var queryBuffer bytes.Buffer
	err = tmpl.Execute(&queryBuffer, m)
	if err != nil {
		return "", err
	}

	return queryBuffer.String(), nil
}

func AppDbIndexesDiffer(indexDefs []string, indexSpecs []string, tableName string) (bool, error) {
	if len(indexDefs) != len(indexSpecs) {
		return true, nil
	}
	diff := make(map[string]int, len(indexDefs))
	for _, _x := range indexDefs {
		diff[_x]++
	}
	for _, _y := range indexSpecs {
		_y, err := AppSubstituteTable(_y, tableName)
		if err != nil {
			return false, err
		}

		if _, ok := diff[_y]; !ok {
			return true, nil
		}
		diff[_y] -= 1
		if diff[_y] == 0 {
			delete(diff, _y)
		}
	}
	return len(diff) != 0, nil
}
