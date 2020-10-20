package database

import (
	. "cyndi-operator/controllers/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Application Database", func() {
	var (
		db     *AppDatabase
		config *CyndiConfiguration
	)

	config, _ = BuildCyndiConfig(nil, nil)

	BeforeEach(uniqueTable)
	BeforeEach(func() {
		db = NewAppDatabase(getDBParams())

		err := db.Connect()
		Expect(err).ToNot(HaveOccurred())

		rows, err := db.runQuery(`CREATE SCHEMA "inventory";`)
		Expect(err).ToNot(HaveOccurred())
		rows.Close()
	})

	AfterEach(func() {
		if db != nil {
			err := db.Close()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("with successful connection", func() {

		It("should be able to create a table", func() {
			err := db.CreateTable(TestTable, config.DBTableInitScript)
			Expect(err).To(BeNil())
		})

		It("should check for table existence", func() {
			err := db.CreateTable(TestTable, config.DBTableInitScript)
			Expect(err).To(BeNil())

			exists, err := db.CheckIfTableExists(TestTable)
			Expect(exists).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should check for table existence (negative)", func() {
			exists, err := db.CheckIfTableExists(TestTable)
			Expect(exists).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to delete the table", func() {
			err := db.CreateTable(TestTable, config.DBTableInitScript)
			Expect(err).To(BeNil())

			err = db.DeleteTable(TestTable)
			Expect(err).ToNot(HaveOccurred())

			exists, err := db.CheckIfTableExists(TestTable)
			Expect(exists).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
		})

		It("noops if the table does not exist", func() {
			err := db.DeleteTable(TestTable)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be able to update the view", func() {
			err := db.CreateTable(TestTable, config.DBTableInitScript)
			Expect(err).To(BeNil())

			err = db.UpdateView(TestTable)
			Expect(err).To(BeNil())

			rows, err := db.runQuery("SELECT * FROM inventory.hosts")
			Expect(err).ToNot(HaveOccurred())
			rows.Close()
		})
	})
})
