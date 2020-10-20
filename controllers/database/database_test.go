package database

import (
	"cyndi-operator/test"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	test.Setup(t, "Database")
}

func createSeededTable(db *Database, TestTable string, ids ...string) {
	rows, err := db.runQuery(fmt.Sprintf("CREATE TABLE %s (id uuid PRIMARY KEY)", TestTable))
	Expect(err).ToNot(HaveOccurred())
	rows.Close()

	for _, id := range ids {
		rows, err = db.runQuery(fmt.Sprintf("INSERT INTO %s (id) VALUES ('%s')", TestTable, id))
		Expect(err).ToNot(HaveOccurred())
		rows.Close()
	}

}

var _ = Describe("Database", func() {
	var db *Database

	BeforeEach(uniqueTable)
	BeforeEach(func() {
		db = NewDatabase(getDBParams())

		err := db.Connect()
		Expect(err).ToNot(HaveOccurred())

		_, err = db.Exec(`DROP SCHEMA IF EXISTS "inventory" CASCADE; CREATE SCHEMA "inventory";`)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if db != nil {
			err := db.Close()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("With database connection", func() {
		It("Simple query", func() {
			rows, err := db.runQuery("SELECT 1+1")
			Expect(err).ToNot(HaveOccurred())
			rows.Close()
		})

		Describe("Counting hosts", func() {
			It("Counts hosts", func() {
				createSeededTable(db, TestTable, "374e613b-ee69-49e4-b0e8-3886f1f512ef", "56d7bb17-b6f6-40a8-a37b-55432efc990a")

				count, err := db.CountHosts(TestTable)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(2)))
			})
		})

		Describe("Fetching host ids", func() {
			It("Get host ids", func() {
				createSeededTable(db, TestTable, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4")

				ids, err := db.GetHostIds(TestTable)
				Expect(err).ToNot(HaveOccurred())
				Expect(ids).To(HaveLen(3))
				Expect(ids[0]).To(Equal("2c201892-f907-414c-ad85-a455f71a90c0"))
				Expect(ids[1]).To(Equal("8dbfff32-b59e-40e5-b784-bdcbff7d8ac4"))
				Expect(ids[2]).To(Equal("a77d5711-b670-4ead-97e1-c091624c5f22"))
			})
		})
	})
})
