module cyndi-operator

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/google/go-cmp v0.3.0
	github.com/google/gops v0.3.10 // indirect
	github.com/jackc/pgx v3.6.2+incompatible
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/shirou/gopsutil v2.20.6+incompatible // indirect
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	golang.org/x/sys v0.0.0-20200724161237-0e2f3a69832c // indirect
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	sigs.k8s.io/controller-runtime v0.6.0
)
