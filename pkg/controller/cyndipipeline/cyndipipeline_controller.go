package cyndipipeline

import (
	"context"
	cyndiv1beta1 "cyndi-operator/pkg/apis/cyndi/v1beta1"
	pgx "github.com/jackc/pgx"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_cyndipipeline")

// Add creates a new CyndiPipeline Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCyndiPipeline{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cyndipipeline-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CyndiPipeline
	err = c.Watch(&source.Kind{Type: &cyndiv1beta1.CyndiPipeline{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner CyndiPipeline
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cyndiv1beta1.CyndiPipeline{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileCyndiPipeline implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileCyndiPipeline{}

// ReconcileCyndiPipeline reconciles a CyndiPipeline object
type ReconcileCyndiPipeline struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile test
func (r *ReconcileCyndiPipeline) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CyndiPipeline")

	instance := &cyndiv1beta1.CyndiPipeline{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	reqLogger.Info("Setting up database")
	db, err := connectToDB()
	if err != nil {
		return reconcile.Result{}, err
	}

	err = createSchema(db)
	if err != nil {
		return reconcile.Result{}, err
	}

	exists, err := checkIfTableExists(db)
	if err != nil {
		return reconcile.Result{}, err
	}

	if exists != true {
		reqLogger.Info("Creating table")
		err = createTable(db)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		reqLogger.Info("Table exists")
	}

	err = updateView(db)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = createConnector(instance, r)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Reconcile complete, don't requeue", "Pod.Namespace", "default", "Pod.Name", "my-source-connector")
	return reconcile.Result{}, nil
}

func newConnectorForCR(cr *cyndiv1beta1.CyndiPipeline) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "my-source-connector",
			"namespace": "default",
			"labels": map[string]interface{}{
				"strimzi.io/cluster": "my-connector-cluster",
			},
		},
		"spec": map[string]interface{}{
			"tasksMax": 2,
			"config": map[string]interface{}{
				"file":  "/opt/kafka/LICENSE",
				"topic": "my-topic",
			},
			"class": "org.apache.kafka.connect.file.FileStreamSourceConnector",
		},
	}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "KafkaConnector",
		Version: "kafka.strimzi.io/v1alpha1",
	})
	return u
}

func createSchema(db *pgx.Conn) error {
	rows, err := db.Query(`CREATE SCHEMA IF NOT EXISTS inventory`)
	rows.Close()
	return err
}

func connectToDB() (*pgx.Conn, error) {
	connStr := "host=inventory-db user=insights password=insights dbname=insights sslmode=disable"
	config, err := pgx.ParseDSN(connStr)
	db, err := pgx.Connect(config)
	return db, err
}

func checkIfTableExists(db *pgx.Conn) (bool, error) {
	rows, err := db.Query(
		`SELECT exists
            (SELECT FROM information_schema.tables
            WHERE table_schema = 'inventory'
            AND table_name = 'hosts_v1_0')`)

	var exists bool
	rows.Next()
	err = rows.Scan(&exists)
	if err != nil {
		return false, err
	}
	rows.Close()

	return exists, err
}

func createTable(db *pgx.Conn) error {
	dbSchema := `
        CREATE TABLE inventory.hosts_v1_0 (
            id uuid PRIMARY KEY,
            account character varying(10) NOT NULL,
            display_name character varying(200) NOT NULL,
            tags jsonb NOT NULL,
            updated timestamp with time zone NOT NULL,
            created timestamp with time zone NOT NULL,
            stale_timestamp timestamp with time zone NOT NULL
        );
        CREATE INDEX hosts_v1_0_account_index ON inventory.hosts_v1_0 (account);
        CREATE INDEX hosts_v1_0_display_name_index ON inventory.hosts_v1_0 (display_name);
        CREATE INDEX hosts_v1_0_tags_index ON inventory.hosts_v1_0 USING GIN (tags JSONB_PATH_OPS);
        CREATE INDEX hosts_v1_0_stale_timestamp_index ON inventory.hosts_v1_0 (stale_timestamp);`
	_, err := db.Exec(dbSchema)
	return err
}

func updateView(db *pgx.Conn) error {
	_, err := db.Exec(`CREATE OR REPLACE view inventory.hosts as select * from inventory.hosts_v1_0`)
	return err
}

func createConnector(cr *cyndiv1beta1.CyndiPipeline, r *ReconcileCyndiPipeline) error {
	connector := newConnectorForCR(cr)
	if err := controllerutil.SetControllerReference(cr, connector, r.scheme); err != nil {
		return err
	}

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "KafkaConnector",
		Version: "kafka.strimzi.io/v1alpha1",
	})

	err := r.client.Get(context.TODO(), client.ObjectKey{Name: "my-source-connector", Namespace: "default"}, found)
	if err != nil && errors.IsNotFound(err) {
		err = r.client.Create(context.TODO(), connector)
	}
	return err
}
