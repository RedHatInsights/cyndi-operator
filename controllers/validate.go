package controllers

import (
	"cyndi-operator/controllers/probes"
	"cyndi-operator/controllers/utils"
	"math"
)

const inventoryTableName = "public.hosts" // TODO: move
const countMismatchThreshold = 0.5
const idDiffMaxLength = 50

func (i *ReconcileIteration) validate() (isValid bool, mismatchRatio float64, mismatchCount int64, hostCount int64, err error) {
	hbiHostCount, err := i.InventoryDb.CountHosts(inventoryTableName, i.Instance.Spec.InsightsOnly)
	if err != nil {
		return false, -1, -1, -1, err
	}

	appTable := utils.AppFullTableName(i.Instance.Status.TableName)

	appHostCount, err := i.AppDb.CountHosts(appTable, false)
	if err != nil {
		return false, -1, -1, -1, err
	}

	probes.AppHostCount(i.Instance, appHostCount)

	countMismatch := utils.Abs(hbiHostCount - appHostCount)
	countMismatchRatio := float64(countMismatch) / math.Max(float64(hbiHostCount), 1)

	i.Log.Info("Fetched host counts", "hbi", hbiHostCount, "app", appHostCount, "countMismatchRatio", countMismatchRatio)

	// if the counts are way off don't even bother comparing ids
	if countMismatchRatio > countMismatchThreshold {
		i.Log.Info("Count mismatch ratio is above threashold, exiting early", "countMismatchRatio", countMismatchRatio)
		probes.ValidationFinished(i.Instance, i.getValidationConfig().PercentageThreshold, countMismatchRatio, false)
		return false, countMismatchRatio, countMismatch, appHostCount, nil
	}

	hbiIds, err := i.InventoryDb.GetHostIds(inventoryTableName, i.Instance.Spec.InsightsOnly)
	if err != nil {
		return false, -1, -1, -1, err
	}

	appIds, err := i.AppDb.GetHostIds(appTable, false)
	if err != nil {
		return false, -1, -1, -1, err
	}

	i.Log.Info("Fetched host ids")
	inHbiOnly := utils.Difference(hbiIds, appIds)
	inAppOnly := utils.Difference(appIds, hbiIds)
	mismatchCount = int64(len(inHbiOnly) + len(inAppOnly))

	validationThresholdPercent := i.getValidationConfig().PercentageThreshold

	idMismatchRatio := float64(mismatchCount) / math.Max(float64(len(hbiIds)), 1)
	result := (idMismatchRatio * 100) <= float64(validationThresholdPercent)

	probes.ValidationFinished(i.Instance, i.getValidationConfig().PercentageThreshold, idMismatchRatio, result)
	i.Log.Info(
		"Validation results",
		"validationThresholdPercent", validationThresholdPercent,
		"idMismatchRatio", idMismatchRatio,
		// if the list is too long truncate it to first 50 ids to avoid log polution
		"inHbiOnly", inHbiOnly[:utils.Min(idDiffMaxLength, len(inHbiOnly))],
		"inAppOnly", inAppOnly[:utils.Min(idDiffMaxLength, len(inAppOnly))],
	)
	return result, idMismatchRatio, mismatchCount, appHostCount, nil
}
