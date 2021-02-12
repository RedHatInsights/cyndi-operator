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

		_, _ = db.Exec(`CREATE ROLE cyndi_reader;`)
		_, err = db.Exec(`DROP SCHEMA IF EXISTS "inventory" CASCADE; CREATE SCHEMA "inventory";`)
		Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
		})

		It("should check for table existence", func() {
			err := db.CreateTable(TestTable, config.DBTableInitScript)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			err = db.UpdateView(TestTable)
			Expect(err).ToNot(HaveOccurred())

			rows, err := db.RunQuery("SELECT * FROM inventory.hosts")
			Expect(err).ToNot(HaveOccurred())
			rows.Close()
		})

		It("should be able to fetch the current table", func() {
			err := db.CreateTable(TestTable, config.DBTableInitScript)
			Expect(err).ToNot(HaveOccurred())

			err = db.UpdateView(TestTable)
			Expect(err).ToNot(HaveOccurred())

			table, err := db.GetCurrentTable()
			Expect(err).ToNot(HaveOccurred())
			Expect(*table).To(Equal(TestTable))
		})

		It("should return nil if the view does not exist", func() {
			err := db.CreateTable(TestTable, config.DBTableInitScript)
			Expect(err).ToNot(HaveOccurred())

			table, err := db.GetCurrentTable()
			Expect(err).ToNot(HaveOccurred())
			Expect(table).To(BeNil())
		})

		It("should return empty array if there are no tables", func() {
			tables, err := db.GetCyndiTables()
			Expect(err).ToNot(HaveOccurred())
			Expect(tables).To(HaveLen(0))
		})

		It("should list Cyndi tables", func() {
			err := db.CreateTable("hosts_v1_1", config.DBTableInitScript)
			Expect(err).ToNot(HaveOccurred())

			err = db.CreateTable("hosts_v1_2", config.DBTableInitScript)
			Expect(err).ToNot(HaveOccurred())

			err = db.CreateTable("hosts_v1_3", config.DBTableInitScript)
			Expect(err).ToNot(HaveOccurred())

			tables, err := db.GetCyndiTables()
			Expect(err).ToNot(HaveOccurred())
			Expect(tables).To(HaveLen(3))
			Expect(tables[0]).To(Equal("hosts_v1_1"))
			Expect(tables[1]).To(Equal("hosts_v1_2"))
			Expect(tables[2]).To(Equal("hosts_v1_3"))
		})
	})
})
