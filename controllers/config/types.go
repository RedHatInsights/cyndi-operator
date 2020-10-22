package config

type DBParams struct {
	Name     string
	Host     string
	Port     string
	User     string
	Password string
}

type ValidationConfiguration struct {
	Interval            int64
	AttemptsThreshold   int64
	PercentageThreshold int64
}

type CyndiConfiguration struct {
	Topic string

	ConnectCluster     string // TODO should go to CRD
	ConnectorTemplate  string
	ConnectorTasksMax  int64
	ConnectorBatchSize int64
	ConnectorMaxAge    int64 // TODO should go to CRD

	DBTableInitScript string

	ValidationConfig     ValidationConfiguration
	ValidationConfigInit ValidationConfiguration

	ConfigMapVersion string
}
