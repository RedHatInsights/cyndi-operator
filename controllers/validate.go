package controllers

import (
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx"
	"strings"
	"time"
)

type host struct {
	ID          string
	Account     string
	DisplayName string
	Tags        string
}

func getSystemsFromAppDB(instance *cyndiv1beta1.CyndiPipeline, db *pgx.Conn, now string) ([]host, error) {
	insightsOnlyQuery := ""
	if instance.Spec.InsightsOnly == true {
		insightsOnlyQuery = "AND canonical_facts ? 'insights_id'"
	}

	query := fmt.Sprintf("SELECT id, account, display_name, tags FROM inventory.%s WHERE updated < '%s' %s ORDER BY id LIMIT 10 OFFSET 0", instance.Status.TableName, now, insightsOnlyQuery)
	hosts, err := getSystemsFromDB(db, query)
	return hosts, err
}

func getSystemsFromHBIDB(instance *cyndiv1beta1.CyndiPipeline, now string) ([]host, error) {
	db, err := connectToInventoryDB(instance)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT id, account, display_name, tags FROM hosts WHERE modified_on < '%s' ORDER BY id LIMIT 10 OFFSET 0", now)
	hosts, err := getSystemsFromDB(db, query)
	return hosts, err
}

func getSystemsFromDB(db *pgx.Conn, query string) ([]host, error) {
	hosts, err := db.Query(query)
	var hostsParsed []host

	if err != nil {
		return hostsParsed, err
	}

	defer hosts.Close()

	for hosts.Next() {
		var (
			id          uuid.UUID
			account     string
			displayName string
			tags        string
		)
		err = hosts.Scan(&id, &account, &displayName, &tags)
		if err != nil {
			return hostsParsed, err
		}

		hostsParsed = append(hostsParsed, host{ID: fmt.Sprintf("%x", id), Account: account, DisplayName: displayName, Tags: tags})
	}

	return hostsParsed, nil
}

func validate(instance *cyndiv1beta1.CyndiPipeline, appDb *pgx.Conn) (bool, error) {
	now := time.Now().Format(time.RFC3339)
	hbiHosts, err := getSystemsFromHBIDB(instance, now)
	if err != nil {
		return false, err
	}
	appHosts, err := getSystemsFromAppDB(instance, appDb, now)
	if err != nil {
		return false, err
	}

	diff := cmp.Diff(hbiHosts, appHosts)
	diff = strings.ReplaceAll(diff, "\n", "")
	diff = strings.ReplaceAll(diff, "\t", "")
	log.Info(diff)

	return diff == "", err
}
