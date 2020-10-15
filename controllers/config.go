package controllers

import (
	"cyndi-operator/controllers/config"

	"k8s.io/apimachinery/pkg/api/errors"
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

	if err != nil && !errors.IsNotFound(err) {
		return err
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

	i.config, err = config.BuildCyndiConfig(nil, cyndiConfig)
	return err
}
