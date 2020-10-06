package controllers

import (
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx"
)

const inventoryTableName = "hosts" // TODO: move
const countMismatchThreshold = 0.5

func (i *ReconcileIteration) validate() (bool, error) {
	db, err := connectToDB(i.HBIDBParams)
	if err != nil {
		return false, err
	}

	defer i.closeDB(db)

	hbiHostCount, err := i.countSystems(db, inventoryTableName, false)
	if err != nil {
		return false, err
	}

	appTable := fmt.Sprintf("inventory.%s", i.Instance.Status.TableName)

	appHostCount, err := i.countSystems(i.AppDb, appTable, true)
	if err != nil {
		return false, err
	}

	countMismatchRatio := float64(abs(hbiHostCount-appHostCount) / hbiHostCount)

	log.Info("Fetched host counts", "hbi", hbiHostCount, "app", appHostCount, "countMismatchRatio", countMismatchRatio)

	// if the counts are way off don't even bother comparing ids
	if countMismatchRatio > countMismatchThreshold {
		log.Info("Count mismatch ratio is above threashold, exiting early", "countMismatchRatio", countMismatchRatio)
		return false, nil
	}

	hbiIds, err := i.getHostIds(db, inventoryTableName, false)
	if err != nil {
		return false, err
	}

	appIds, err := i.getHostIds(i.AppDb, appTable, true)
	if err != nil {
		return false, err
	}

	var r DiffReporter

	log.Info("Fetched host ids")
	diff := cmp.Diff(hbiIds, appIds, cmp.Reporter(&r))
	log.Info(diff) // TODO

	validationThresholdPercent := float64(i.ValidationParams.PercentageThreshold)
	if i.Instance.Status.InitialSyncInProgress == true {
		validationThresholdPercent = float64(i.ValidationParams.InitPercentageThreshold)
	}

	idMismatchRatio := float64(len(r.diffs)) / float64(len(hbiIds))

	log.Info("Validation results", "validationThresholdPercent", validationThresholdPercent, "idMismatchRatio", idMismatchRatio)
	return (idMismatchRatio * 100) <= validationThresholdPercent, nil
}

// TODO move to database
func (i *ReconcileIteration) countSystems(db *pgx.Conn, table string, view bool) (int64, error) {

	// TODO: add modified_on filter
	//query := fmt.Sprintf(
	//	"SELECT count(*) FROM %s WHERE modified_on < '%s'", table, i.Now)
	// also add "AND canonical_facts ? 'insights_id'"
	// waiting on https://issues.redhat.com/browse/RHCLOUD-9545
	query := fmt.Sprintf("SELECT count(*) FROM %s", table)
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

// TODO move to database
func (i *ReconcileIteration) getHostIds(db *pgx.Conn, table string, view bool) ([]string, error) {
	// TODO" "AND canonical_facts ? 'insights_id'" when !view and insightsOnly
	query := fmt.Sprintf("SELECT id FROM %s ORDER BY id", table)
	rows, err := db.Query(query)

	var ids []string

	if err != nil {
		return ids, err
	}

	defer rows.Close()

	for rows.Next() {
		var id string
		err = rows.Scan(&id)

		if err != nil {
			return ids, err
		}

		ids = append(ids, id)
	}

	return ids, nil
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

// seriously golang?
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}

	return x
}
