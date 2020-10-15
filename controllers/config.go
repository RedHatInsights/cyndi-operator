package controllers

import (
	"cyndi-operator/controllers/config"
	"errors"
	"strconv"
)

/*

Code for loading configuration, secrets, etc.

*/

const inventorySecretName = "host-inventory-db"
const configMapName = "cyndi"

func (i *ReconcileIteration) loadAppDBSecret() error {
	secret, err := fetchSecret(i.Client, i.Instance.Namespace, i.Instance.Spec.AppName+"-db")

	if err != nil {
		return err
	}

	i.AppDBParams, err = config.ParseDBSecret(secret)
	return err
}

func (i *ReconcileIteration) loadHBIDBSecret() error {
	secret, err := fetchSecret(i.Client, i.Instance.Namespace, inventorySecretName)

	if err != nil {
		return err
	}

	i.HBIDBParams, err = config.ParseDBSecret(secret)
	return err
}

func (i *ReconcileIteration) parseConfig() error {
	cyndiConfig, err := fetchConfigMap(i.Client, i.Instance.Namespace, configMapName)

	if err != nil {
		return err
	}

	i.ConnectorConfig = cyndiConfig.Data["connector.config"]
	if i.ConnectorConfig == "" {
		return errors.New("connector.config is missing from cyndi configmap")
	}

	i.DBSchema = cyndiConfig.Data["db.schema"]
	if i.DBSchema == "" {
		return errors.New("db.schema is missing from cyndi configmap")
	}

	i.ConnectCluster = cyndiConfig.Data["connect.cluster"]
	if i.ConnectCluster == "" {
		return errors.New("connect.cluster is missing from cyndi configmap")
	}

	i.ConnectorTasksMax, err = strconv.ParseInt(cyndiConfig.Data["connector.tasks.max"], 10, 64)
	if i.ConnectCluster == "" || err != nil {
		return errors.New("connect.cluster is missing from cyndi configmap or it is malformed")
	}

	if i.Instance.Status.CyndiConfigVersion == "" {
		i.Instance.Status.CyndiConfigVersion = cyndiConfig.ResourceVersion
	} else if i.Instance.Status.CyndiConfigVersion != cyndiConfig.ResourceVersion {
		//cyndi configmap changed, perform a refresh to use latest values
		i.Instance.Status.CyndiConfigVersion = cyndiConfig.ResourceVersion
		if err = i.triggerRefresh(); err != nil {
			return err
		}
	}

	i.ValidationParams = ValidationParams{}

	i.ValidationParams.Interval, err =
		strconv.ParseInt(cyndiConfig.Data["validation.interval"], 10, 64)
	if err != nil || i.ValidationParams.Interval <= 0 {
		return errors.New("unable to parse validation.interval from cyndi configmap")
	}

	i.ValidationParams.AttemptsThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["validation.attempts.threshold"], 10, 64)
	if err != nil || i.ValidationParams.AttemptsThreshold <= 0 {
		return errors.New("unable to parse validation.attempts.threshold from cyndi configmap")
	}

	i.ValidationParams.PercentageThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["validation.percentage.threshold"], 10, 64)
	if err != nil || i.ValidationParams.PercentageThreshold <= 0 {
		return errors.New("unable to parse validation.percentage.threshold from cyndi configmap")
	}

	i.ValidationParams.InitInterval, err =
		strconv.ParseInt(cyndiConfig.Data["init.validation.interval"], 10, 64)
	if err != nil || i.ValidationParams.InitInterval <= 0 {
		return errors.New("unable to parse init.validation.interval from cyndi configmap")
	}

	i.ValidationParams.InitAttemptsThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["init.validation.attempts.threshold"], 10, 64)
	if err != nil || i.ValidationParams.InitAttemptsThreshold <= 0 {
		return errors.New("unable to parse init.validation.attempts.threshold from cyndi configmap")
	}

	i.ValidationParams.InitPercentageThreshold, err =
		strconv.ParseInt(cyndiConfig.Data["init.validation.percentage.threshold"], 10, 64)
	if err != nil || i.ValidationParams.InitPercentageThreshold <= 0 {
		return errors.New("unable to parse init.validation.percentage.threshold from cyndi configmap")
	}

	return nil
}
