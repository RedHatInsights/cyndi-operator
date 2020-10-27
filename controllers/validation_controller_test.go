package controllers

import (
	"context"
	"cyndi-operator/controllers/database"
	"cyndi-operator/test"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "cyndi-operator/controllers/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	namespacedName types.NamespacedName
	dbParams       DBParams
	hbiDb          *database.Database
	appDb          *database.AppDatabase
	r              *ValidationReconciler
)

func createSeededTable(db *database.Database, TestTable string, ids ...string) {
	rows, err := db.RunQuery(fmt.Sprintf("CREATE TABLE %s (id uuid PRIMARY KEY)", TestTable))
	Expect(err).ToNot(HaveOccurred())
	rows.Close()

	seedTable(db, TestTable, ids...)
}

func seedTable(db *database.Database, TestTable string, ids ...string) {
	for _, id := range ids {
		rows, err := db.RunQuery(fmt.Sprintf("INSERT INTO %s (id) VALUES ('%s')", TestTable, id))
		Expect(err).ToNot(HaveOccurred())
		rows.Close()
	}
}

func initializePipeline(toValid bool) {
	pipeline := getPipeline(namespacedName)
	pipeline.TransitionToInitialSync("123456")
	Expect(test.Client.Status().Update(context.TODO(), pipeline)).ToNot(HaveOccurred())

	if toValid {
		pipeline = getPipeline(namespacedName)
		pipeline.SetValid(metav1.ConditionTrue, "ValidationSucceeded", "success")
		Expect(test.Client.Status().Update(context.TODO(), pipeline)).ToNot(HaveOccurred())
	}
}

var _ = Describe("Validation controller", func() {
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

		r = NewValidationReconciler(test.Client, test.Clientset, scheme.Scheme, logf.Log.WithName("test"), false)

		dbParams = getDBParams()

		createDbSecret(namespacedName.Namespace, "host-inventory-db", dbParams)
		createDbSecret(namespacedName.Namespace, fmt.Sprintf("%s-db", namespacedName.Name), dbParams)

		appDb = database.NewAppDatabase(&dbParams)
		err := appDb.Connect()
		Expect(err).ToNot(HaveOccurred())

		_, err = appDb.Exec(`DROP SCHEMA IF EXISTS "inventory" CASCADE; CREATE SCHEMA "inventory";`)
		Expect(err).ToNot(HaveOccurred())

		hbiDb = database.NewDatabase(&dbParams)
		err = hbiDb.Connect()
		Expect(err).ToNot(HaveOccurred())

		_, err = hbiDb.Exec(`DROP TABLE IF EXISTS public.hosts; CREATE TABLE public.hosts (id uuid PRIMARY KEY);`)
		Expect(err).ToNot(HaveOccurred())
	})

	var reconcile = func() (result ctrl.Result) {
		result, err := r.Reconcile(ctrl.Request{NamespacedName: namespacedName})
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

			// TODO: inheritance
			seedTable(hbiDb, "public.hosts", hosts...)
			createSeededTable(&appDb.Database, fmt.Sprintf("inventory.%s", pipeline.Status.TableName), hosts...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeTrue())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationSucceeded"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation succeeded - 0 hosts (0.00%) do not match which is below the threshold for invalid pipeline (40%)"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
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

			// TODO: inheritance
			seedTable(hbiDb, "public.hosts", hosts...)
			createSeededTable(&appDb.Database, fmt.Sprintf("inventory.%s", pipeline.Status.TableName), hosts[0:5]...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeTrue())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationSucceeded"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation succeeded - 1 hosts (16.67%) do not match which is below the threshold for invalid pipeline (20%)"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(0)))
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

			// TODO: inheritance
			seedTable(hbiDb, "public.hosts", hosts...)
			createSeededTable(&appDb.Database, fmt.Sprintf("inventory.%s", pipeline.Status.TableName), hosts[0:1]...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeFalse())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationFailed"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation failed - 2 hosts (66.67%) do not match which is above the threshold for invalid pipeline (40%)"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeTrue())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(1)))
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

			// TODO: inheritance
			seedTable(hbiDb, "public.hosts", hosts...)
			createSeededTable(&appDb.Database, fmt.Sprintf("inventory.%s", pipeline.Status.TableName), hosts[0:4]...)

			reconcile()
			pipeline = getPipeline(namespacedName)
			Expect(pipeline.IsValid()).To(BeFalse())
			Expect(pipeline.Status.Conditions[0].Reason).To(Equal("ValidationFailed"))
			Expect(pipeline.Status.Conditions[0].Message).To(Equal("Validation failed - 2 hosts (33.33%) do not match which is above the threshold for invalid pipeline (20%)"))
			Expect(pipeline.Status.InitialSyncInProgress).To(BeFalse())
			Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(1)))
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

			// TODO: inheritance
			seedTable(hbiDb, "public.hosts", hosts...)
			createSeededTable(&appDb.Database, fmt.Sprintf("inventory.%s", pipeline.Status.TableName), hosts[0:1]...)

			for i := 1; i < 10; i++ {
				reconcile()
				pipeline = getPipeline(namespacedName)
				Expect(pipeline.IsValid()).To(BeFalse())
				Expect(pipeline.Status.ValidationFailedCount).To(Equal(int64(i)))
			}
		})
	})
})
