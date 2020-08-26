package controllers

import (
	"encoding/json"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx"
	"sort"
	"strings"
)

type tag struct {
	Namespace string
	Key       string
	Value     string
}

type sortedTag []tag

func (a sortedTag) Len() int { return len(a) }
func (a sortedTag) Less(i, j int) bool {
	if a[i].Namespace == a[j].Namespace {
		if a[i].Key == a[j].Key {
			return a[i].Value < a[j].Value
		} else {
			return a[i].Key < a[j].Key
		}
	} else {
		return a[i].Namespace < a[j].Namespace
	}
}
func (a sortedTag) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type host struct {
	ID          string
	Account     string
	DisplayName string
	Tags        string
}

func (i *ReconcileIteration) validate() error {
	hbiHosts, err := i.getSystemsFromHBIDB()
	if err != nil {
		return err
	}
	appHosts, err := i.getSystemsFromAppDB()
	if err != nil {
		return err
	}
	totalHosts, err := i.countSystemsInHBIDB()
	if err != nil {
		return err
	}

	var r DiffReporter
	diff := cmp.Diff(hbiHosts, appHosts, cmp.Reporter(&r))
	log.Info(diff)

	percentageThreshold := float64(i.ValidationParams.PercentageThreshold)
	if i.Instance.Status.InitialSyncInProgress == true {
		percentageThreshold = float64(i.ValidationParams.InitPercentageThreshold)
	}
	percentageThreshold = percentageThreshold / 100

	isValid := diff == ""
	percentageValid := 1 - float64(len(r.diffs))/float64(totalHosts)
	if isValid == false && percentageValid < percentageThreshold {
		i.Instance.Status.ValidationFailedCount++
	} else {
		i.Instance.Status.ValidationFailedCount = 0
	}

	i.Instance.Status.SyndicatedDataIsValid = isValid
	return err
}

func (i *ReconcileIteration) countSystemsInHBIDB() (int64, error) {
	db, err := i.connectToInventoryDB()
	if err != nil {
		return -1, err
	}
	query := fmt.Sprintf(
		"SELECT count(*) FROM hosts WHERE modified_on < '%s'", i.Now)
	rows, err := db.Query(query)

	defer rows.Close()

	var response int64
	for rows.Next() {
		var count int64
		err = rows.Scan(&count)
		if err != nil {
			return -1, err
		}
		response = count
	}

	if err != nil {
		return -1, err
	}

	return response, err
}

func (i *ReconcileIteration) getSystemsFromHBIDB() ([]host, error) {
	db, err := i.connectToInventoryDB()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(
		"SELECT id, account, display_name, tags FROM hosts WHERE modified_on < '%s' ORDER BY id", i.Now)
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
	sort.Sort(sortedTag(tagsJson))
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

	sort.Sort(sortedTag(tagsFlat))
	tagsMarshalled, err := json.Marshal(tagsFlat)
	*tags = string(tagsMarshalled)
	return err
}

type DiffReporter struct {
	path  cmp.Path
	diffs []string
}

func (r *DiffReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *DiffReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		r.diffs = append(r.diffs, fmt.Sprintf("%#v:\n\t-: %+v\n\t+: %+v\n", r.path, vx, vy))
	}
}

func (r *DiffReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *DiffReporter) String() string {
	return strings.Join(r.diffs, "\n")
}
