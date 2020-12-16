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

	ConnectCluster                  string
	ConnectorTemplate               string
	ConnectorTasksMax               int64
	ConnectorBatchSize              int64
	ConnectorMaxAge                 int64
	ConnectorAllowlistSystemProfile string

	// the secret for the inventory DB we should connect to when validating
	InventoryDbSecret string

	DBTableInitScript string

	// How often the Reconcile function should run even if there is no event
	StandardInterval int64

	ValidationConfig     ValidationConfiguration
	ValidationConfigInit ValidationConfiguration

	ConfigMapVersion string
}
