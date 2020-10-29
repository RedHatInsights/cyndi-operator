package controllers

import (
	"cyndi-operator/controllers/utils"
	"fmt"

	"github.com/google/go-cmp/cmp"
)

const inventoryTableName = "hosts" // TODO: move
const countMismatchThreshold = 0.5

func (i *ReconcileIteration) validate() (isValid bool, mismatchRatio float64, mismatchCount int64, hostCount int64, err error) {
	hbiHostCount, err := i.InventoryDb.CountHosts(inventoryTableName)
	if err != nil {
		return false, -1, -1, -1, err
	}

	appTable := fmt.Sprintf("inventory.%s", i.Instance.Status.TableName)

	appHostCount, err := i.AppDb.CountHosts(appTable)
	if err != nil {
		return false, -1, -1, -1, err
	}

	countMismatch := utils.Abs(hbiHostCount - appHostCount)
	countMismatchRatio := float64(countMismatch / hbiHostCount)

	i.Log.Info("Fetched host counts", "hbi", hbiHostCount, "app", appHostCount, "countMismatchRatio", countMismatchRatio)

	// if the counts are way off don't even bother comparing ids
	if countMismatchRatio > countMismatchThreshold {
		i.Log.Info("Count mismatch ratio is above threashold, exiting early", "countMismatchRatio", countMismatchRatio)
		return false, countMismatchRatio, countMismatch, appHostCount, nil
	}

	hbiIds, err := i.InventoryDb.GetHostIds(inventoryTableName)
	if err != nil {
		return false, -1, -1, -1, err
	}

	appIds, err := i.AppDb.GetHostIds(appTable)
	if err != nil {
		return false, -1, -1, -1, err
	}

	var r DiffReporter

	i.Log.Info("Fetched host ids")
	diff := cmp.Diff(hbiIds, appIds, cmp.Reporter(&r))
	i.Log.Info(diff) // TODO

	validationThresholdPercent := i.getValidationConfig().PercentageThreshold

	idMismatchRatio := float64(len(r.diffs)) / float64(len(hbiIds))

	i.Log.Info("Validation results", "validationThresholdPercent", validationThresholdPercent, "idMismatchRatio", idMismatchRatio)
	return (idMismatchRatio * 100) <= float64(validationThresholdPercent), idMismatchRatio, int64(len(r.diffs)), appHostCount, nil
}
