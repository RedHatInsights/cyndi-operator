package database

import (
	"fmt"
	logr "github.com/go-logr/logr/testing"
	"testing"

	"github.com/RedHatInsights/cyndi-operator/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	test.Setup(t, "Database")
}

func createHbiTable(db Database, TestTable string) {
	rows, err := db.RunQuery(fmt.Sprintf("CREATE TABLE %s (id uuid PRIMARY KEY, insights_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid)", TestTable))
	Expect(err).ToNot(HaveOccurred())
	rows.Close()
}

func seedHbiTable(db Database, TestTable string, insights bool, ids ...string) {
	// Default insights_id is the zero UUID (no insights_id)
	var insightsId = "00000000-0000-0000-0000-000000000000"

	if insights {
		insightsId = "7597d33e-a1a6-4fda-ad1e-b86b73c722fd"
	}

	for _, id := range ids {
		rows, err := db.RunQuery(fmt.Sprintf("INSERT INTO %s (id, insights_id) VALUES ('%s', '%s')", TestTable, id, insightsId))
		Expect(err).ToNot(HaveOccurred())
		rows.Close()
	}
}

var _ = Describe("Database", func() {
	var db Database

	BeforeEach(uniqueTable)
	BeforeEach(func() {
		db = NewBaseDatabase(getDBParams(), logr.TestLogger{})

		err := db.Connect()
		Expect(err).ToNot(HaveOccurred())

		_, _ = db.Exec(`CREATE ROLE cyndi_reader;`)
		_, err = db.Exec(`DROP SCHEMA IF EXISTS "inventory" CASCADE; CREATE SCHEMA "inventory";`)
		Expect(err).ToNot(HaveOccurred())

		createHbiTable(db, TestTable)
	})

	AfterEach(func() {
		if db != nil {
			err := db.Close()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("With database connection", func() {
		It("Simple query", func() {
			rows, err := db.RunQuery("SELECT 1+1")
			Expect(err).ToNot(HaveOccurred())
			rows.Close()
		})

		Describe("Counting hosts", func() {
			It("Counts all hosts", func() {
				seedHbiTable(db, TestTable, false, "374e613b-ee69-49e4-b0e8-3886f1f512ef", "56d7bb17-b6f6-40a8-a37b-55432efc990a")

				count, err := db.CountHosts(TestTable, false, []map[string]string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(2)))
			})

			It("Counts insights hosts", func() {
				seedHbiTable(db, TestTable, false, "374e613b-ee69-49e4-b0e8-3886f1f512ef")
				seedHbiTable(db, TestTable, true, "4db4bc46-ccf1-447f-8485-3f39c719fde7", "9cb651e4-3505-4f62-bb00-12fd9a19cd63")

				count, err := db.CountHosts(TestTable, true, []map[string]string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(2)))
			})

			It("Counts insights hosts with additionalFilters", func() {
				seedHbiTable(db, TestTable, false, "374e613b-ee69-49e4-b0e8-3886f1f512ef")
				seedHbiTable(db, TestTable, true, "4db4bc46-ccf1-447f-8485-3f39c719fde7", "9cb651e4-3505-4f62-bb00-12fd9a19cd63")

				filters := []map[string]string{{"name": "testFilter", "type": "com.redhat.insights.kafka.connect.transforms.Filter", "if": "record.headers().lastWithName('insights_id').value() && record.headers().lastWithName('insights_id').value() != '00000000-0000-0000-0000-000000000000'", "where": "insights_id != '00000000-0000-0000-0000-000000000000'::uuid"}}

				count, err := db.CountHosts(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(2)))
			})
		})

		Describe("Fetching host ids", func() {
			It("Gets all host ids", func() {
				seedHbiTable(db, TestTable, false, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4")

				ids, err := db.GetHostIds(TestTable, false, []map[string]string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(ids).To(HaveLen(3))
				Expect(ids[0]).To(Equal("2c201892-f907-414c-ad85-a455f71a90c0"))
				Expect(ids[1]).To(Equal("8dbfff32-b59e-40e5-b784-bdcbff7d8ac4"))
				Expect(ids[2]).To(Equal("a77d5711-b670-4ead-97e1-c091624c5f22"))
			})

			It("Gets insights host ids", func() {
				seedHbiTable(db, TestTable, false, "1ed2df3f-db4c-4002-8e89-c63d21a55e49", "2d5f1895-0d6b-4655-8c09-5f2c04fa0d8a")
				seedHbiTable(db, TestTable, true, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4")

				ids, err := db.GetHostIds(TestTable, true, []map[string]string{})
				Expect(err).ToNot(HaveOccurred())
				Expect(ids).To(HaveLen(3))
				Expect(ids[0]).To(Equal("2c201892-f907-414c-ad85-a455f71a90c0"))
				Expect(ids[1]).To(Equal("8dbfff32-b59e-40e5-b784-bdcbff7d8ac4"))
				Expect(ids[2]).To(Equal("a77d5711-b670-4ead-97e1-c091624c5f22"))
			})

			It("Gets insights host ids with additionalFilters", func() {
				seedHbiTable(db, TestTable, false, "1ed2df3f-db4c-4002-8e89-c63d21a55e49", "2d5f1895-0d6b-4655-8c09-5f2c04fa0d8a")
				seedHbiTable(db, TestTable, true, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4")

				filters := []map[string]string{{"name": "testFilter", "type": "com.redhat.insights.kafka.connect.transforms.Filter", "if": "record.headers().lastWithName('insights_id').value() && record.headers().lastWithName('insights_id').value() != '00000000-0000-0000-0000-000000000000'", "where": "insights_id != '00000000-0000-0000-0000-000000000000'::uuid"}}

				ids, err := db.GetHostIds(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(ids).To(HaveLen(3))
				Expect(ids[0]).To(Equal("2c201892-f907-414c-ad85-a455f71a90c0"))
				Expect(ids[1]).To(Equal("8dbfff32-b59e-40e5-b784-bdcbff7d8ac4"))
				Expect(ids[2]).To(Equal("a77d5711-b670-4ead-97e1-c091624c5f22"))
			})
		})
	})
})
