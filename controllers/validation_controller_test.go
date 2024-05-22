package controllers

import (
	"context"
	"fmt"
	logr "github.com/go-logr/logr/testing"

	cyndi "github.com/RedHatInsights/cyndi-operator/api/v1alpha1"
	"github.com/RedHatInsights/cyndi-operator/controllers/database"
	"github.com/RedHatInsights/cyndi-operator/controllers/utils"
	"github.com/RedHatInsights/cyndi-operator/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/RedHatInsights/cyndi-operator/controllers/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

/*
 * Tests for ValidationController. CyndiPipelineReconciler is mocked (see initializePipeline)
 */

func createApplicationTable(db database.Database, TestTable string) {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id uuid PRIMARY KEY)", TestTable))
	Expect(err).ToNot(HaveOccurred())
}

func seedTable(db database.Database, TestTable string, insights bool, ids ...string) {
	var template = "INSERT INTO %s (id) VALUES ('%s')"

	if insights {
		template = `INSERT INTO %s (id, canonical_facts) VALUES ('%s', '{"insights_id": "7597d33e-a1a6-4fda-ad1e-b86b73c722fd"}')`
	}

	for _, id := range ids {
		_, err := db.Exec(fmt.Sprintf(template, TestTable, id))
		Expect(err).ToNot(HaveOccurred())
	}
}

var _ = Describe("Validation controller", func() {
	var (
		namespacedName types.NamespacedName
		dbParams       DBParams
		hbiDb          database.Database
		appDb          *database.AppDatabase
		r              *ValidationReconciler
	)

	initializePipeline := func(toValid bool) {
		pipeline := getPipeline(namespacedName)
		pipeline.TransitionToInitialSync("123456")
		Expect(test.Client.Status().Update(context.TODO(), pipeline)).ToNot(HaveOccurred())

		if toValid {
			pipeline = getPipeline(namespacedName)
			pipeline.SetValid(metav1.ConditionTrue, "ValidationSucceeded", "success", 1)
			Expect(test.Client.Status().Update(context.TODO(), pipeline)).ToNot(HaveOccurred())
		}
	}

	BeforeEach(func() {
		namespacedName = types.NamespacedName{
			Name:      "test-pipeline-01",
			Namespace: test.UniqueNamespace(),
		}

		createConfigMap(namespacedName.Namespace, "cyndi", map[string]string{
			"init.validation.attempts.threshold":   "5",
			"init.validation.percentage.threshold": "40",
			"validation.attempts.threshold":        "3",
			"validation.percentage.threshold":      "20",
		})

		r = NewValidationReconciler(test.Client, test.Clientset, scheme.Scheme, logf.Log.WithName("test"), record.NewFakeRecorder(10), false)

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

	var reconcile = func() (result ctrl.Result) {
		result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: namespacedName})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())
		return
	}

	Describe("Valid pipeline", func() {
		It("Correctly validates fully in-sync table", func() {
			createPipeline(namespacedName)

			var hosts = []string{
				"3b8c0b37-6208-4323-b7df-030fee22db0c",
				"99d28b1e-aad8-4ac0-8d98-ef33e7d3856e",
				"14bcbbb5-8837-4d24-8122-1d44b65680f5",
			}

			initializePipeline(false)
			pipeline := getPipeline(namespacedName)

			seedTable(hbiDb, "public.hosts", false, hosts...)
			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			createApplicationTable(appDb, appTable)
			seedTable(appDb, appTable, false, hosts...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeTrue())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationSucceeded"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation succeeded - 0 hosts (0.00%) do not match"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
			Expect(pipeline.Status.HostCount).To(Equal(int64(3)))
		})

		It("Correctly validates fully in-sync insightsOnly table", func() {
			createPipeline(namespacedName, &cyndi.CyndiPipelineSpec{InsightsOnly: true})

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

			initializePipeline(false)
			pipeline := getPipeline(namespacedName)

			seedTable(hbiDb, "public.hosts", true, insightsHosts...)
			seedTable(hbiDb, "public.hosts", false, otherHosts...)

			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			createApplicationTable(appDb, appTable)
			seedTable(appDb, appTable, false, insightsHosts...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeTrue())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationSucceeded"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation succeeded - 0 hosts (0.00%) do not match"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
			Expect(pipeline.Status.HostCount).To(Equal(int64(3)))
		})

		It("Correctly validates pipeline that's slightly off", func() {
			createPipeline(namespacedName)

			var hosts = []string{
				"3b8c0b37-6208-4323-b7df-030fee22db0c",
				"99d28b1e-aad8-4ac0-8d98-ef33e7d3856e",
				"14bcbbb5-8837-4d24-8122-1d44b65680f5",
				"f341463d-f013-4213-91c7-824aa775283b",
				"10be75f7-f84a-47ad-9b31-c54fdfdbe0c7",
				"24b8e15c-66d8-4a03-9468-432fdd28de6a",
			}

			initializePipeline(true)
			pipeline := getPipeline(namespacedName)

			seedTable(hbiDb, "public.hosts", false, hosts...)
			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			createApplicationTable(appDb, appTable)
			seedTable(appDb, appTable, false, hosts[0:5]...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeTrue())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationSucceeded"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation succeeded - 1 hosts (16.67%) do not match"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
			Expect(pipeline.Status.HostCount).To(Equal(int64(5)))
		})
	})

	Describe("Invalid pipeline", func() {
		It("Correctly invalidates pipeline that's way off", func() {
			createPipeline(namespacedName)

			var hosts = []string{
				"3b8c0b37-6208-4323-b7df-030fee22db0c",
				"99d28b1e-aad8-4ac0-8d98-ef33e7d3856e",
				"14bcbbb5-8837-4d24-8122-1d44b65680f5",
			}

			initializePipeline(false)
			pipeline := getPipeline(namespacedName)

			seedTable(hbiDb, "public.hosts", false, hosts...)
			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			createApplicationTable(appDb, appTable)
			seedTable(appDb, appTable, false, hosts[0:1]...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeFalse())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationFailed"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation failed - 2 hosts (66.67%) do not match"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeTrue())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(1)))
			Expect(pipeline.Status.HostCount).To(Equal(int64(1)))
		})

		It("Correctly invalidates pipeline that's somewhat off", func() {
			createPipeline(namespacedName)

			var hosts = []string{
				"3b8c0b37-6208-4323-b7df-030fee22db0c",
				"99d28b1e-aad8-4ac0-8d98-ef33e7d3856e",
				"14bcbbb5-8837-4d24-8122-1d44b65680f5",
				"f341463d-f013-4213-91c7-824aa775283b",
				"10be75f7-f84a-47ad-9b31-c54fdfdbe0c7",
				"24b8e15c-66d8-4a03-9468-432fdd28de6a",
			}

			initializePipeline(true)
			pipeline := getPipeline(namespacedName)

			seedTable(hbiDb, "public.hosts", false, hosts...)
			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			createApplicationTable(appDb, appTable)
			seedTable(appDb, appTable, false, hosts[0:4]...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeFalse())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationFailed"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation failed - 2 hosts (33.33%) do not match"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(1)))
			Expect(pipeline.Status.HostCount).To(Equal(int64(4)))
		})

		It("Keeps incrementing ValidationFailedCount if failures persist", func() {
			createPipeline(namespacedName)

			var hosts = []string{
				"3b8c0b37-6208-4323-b7df-030fee22db0c",
				"99d28b1e-aad8-4ac0-8d98-ef33e7d3856e",
				"14bcbbb5-8837-4d24-8122-1d44b65680f5",
			}

			initializePipeline(false)
			pipeline := getPipeline(namespacedName)

			seedTable(hbiDb, "public.hosts", false, hosts...)
			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			createApplicationTable(appDb, appTable)
			seedTable(appDb, appTable, false, hosts[0:1]...)

			for i := 1; i < 10; i++ {
				reconcile()
				pipeline = getPipeline(namespacedName)
				Expect(pipeline.IsValid()).To(BeFalse())
				Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(i)))
			}
		})

		It("Uses threshold defined on the pipeline level", func() {
			threshold := int64(5)
			createPipeline(namespacedName, &cyndi.CyndiPipelineSpec{ValidationThreshold: &threshold})

			var hosts = []string{
				"3b8c0b37-6208-4323-b7df-030fee22db0c",
				"99d28b1e-aad8-4ac0-8d98-ef33e7d3856e",
				"14bcbbb5-8837-4d24-8122-1d44b65680f5",
				"e1f13b37-a74c-4b49-a764-bcb094713201",
				"e805b88e-f796-4c3c-8c36-3026d598e502",
				"a472db5c-cb32-492e-a1ca-e86baa0bad72",
			}

			initializePipeline(false)
			pipeline := getPipeline(namespacedName)

			seedTable(hbiDb, "public.hosts", false, hosts...)
			appTable := utils.AppFullTableName(pipeline.Status.TableName)
			createApplicationTable(appDb, appTable)
			seedTable(appDb, appTable, false, hosts[1:6]...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeFalse())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationFailed"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation failed - 1 hosts (16.67%) do not match"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeTrue())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(1)))
			Expect(pipeline.Status.HostCount).To(Equal(int64(5)))
		})
	})

	Describe("Failures", func() {
		It("Fails if HBI DB secret is missing", func() {
			dbSecret, err := utils.FetchSecret(test.Client, namespacedName.Namespace, "host-inventory-read-only-db")
			Expect(err).ToNot(HaveOccurred())
			dbSecret.Data["db.host"] = []byte("localhost")
			dbSecret.Data["db.port"] = []byte("55432")
			err = test.Client.Update(context.TODO(), dbSecret)
			Expect(err).ToNot(HaveOccurred())

			createPipeline(namespacedName)
			_, err = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: namespacedName})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix(`Error connecting to localhost:55432/test as postgres`))

			recorder, _ := r.Recorder.(*record.FakeRecorder)
			Expect(recorder.Events).To(HaveLen(1))
		})
	})
})
