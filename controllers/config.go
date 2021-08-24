package controllers

import (
	"fmt"

	"github.com/RedHatInsights/cyndi-operator/controllers/config"
	"github.com/RedHatInsights/cyndi-operator/controllers/utils"

	"k8s.io/apimachinery/pkg/api/errors"
)

/*

Code for loading configuration, secrets, etc.

*/

const configMapName = "cyndi"
const globalConfigNamespace = "cyndi"

func (i *ReconcileIteration) parseConfig() (err error) {
	configMaps := []map[string]string{}

	for _, namespace := range []string{globalConfigNamespace, i.Instance.Namespace} {
		if cyndiConfig, err := utils.FetchConfigMap(i.Client, namespace, configMapName); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else if cyndiConfig != nil {
			configMaps = append(configMaps, (*cyndiConfig).Data)
		}
	}

	i.config, err = config.BuildCyndiConfig(i.Instance, utils.Merge(configMaps...))

	if err != nil {
		return fmt.Errorf("Error parsing %s configmap in %s: %w", configMapName, i.Instance.Namespace, err)
	}

	return err
}
