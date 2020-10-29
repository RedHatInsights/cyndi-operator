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

	if err != nil {
		if errors.IsNotFound(err) {
			cyndiConfig = nil
		} else {
			return err
		}
	}

	i.config, err = config.BuildCyndiConfig(i.Instance, cyndiConfig)
	return err
}
