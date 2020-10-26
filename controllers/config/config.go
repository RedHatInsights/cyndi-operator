package config

import (
	cyndiv1beta1 "cyndi-operator/api/v1beta1"
	"cyndi-operator/controllers/utils"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildCyndiConfig(instance *cyndiv1beta1.CyndiPipeline, cm *corev1.ConfigMap) (*CyndiConfiguration, error) {
	var err error
	config := &CyndiConfiguration{}

	config.Topic = getStringValue(cm, "connector.topic", defaultTopic)
	config.ConnectCluster = getStringValue(cm, "connect.cluster", defaultConnectCluster)
	config.ConnectorTemplate = getStringValue(cm, "connector.config", defaultConnectorTemplate)

	if config.ConnectorTasksMax, err = getIntValue(cm, "connector.tasks.max", defaultConnectorTasksMax); err != nil {
		return config, err
	}

	if config.ConnectorBatchSize, err = getIntValue(cm, "connector.batch.size", defaultConnectorBatchSize); err != nil {
		return config, err
	}

	if config.ConnectorMaxAge, err = getIntValue(cm, "connector.max.age", defaultConnectorMaxAge); err != nil {
		return config, err
	}

	config.DBTableInitScript = getStringValue(cm, "db.schema", defaultDBTableInitScript)

	if config.StandardInterval, err = getIntValue(cm, "standard.interval", defaultStandardInterval); err != nil {
		return config, err
	}

	if config.ValidationConfig, err = getValidationConfig(cm, "", defaultValidationConfig); err != nil {
		return config, err
	}

	if config.ValidationConfigInit, err = getValidationConfig(cm, "init.", defaultValidationConfigInit); err != nil {
		return config, err
	}

	if cm == nil {
		config.ConfigMapVersion = "-1"
	} else {
		config.ConfigMapVersion = cm.ResourceVersion
	}

	return config, err
}

func getStringValue(cm *corev1.ConfigMap, key string, defaultValue string) string {
	if cm == nil {
		return defaultValue
	}

	if value, ok := cm.Data[key]; ok {
		return value
	}

	return defaultValue
}

func getIntValue(cm *corev1.ConfigMap, key string, defaultValue int64) (int64, error) {
	if cm == nil {
		return defaultValue, nil
	}

	if value, ok := cm.Data[key]; ok {
		if parsed, err := strconv.ParseInt(value, 10, 64); err != nil {
			return -1, fmt.Errorf("%s is not a valid value for %s", value, key)
		} else {
			return parsed, nil
		}
	}

	return defaultValue, nil
}

func getValidationConfig(cm *corev1.ConfigMap, prefix string, defaultValue ValidationConfiguration) (ValidationConfiguration, error) {
	var (
		err    error
		result = ValidationConfiguration{}
	)

	if result.Interval, err = getIntValue(cm, fmt.Sprintf("%svalidation.interval", prefix), defaultValue.Interval); err != nil {
		return result, err
	}

	if result.AttemptsThreshold, err = getIntValue(cm, fmt.Sprintf("%svalidation.attempts.threshold", prefix), defaultValue.AttemptsThreshold); err != nil {
		return result, err
	}

	if result.PercentageThreshold, err = getIntValue(cm, fmt.Sprintf("%svalidation.percentage.threshold", prefix), defaultValue.PercentageThreshold); err != nil {
		return result, err
	}

	return result, err
}

func LoadSecret(c client.Client, namespace string, name string) (DBParams, error) {
	secret, err := utils.FetchSecret(c, namespace, name)

	if err != nil {
		return DBParams{}, err
	}

	params, err := ParseDBSecret(secret)
	return params, err
}
