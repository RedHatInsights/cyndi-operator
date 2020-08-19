package controllers

import (
	"encoding/json"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx"
	"strings"
)

type tag struct {
	Namespace string
	Key       string
	Value     string
}

type host struct {
	ID          string
	Account     string
	DisplayName string
	Tags        string
}

func (i *ReconcileIteration) validate() (bool, error) {
	hbiHosts, err := i.getSystemsFromHBIDB()
	if err != nil {
		return false, err
	}
	appHosts, err := i.getSystemsFromAppDB()
	if err != nil {
		return false, err
	}

	diff := cmp.Diff(hbiHosts, appHosts)
	diff = strings.ReplaceAll(diff, "\n", "")
	diff = strings.ReplaceAll(diff, "\t", "")
	log.Info(diff)

	isValid := diff == ""
	if isValid == false {
		i.Instance.Status.ValidationFailedCount++
	} else {
		i.Instance.Status.ValidationFailedCount = 0
	}

	i.Instance.Status.SyndicatedDataIsValid = isValid
	return isValid, err
}

func (i *ReconcileIteration) getSystemsFromHBIDB() ([]host, error) {
	db, err := i.connectToInventoryDB()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(
		"SELECT id, account, display_name, tags FROM hosts WHERE modified_on < '%s' ORDER BY id LIMIT 10 OFFSET 0", i.Now)
	hosts, err := getSystemsFromDB(db, query, false)
	if err != nil {
		return hosts, err
	}

	err = db.Close()
	return hosts, err
}

func (i *ReconcileIteration) getSystemsFromAppDB() ([]host, error) {
	insightsOnlyQuery := ""
	if i.Instance.Spec.InsightsOnly == true {
		insightsOnlyQuery = "AND canonical_facts ? 'insights_id'"
	}

	query := fmt.Sprintf(
		"SELECT id, account, display_name, tags FROM inventory.%s WHERE updated < '%s' %s ORDER BY id LIMIT 10 OFFSET 0",
		i.Instance.Status.TableName, i.Now, insightsOnlyQuery)
	hosts, err := getSystemsFromDB(i.AppDb, query, true)
	return hosts, err
}

func getSystemsFromDB(db *pgx.Conn, query string, appDB bool) ([]host, error) {
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

		if appDB == true {
			err = parseTags(&tags)
		} else {
			err = flattenTags(&tags)
		}

		if err != nil {
			return hostsParsed, err
		}

		hostsParsed = append(
			hostsParsed,
			host{ID: fmt.Sprintf("%x", id), Account: account, DisplayName: displayName, Tags: tags})
	}

	return hostsParsed, nil
}

func parseTags(tags *string) error {
	var tagsJson []tag
	err := json.Unmarshal([]byte(*tags), &tagsJson)
	if err != nil {
		return err
	}
	tagsMarshalled, err := json.Marshal(tagsJson)
	*tags = string(tagsMarshalled)
	return err
}

func flattenTags(tags *string) error {
	var tagsFlat []tag
	tagsMap := make(map[string]interface{})
	err := json.Unmarshal([]byte(*tags), &tagsMap)
	if err != nil {
		return err
	}

	for namespace, keyValues := range tagsMap {
		keyValuesMap := keyValues.(map[string]interface{})

		for key, values := range keyValuesMap {
			valuesArray := values.([]interface{})
			if values == nil || len(valuesArray) == 0 {
				tagsFlat = append(tagsFlat, tag{Namespace: namespace, Key: "", Value: ""})
			} else {
				for _, value := range valuesArray {
					tagsFlat = append(tagsFlat, tag{Namespace: namespace, Key: key, Value: value.(string)})
				}
			}
		}
	}

	tagsMarshalled, err := json.Marshal(tagsFlat)
	*tags = string(tagsMarshalled)
	return err
}
