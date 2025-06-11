package controllers

import (
	"context"
	"fmt"
	logr "github.com/go-logr/logr/testing"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
	connect "github.com/RedHatInsights/cyndi-operator/controllers/connect"
	"github.com/RedHatInsights/cyndi-operator/controllers/database"
	"github.com/RedHatInsights/cyndi-operator/controllers/utils"
	"github.com/RedHatInsights/cyndi-operator/test"

	. "github.com/RedHatInsights/cyndi-operator/controllers/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Integration tests", func() {
	var (
		namespacedName       types.NamespacedName
		dbParams             DBParams
		hbiDb                database.Database
		appDb                *database.AppDatabase
		cyndiReconciler      *CyndiPipelineReconciler
		validationReconciler *ValidationReconciler
	)

	var reconcile = func(reconcilers ...reconcile.Reconciler) (pipeline *cyndi.CyndiPipeline) {
		for _, r := range reconcilers {
			result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: namespacedName})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		}

		return getPipeline(namespacedName)
	}

	var seedAppTable = func(db database.Database, TestTable string, ids ...string) {
		template := `INSERT INTO %s (id, account, org_id, display_name, tags, updated, created, last_check_in, stale_timestamp, system_profile, reporter, per_reporter_staleness, arch, host_type, operating_system) VALUES ('%s', '000001', 'test01', 'test01', '{}', NOW(), NOW(), NOW(), NOW(), '{}', 'puptoo', '{}', 'test01', 'test' '{}')`

		for _, id := range ids {
			_, err := db.Exec(fmt.Sprintf(template, TestTable, id))
			Expect(err).ToNot(HaveOccurred())
		}
	}

	BeforeEach(func() {
		namespacedName = types.NamespacedName{
			Name:      "integration-test-pipeline",
			Namespace: test.UniqueNamespace(),
		}

		createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{
			"init.validation.attempts.threshold":   "5",
			"init.validation.percentage.threshold": "20",
			"validation.attempts.threshold":        "3",
			"validation.percentage.threshold":      "20",
		})

		cyndiReconciler = newCyndiReconciler()
		validationReconciler = NewValidationReconciler(test.Client, test.Clientset, scheme.Scheme, logf.Log.WithName("test"), record.NewFakeRecorder(10), false)

		dbParams = getDBParams()

		createDbSecret(namespacedName.Namespace, "host-inventory-read-only-db", dbParams)
		createDbSecret(namespacedName.Namespace, utils.AppDefaultDbSecretName(namespacedName.Name), dbParams)

		appDb = database.NewAppDatabase(&dbParams, logr.TestLogger{})
		err := appDb.Connect()
		Expect(err).ToNot(HaveOccurred())

		_, _ = appDb.Exec(`CREATE ROLE cyndi_reader;`)
		_, err = appDb.Exec(`DROP SCHEMA IF EXISTS "inventory" CASCADE; CREATE SCHEMA "inventory";`)
		Expect(err).ToNot(HaveOccurred())

		hbiDb = database.NewBaseDatabase(&dbParams, logr.TestLogger{})
		err = hbiDb.Connect()
		Expect(err).ToNot(HaveOccurred())

		_, err = hbiDb.Exec(`DROP TABLE IF EXISTS public.hosts CASCADE; CREATE TABLE public.hosts (id uuid PRIMARY KEY, canonical_facts jsonb);`)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		appDb.Close()
		hbiDb.Close()
	})

	Describe("Normal procedures", func() {
		It("Creates a new InsightsOnly pipeline", func() {
			var (
				insightsHosts = []string{
					"3b8c0b37-6208-4323-b7df-030fee22db0c",
					"99d28b1e-aad8-4ac0-8d98-ef33e7d3856e",
					"14bcbbb5-8837-4d24-8122-1d44b65680f5",
				}

				otherHosts = []string{
					"45f639ff-f1f5-4469-9a7b-35295fdb75fc",
					"d2b58af8-fd82-4be1-83b1-1d1071b8bc95",
					"5d378adc-11dc-4791-8f24-cb29e21918a4",
					"f049590f-96ca-47fb-b35c-bcc097a767d7",
				}
			)

			// TODO: move to beforeEach?
			seedTable(hbiDb, "public.hosts", true, insightsHosts...)
			seedTable(hbiDb, "public.hosts", false, otherHosts...)

			createPipeline(namespacedName, &cyndi.CyndiPipelineSpec{InsightsOnly: true})

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))

			// start initial sync
			pipeline = reconcile(cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))

			// keeps validating while initial sync is in progress
			for i := 1; i < 4; i++ {
				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
				Expect(pipeline.GetValid()).To(Equal(metav1.ConditionFalse))
				Expect(pipeline.Status.HostCount).To(Equal(int64(0)))
				Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(i)))
			}

			// keeps validating while the first few hosts are replicated
			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			seedAppTable(appDb, appTable, insightsHosts[0:2]...)

			pipeline = reconcile(validationReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionFalse))
			Expect(pipeline.Status.HostCount).To(Equal(int64(2)))

			// complete the initial sync
			seedAppTable(appDb, appTable, insightsHosts[2:3]...)
			pipeline = reconcile(validationReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionTrue))
			Expect(pipeline.Status.HostCount).To(Equal(int64(3)))

			// transition to valid and create inventory.hosts view
			pipeline = reconcile(cyndiReconciler)
			Expect(pipeline.IsValid()).To(BeTrue())
			activeTable, err := appDb.GetCurrentTable()
			Expect(err).ToNot(HaveOccurred())
			Expect(*activeTable).To(Equal(pipeline.Status.ActiveTableName))

			// further reconcilations are noop
			pipeline = reconcile(validationReconciler, cyndiReconciler)
			Expect(pipeline.IsValid()).To(BeTrue())
		})

		It("Refreshes a pipeline when it becomes out of sync", func() {
			var (
				insightsHosts = []string{
					"0038cb4d-665b-4e94-87ab-a5b8a50916c5",
					"64d799f2-2645-4818-b61a-daa53e805a72",
					"2af6bf52-e681-477b-ae5f-72e449da32e4",
				}
			)

			seedTable(hbiDb, "public.hosts", true, insightsHosts...)

			createPipeline(namespacedName, &cyndi.CyndiPipelineSpec{InsightsOnly: true})

			pipeline := getPipeline(namespacedName)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))

			// start initial sync
			pipeline = reconcile(cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))

			appTable1 := pipeline.Status.TableName
			seedAppTable(appDb, utils.AppFullTableName(appTable1), insightsHosts...)

			// transitions to valid as all hosts are replicated
			pipeline = reconcile(validationReconciler, cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
			Expect(pipeline.IsValid()).To(BeTrue())
			Expect(pipeline.Status.HostCount).To(Equal(int64(3)))

			// add a new host to HBI which "fails" to get replicated
			newHost := "0ce8b6a5-32f0-4152-995a-73a390d89744"
			seedTable(hbiDb, "public.hosts", true, newHost)

			for i := 1; i < 3; i++ {
				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INVALID))
				Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(i)))
			}

			pipeline = reconcile(validationReconciler, cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))
			Expect(pipeline.Status.ActiveTableName).To(Equal(appTable1))

			pipeline = reconcile(validationReconciler, cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
			Expect(pipeline.Status.ActiveTableName).To(Equal(appTable1))
			Expect(pipeline.Status.TableName).ToNot(Equal(pipeline.Status.ActiveTableName))

			appTable2 := pipeline.Status.TableName
			seedAppTable(appDb, utils.AppFullTableName(appTable2), insightsHosts...)
			seedAppTable(appDb, utils.AppFullTableName(appTable2), newHost)

			pipeline = reconcile(validationReconciler, cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
			Expect(pipeline.IsValid()).To(BeTrue())
			Expect(pipeline.Status.HostCount).To(Equal(int64(4)))
			Expect(appTable2).To(Equal(pipeline.Status.ActiveTableName))
		})

		It("Removes a pipeline", func() {
			var (
				insightsHosts = []string{
					"0038cb4d-665b-4e94-87ab-a5b8a50916c5",
					"64d799f2-2645-4818-b61a-daa53e805a72",
					"2af6bf52-e681-477b-ae5f-72e449da32e4",
				}
			)

			seedTable(hbiDb, "public.hosts", true, insightsHosts...)

			createPipeline(namespacedName, &cyndi.CyndiPipelineSpec{InsightsOnly: true})

			pipeline := reconcile(cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
			Expect(pipeline.GetValid()).To(Equal(metav1.ConditionUnknown))

			appTable := pipeline.Status.TableName
			seedAppTable(appDb, utils.AppFullTableName(appTable), insightsHosts...)

			// transitions to valid as all hosts are replicated
			pipeline = reconcile(validationReconciler, cyndiReconciler)
			Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))

			err := test.Client.Delete(context.TODO(), pipeline)
			Expect(err).ToNot(HaveOccurred())

			result, err := validationReconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: namespacedName})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeZero())

			result, err = cyndiReconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: namespacedName})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeZero())

			pipeline, err = utils.FetchCyndiPipeline(test.Client, namespacedName)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			connectors, err := connect.GetConnectorsForOwner(test.Client, namespacedName.Namespace, pipeline.GetUIDString())
			Expect(err).ToNot(HaveOccurred())
			Expect(connectors.Items).To(BeEmpty())

			tables, err := appDb.GetCyndiTables()
			Expect(err).ToNot(HaveOccurred())
			Expect(tables).To(BeEmpty())
		})
	})

	Describe("Abnormal procedures", func() {
		Context("Refresh does not succeed", func() {
			It("It switches to the new table if it's healthier", func() {
				var (
					insightsHosts = []string{
						"0038cb4d-665b-4e94-87ab-a5b8a50916c5",
						"64d799f2-2645-4818-b61a-daa53e805a72",
						"2af6bf52-e681-477b-ae5f-72e449da32e4",
						"9c3806b5-3425-4f35-94b6-6a255377d221",
						"04967f96-aa62-42de-95a4-008a2f662b79",
					}
				)

				// start with one host and transition to STATE_VALID
				seedTable(hbiDb, "public.hosts", true, insightsHosts[0:1]...)

				createPipeline(namespacedName, &cyndi.CyndiPipelineSpec{InsightsOnly: true})

				pipeline := getPipeline(namespacedName)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))

				// start initial sync
				pipeline = reconcile(cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))

				appTable1 := pipeline.Status.TableName
				seedAppTable(appDb, utils.AppFullTableName(appTable1), insightsHosts[0:1]...)

				// transitions to valid as all hosts are replicated
				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
				Expect(pipeline.Status.HostCount).To(Equal(int64(1)))

				// now seed the remaining hosts - making the pipeline invalid
				seedTable(hbiDb, "public.hosts", true, insightsHosts[1:5]...)

				for ; pipeline.GetState() != cyndi.STATE_NEW; pipeline = reconcile(validationReconciler, cyndiReconciler) {
				}

				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))

				appTable2 := pipeline.Status.TableName
				seedAppTable(appDb, utils.AppFullTableName(appTable2), insightsHosts[0:3]...) // replicate 3 hosts - still not valid but better than appTable1

				// as this pipeline fails to become valid eventually it is refreshed again
				for ; pipeline.GetState() != cyndi.STATE_NEW; pipeline = reconcile(validationReconciler, cyndiReconciler) {
				}

				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
				Expect(pipeline.Status.ActiveTableName).To(Equal(appTable2))
			})

			It("It keeps the old table active if it's healthier than the new one", func() {
				var (
					insightsHosts = []string{
						"0038cb4d-665b-4e94-87ab-a5b8a50916c5",
						"64d799f2-2645-4818-b61a-daa53e805a72",
						"2af6bf52-e681-477b-ae5f-72e449da32e4",
						"9c3806b5-3425-4f35-94b6-6a255377d221",
						"04967f96-aa62-42de-95a4-008a2f662b79",
					}
				)

				// start with one host and transition to STATE_VALID
				seedTable(hbiDb, "public.hosts", true, insightsHosts[0:3]...)

				createPipeline(namespacedName, &cyndi.CyndiPipelineSpec{InsightsOnly: true})

				pipeline := getPipeline(namespacedName)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_NEW))

				// start initial sync
				pipeline = reconcile(cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))

				appTable1 := pipeline.Status.TableName
				seedAppTable(appDb, utils.AppFullTableName(appTable1), insightsHosts[0:3]...)

				// transitions to valid as all hosts are replicated
				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_VALID))
				Expect(pipeline.Status.HostCount).To(Equal(int64(3)))

				// now seed the remaining hosts - making the pipeline invalid
				seedTable(hbiDb, "public.hosts", true, insightsHosts[3:5]...)

				for ; pipeline.GetState() != cyndi.STATE_NEW; pipeline = reconcile(validationReconciler, cyndiReconciler) {
				}

				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))

				appTable2 := pipeline.Status.TableName
				seedAppTable(appDb, utils.AppFullTableName(appTable2), insightsHosts[0:2]...) // replicate 2 hosts - still not valid and worse than appTable1

				// as this pipeline fails to become valid eventually it is refreshed again
				for ; pipeline.GetState() != cyndi.STATE_NEW; pipeline = reconcile(validationReconciler, cyndiReconciler) {
				}

				pipeline = reconcile(validationReconciler, cyndiReconciler)
				Expect(pipeline.GetState()).To(Equal(cyndi.STATE_INITIAL_SYNC))
				Expect(pipeline.Status.ActiveTableName).To(Equal(appTable1))
			})
		})
	})
})
