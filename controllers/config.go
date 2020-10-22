package controllers

import (
	"cyndi-operator/controllers/config"
	"cyndi-operator/controllers/utils"

	"k8s.io/apimachinery/pkg/api/errors"
)

/*

Code for loading configuration, secrets, etc.

*/

const configMapName = "cyndi"

func (i *ReconcileIteration) parseConfig() error {
	cyndiConfig, err := utils.FetchConfigMap(i.Client, i.Instance.Namespace, configMapName)

	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	configMapVersion := cyndiConfig.ResourceVersion
	if errors.IsNotFound(err) {
		configMapVersion = "-1"
	}

	if i.Instance.Status.CyndiConfigVersion == "" {
		i.Instance.Status.CyndiConfigVersion = configMapVersion
	} else if i.Instance.Status.CyndiConfigVersion != configMapVersion {
		//cyndi configmap changed, perform a refresh to use latest values
		i.Instance.Status.CyndiConfigVersion = configMapVersion
		if err = i.triggerRefresh(); err != nil {
			return err
		}
	}

	i.config, err = config.BuildCyndiConfig(nil, cyndiConfig)
	return err
}
