package cyndipipeline

import (
	"context"
	cyndiv1beta1 "cyndi-operator/pkg/apis/cyndi/v1beta1"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/google/go-cmp/cmp"
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
	"strconv"
	"strings"
	"time"
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

	log.Info("instance.Status.PipelineVersion")
	log.Info(instance.Status.PipelineVersion)
	log.Info(instance.Spec.AppName)
	log.Info(instance.Spec.AppDBHostname)

	if instance.Status.PipelineVersion == "" {
		log.Info("Setting pipeline version vars")
		instance.Status.PipelineVersion = fmt.Sprintf(
			"1_%s",
			strconv.FormatInt(time.Now().UnixNano(), 10))

		log.Info(instance.Status.PipelineVersion)

		instance.Status.ConnectorName = fmt.Sprintf(
			"syndication-pipeline-%s-%s",
			instance.Spec.AppName,
			strings.Replace(instance.Status.PipelineVersion, "_", "-", 1))

		log.Info(instance.Status.ConnectorName)

		instance.Status.TableName = fmt.Sprintf(
			"hosts_v%s",
			instance.Status.PipelineVersion)
		log.Info(instance.Status.TableName)
	}

	err = r.client.Status().Update(context.TODO(), instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Setting up database")
	appDb, err := connectToAppDB(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	exists, err := checkIfTableExists(instance, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}

	if exists != true {
		reqLogger.Info("Creating table")
		err = createTable(instance, appDb)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		reqLogger.Info("Table exists")
	}

	isValid, err := validate(instance, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}
	reqLogger.Info(strconv.FormatBool(isValid))

	err = updateView(instance, appDb)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = createConnector(instance, r)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = appDb.Close()
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Reconcile complete, don't requeue", "Pod.Namespace", "default", "Pod.Name", "my-source-connector")
	return reconcile.Result{RequeueAfter: time.Second * 15}, nil
}

func connectToInventoryDB(instance *cyndiv1beta1.CyndiPipeline) (*pgx.Conn, error) {
	connStr := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s sslmode=%s",
		instance.Spec.InventoryDBHostname,
		instance.Spec.InventoryDBUser,
		instance.Spec.InventoryDBPassword,
		instance.Spec.InventoryDBName,
		instance.Spec.InventoryDBSSLMode)
	config, err := pgx.ParseDSN(connStr)
	db, err := pgx.Connect(config)
	return db, err
}

func connectToAppDB(instance *cyndiv1beta1.CyndiPipeline) (*pgx.Conn, error) {
	connStr := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s sslmode=%s",
		instance.Spec.AppDBHostname,
		instance.Spec.AppDBUser,
		instance.Spec.AppDBPassword,
		instance.Spec.AppDBName,
		instance.Spec.AppDBSSLMode)
	config, err := pgx.ParseDSN(connStr)
	db, err := pgx.Connect(config)
	return db, err
}

func checkIfTableExists(instance *cyndiv1beta1.CyndiPipeline, db *pgx.Conn) (bool, error) {
	query := fmt.Sprintf(
		"SELECT exists (SELECT FROM information_schema.tables WHERE table_schema = 'inventory' AND table_name = '%s')",
		instance.Status.TableName)
	rows, err := db.Query(query)

	var exists bool
	rows.Next()
	err = rows.Scan(&exists)
	if err != nil {
		return false, err
	}
	rows.Close()

	return exists, err
}

func createTable(instance *cyndiv1beta1.CyndiPipeline, db *pgx.Conn) error {
	//TODO: use a template, or something cleaner
	dbSchema := fmt.Sprintf(`
        CREATE TABLE inventory.%s (
            id uuid PRIMARY KEY,
            account character varying(10) NOT NULL,
            display_name character varying(200) NOT NULL,
            tags jsonb NOT NULL,
            updated timestamp with time zone NOT NULL,
            created timestamp with time zone NOT NULL,
            stale_timestamp timestamp with time zone NOT NULL
        );
        CREATE INDEX %s_account_index ON inventory.%s (account);
        CREATE INDEX %s_display_name_index ON inventory.%s (display_name);
        CREATE INDEX %s_tags_index ON inventory.%s USING GIN (tags JSONB_PATH_OPS);
        CREATE INDEX %s_stale_timestamp_index ON inventory.%s (stale_timestamp);`,
		instance.Status.TableName, instance.Status.TableName, instance.Status.TableName, instance.Status.TableName, instance.Status.TableName,
		instance.Status.TableName, instance.Status.TableName, instance.Status.TableName, instance.Status.TableName)
	log.Info(dbSchema)
	_, err := db.Exec(dbSchema)
	return err
}

func updateView(instance *cyndiv1beta1.CyndiPipeline, db *pgx.Conn) error {
	_, err := db.Exec(fmt.Sprintf(`CREATE OR REPLACE view inventory.hosts as select * from inventory.%s`, instance.Status.TableName))
	return err
}

func newConnectorForCR(instance *cyndiv1beta1.CyndiPipeline) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      instance.Status.ConnectorName,
			"namespace": instance.Namespace,
			"labels": map[string]interface{}{
				"strimzi.io/cluster": instance.Spec.KafkaConnectCluster,
			},
		},
		"spec": map[string]interface{}{
			"tasksMax": 2,
			"config": map[string]interface{}{
				"connector.class":                        "io.confluent.connect.jdbc.JdbcSinkConnector",
				"tasks.max":                              "1",
				"topics":                                 "platform.inventory.events",
				"key.converter":                          "org.apache.kafka.connect.storage.StringConverter",
				"value.converter":                        "org.apache.kafka.connect.json.JsonConverter",
				"value.converter.schemas.enable":         false,
				"connection.url":                         "jdbc:postgresql://<%= DB_HOSTNAME %>:<%= DB_PORT %>/<%= DB_NAME %>",
				"connection.user":                        "<%= DB_USER %>",
				"connection.password":                    "<%= DB_PASSWORD %>",
				"dialect.name":                           "EnhancedPostgreSqlDatabaseDialect",
				"auto.create":                            false,
				"insert.mode":                            "upsert",
				"delete.enabled":                         true,
				"batch.size":                             "3000",
				"table.name.format":                      "inventory.hosts_<%= INDEX_VERSION %>_<%= INDEX_MINOR_VERSION %>",
				"pk.mode":                                "record_key",
				"pk.fields":                              "id",
				"fields.whitelist":                       "account,display_name,tags,updated,created,stale_timestamp",
				"transforms":                             "deleteToTombstone,extractHost,tagsToJson,injectSchemaKey,injectSchemaValue",
				"transforms.deleteToTombstone.type":      "com.redhat.insights.kafka.connect.transforms.DropIf$Value",
				"transforms.deleteToTombstone.predicate": "'delete'.equals(record.headers().lastWithName('event_type').value())",
				"transforms.extractHost.type":            "org.apache.kafka.connect.transforms.ExtractField$Value",
				"transforms.extractHost.field":           "host",
				"transforms.tagsToJson.type":             "com.redhat.kafka.connect.transforms.FieldToJson$Value",
				"transforms.tagsToJson.originalField":    "tags",
				"transforms.tagsToJson.destinationField": "tags",
				"transforms.injectSchemaKey.type":        "com.redhat.kafka.connect.transforms.InjectSchema$Key",
				"transforms.injectSchemaKey.schema":      "{\"type\":\"string\",\"optional\":false, \"name\": \"com.redhat.kafka.connect.transforms.pgtype=uuid\"}",
				"transforms.injectSchemaValue.type":      "com.redhat.kafka.connect.transforms.InjectSchema$Value",
				"transforms.injectSchemaValue.schema":    "{\"type\":\"struct\",\"fields\":[{\"type\":\"string\",\"optional\":false,\"field\":\"account\"},{\"type\":\"string\",\"optional\":false,\"field\":\"display_name\"},{\"type\":\"string\",\"optional\":false,\"field\":\"tags\", \"name\": \"com.redhat.kafka.connect.transforms.pgtype=jsonb\"},{\"type\":\"string\",\"optional\":false,\"field\":\"updated\", \"name\": \"com.redhat.kafka.connect.transforms.pgtype=timestamptz\"},{\"type\":\"string\",\"optional\":false,\"field\":\"created\", \"name\": \"com.redhat.kafka.connect.transforms.pgtype=timestamptz\"},{\"type\":\"string\",\"optional\":false,\"field\":\"stale_timestamp\", \"name\": \"com.redhat.kafka.connect.transforms.pgtype=timestamptz\"}],\"optional\":false}",
			},
		},
	}

	u.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "KafkaConnector",
		Version: "kafka.strimzi.io/v1alpha1",
	})
	return u
}

func createConnector(instance *cyndiv1beta1.CyndiPipeline, r *ReconcileCyndiPipeline) error {
	connector := newConnectorForCR(instance)
	if err := controllerutil.SetControllerReference(instance, connector, r.scheme); err != nil {
		return err
	}

	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "KafkaConnector",
		Version: "kafka.strimzi.io/v1alpha1",
	})

	err := r.client.Get(context.TODO(), client.ObjectKey{Name: instance.Status.ConnectorName, Namespace: instance.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		err = r.client.Create(context.TODO(), connector)
	}
	return err
}

type host struct {
	ID          string
	Account     string
	DisplayName string
	Tags        string
}

func getSystemsFromAppDB(instance *cyndiv1beta1.CyndiPipeline, db *pgx.Conn, now string) ([]host, error) {
	insightsOnlyQuery := ""
	if instance.Spec.InsightsOnly == true {
		insightsOnlyQuery = "AND canonical_facts ? 'insights_id'"
	}

	query := fmt.Sprintf("SELECT id, account, display_name, tags FROM inventory.%s WHERE updated < '%s' %s ORDER BY id LIMIT 10 OFFSET 0", instance.Status.TableName, now, insightsOnlyQuery)
	hosts, err := getSystemsFromDB(db, query)
	return hosts, err
}

func getSystemsFromHBIDB(instance *cyndiv1beta1.CyndiPipeline, now string) ([]host, error) {
	db, err := connectToInventoryDB(instance)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT id, account, display_name, tags FROM hosts WHERE modified_on < '%s' ORDER BY id LIMIT 10 OFFSET 0", now)
	hosts, err := getSystemsFromDB(db, query)
	return hosts, err
}

func getSystemsFromDB(db *pgx.Conn, query string) ([]host, error) {
	hosts, err := db.Query(query)
	var hostsParsed []host

	if err != nil {
		return hostsParsed, err
	}

	defer hosts.Close()

	for hosts.Next() {
		var (
			id          uuid.UUID
			account     string
			displayName string
			tags        string
		)
		err = hosts.Scan(&id, &account, &displayName, &tags)
		if err != nil {
			return hostsParsed, err
		}

		hostsParsed = append(hostsParsed, host{ID: fmt.Sprintf("%x", id), Account: account, DisplayName: displayName, Tags: tags})
	}

	return hostsParsed, nil
}

func printHosts(hosts []host) {
	for _, h := range hosts {
		hostStr := h.ID + ", " + h.Account + ", " + h.DisplayName + ", " + h.Tags
		log.Info(hostStr)
	}

}

func validate(instance *cyndiv1beta1.CyndiPipeline, appDb *pgx.Conn) (bool, error) {
	now := time.Now().Format(time.RFC3339)
	hbiHosts, err := getSystemsFromHBIDB(instance, now)
	if err != nil {
		return false, err
	}
	appHosts, err := getSystemsFromAppDB(instance, appDb, now)
	if err != nil {
		return false, err
	}

	log.Info("hbihosts")
	printHosts(hbiHosts)
	log.Info("apphosts")
	printHosts(appHosts)

	diff := cmp.Diff(hbiHosts, appHosts)
	diff = strings.ReplaceAll(diff, "\n", "")
	diff = strings.ReplaceAll(diff, "\t", "")
	log.Info(diff)

	return diff == "", err
}
