package config

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func ParseDBSecret(secret *corev1.Secret) (DBParams, error) {
	var dbParams = DBParams{}
	var err error

	dbParams.Host, err = readSecretValue(secret, "db.host")
	if err != nil {
		return dbParams, err
	}

	dbParams.User, err = readSecretValue(secret, "db.user")
	if err != nil {
		return dbParams, err
	}

	dbParams.Password, err = readSecretValue(secret, "db.password")
	if err != nil {
		return dbParams, err
	}

	dbParams.Name, err = readSecretValue(secret, "db.name")
	if err != nil {
		return dbParams, err
	}

	dbParams.Port, err = readSecretValue(secret, "db.port")
	if err != nil {
		return dbParams, err
	}

	return dbParams, nil
}

func readSecretValue(secret *corev1.Secret, key string) (string, error) {
	value := secret.Data[key]
	if value == nil || string(value) == "" {
		return "", fmt.Errorf("%s missing from %s secret", key, secret.ObjectMeta.Name)
	}

	return string(value), nil
}
