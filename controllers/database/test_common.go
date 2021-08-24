package database

import (
	"fmt"
	"time"

	. "github.com/RedHatInsights/cyndi-operator/controllers/config"

	"github.com/spf13/viper"
)

/*
 * Common utils used in database tests.
 */

var (
	TestTable string
)

func getDBParams() *DBParams {
	options := viper.New()
	options.SetDefault("DBHostHBI", "localhost")
	options.SetDefault("DBPort", "5432")
	options.SetDefault("DBUser", "postgres")
	options.SetDefault("DBPass", "postgres")
	options.SetDefault("DBName", "test")
	options.AutomaticEnv()

	return &DBParams{
		Host:     options.GetString("DBHostHBI"),
		Port:     options.GetString("DBPort"),
		User:     options.GetString("DBUser"),
		Password: options.GetString("DBPass"),
		Name:     options.GetString("DBName"),
	}
}

func uniqueTable() {
	TestTable = fmt.Sprintf("test_%d", time.Now().UnixNano())
}
