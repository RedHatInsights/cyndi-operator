module cyndi-operator

go 1.13

require (
	github.com/go-logr/logr v0.2.1
	github.com/go-logr/zapr v0.2.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/google/go-cmp v0.4.0
	github.com/google/gops v0.3.10 // indirect
	github.com/jackc/pgx v3.6.2+incompatible
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/prometheus/client_golang v1.0.0
	github.com/shirou/gopsutil v2.20.6+incompatible // indirect
	github.com/spf13/viper v1.3.2
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/sys v0.0.0-20200724161237-0e2f3a69832c // indirect
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/client-go v0.19.3
	sigs.k8s.io/controller-runtime v0.6.0
)
