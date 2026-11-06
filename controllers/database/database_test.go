package database

import (
	"fmt"
	"testing"

	logr "github.com/go-logr/logr/testing"

	"github.com/RedHatInsights/cyndi-operator/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	test.Setup(t, "Database")
}

func createHbiTable(db Database, TestTable string) {
	// Create table with org_id to match production structure (partitioned by org_id)
	rows, err := db.RunQuery(fmt.Sprintf(`CREATE TABLE %s (
		id uuid PRIMARY KEY,
		org_id varchar(36) NOT NULL DEFAULT 'org1',
		insights_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid
	)`, TestTable))
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
		rows, err := db.RunQuery(fmt.Sprintf("INSERT INTO %s (id, org_id, insights_id) VALUES ('%s', 'org1', '%s')", TestTable, id, insightsId))
		Expect(err).ToNot(HaveOccurred())
		rows.Close()
	}
}

func seedHbiTableWithOrg(db Database, TestTable string, orgId string, insights bool, ids ...string) {
	// Seed hosts with specific org_id to test partition-aware JOINs
	var insightsId = "00000000-0000-0000-0000-000000000000"

	if insights {
		insightsId = "7597d33e-a1a6-4fda-ad1e-b86b73c722fd"
	}

	for _, id := range ids {
		rows, err := db.RunQuery(fmt.Sprintf("INSERT INTO %s (id, org_id, insights_id) VALUES ('%s', '%s', '%s')", TestTable, id, orgId, insightsId))
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

		Describe("JOIN clause support", func() {
			var profileTable string

			BeforeEach(func() {
				// Create a related table to simulate system_profiles_static
				// Uses (org_id, host_id) composite key to match production structure
				profileTable = TestTable + "_profiles"
				rows, err := db.RunQuery(fmt.Sprintf(`CREATE TABLE %s (
				org_id varchar(36) NOT NULL,
				host_id uuid NOT NULL,
				host_type varchar(12),
				os_name varchar(100),
				PRIMARY KEY (org_id, host_id)
			)`, profileTable))
				Expect(err).ToNot(HaveOccurred())
				rows.Close()
			})

			seedProfileTable := func(orgId string, hostId string, hostType string, osName string) {
				var hostTypeValue, osNameValue string
				if hostType == "" {
					hostTypeValue = "NULL"
				} else {
					hostTypeValue = fmt.Sprintf("'%s'", hostType)
				}
				if osName == "" {
					osNameValue = "NULL"
				} else {
					osNameValue = fmt.Sprintf("'%s'", osName)
				}
				rows, err := db.RunQuery(fmt.Sprintf(
					"INSERT INTO %s (org_id, host_id, host_type, os_name) VALUES ('%s', '%s', %s, %s)",
					profileTable, orgId, hostId, hostTypeValue, osNameValue))
				Expect(err).ToNot(HaveOccurred())
				rows.Close()
			}

			It("Counts hosts with JOIN clause using org_id first (partition pruning order)", func() {
				// Create hosts
				seedHbiTable(db, TestTable, true, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4")

				// Create profiles - one edge host, two non-edge hosts
				// Using org_id to simulate partitioned table structure
				seedProfileTable("org1", "a77d5711-b670-4ead-97e1-c091624c5f22", "edge", "RHEL")
				seedProfileTable("org1", "2c201892-f907-414c-ad85-a455f71a90c0", "", "RHEL")
				seedProfileTable("org1", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4", "", "RHEL")

				// Filter for non-edge hosts using JOIN with org_id first (optimal for partition pruning)
				filters := []map[string]string{{
					"name":  "nonEdge",
					"join":  fmt.Sprintf("LEFT JOIN %s sps ON sps.org_id = h.org_id AND sps.host_id = h.id", profileTable),
					"where": "sps.host_type IS NULL",
				}}

				count, err := db.CountHosts(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(2)))
			})

			It("Gets host ids with JOIN clause using org_id first (partition pruning order)", func() {
				// Create hosts
				seedHbiTable(db, TestTable, true, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4")

				// Create profiles - one edge host, two non-edge hosts
				seedProfileTable("org1", "a77d5711-b670-4ead-97e1-c091624c5f22", "edge", "RHEL")
				seedProfileTable("org1", "2c201892-f907-414c-ad85-a455f71a90c0", "", "RHEL")
				seedProfileTable("org1", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4", "", "RHEL")

				// Filter for non-edge hosts using JOIN with org_id first
				filters := []map[string]string{{
					"name":  "nonEdge",
					"join":  fmt.Sprintf("LEFT JOIN %s sps ON sps.org_id = h.org_id AND sps.host_id = h.id", profileTable),
					"where": "sps.host_type IS NULL",
				}}

				ids, err := db.GetHostIds(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(ids).To(HaveLen(2))
				Expect(ids).To(ContainElement("2c201892-f907-414c-ad85-a455f71a90c0"))
				Expect(ids).To(ContainElement("8dbfff32-b59e-40e5-b784-bdcbff7d8ac4"))
			})

			It("Deduplicates multiple identical JOIN clauses", func() {
				// Create hosts
				seedHbiTable(db, TestTable, true, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0")

				// Create profiles
				seedProfileTable("org1", "a77d5711-b670-4ead-97e1-c091624c5f22", "edge", "CentOS")
				seedProfileTable("org1", "2c201892-f907-414c-ad85-a455f71a90c0", "", "RHEL")

				joinClause := fmt.Sprintf("LEFT JOIN %s sps ON sps.org_id = h.org_id AND sps.host_id = h.id", profileTable)

				// Multiple filters using the same JOIN (should be deduplicated)
				filters := []map[string]string{
					{
						"name":  "nonEdge",
						"join":  joinClause,
						"where": "sps.host_type IS NULL",
					},
					{
						"name":  "nonCentOS",
						"join":  joinClause,
						"where": "COALESCE(sps.os_name, '') NOT ILIKE '%centos%'",
					},
				}

				// Should find only the RHEL non-edge host
				count, err := db.CountHosts(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(1)))

				ids, err := db.GetHostIds(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(ids).To(HaveLen(1))
				Expect(ids[0]).To(Equal("2c201892-f907-414c-ad85-a455f71a90c0"))
			})

			It("Works with mixed filters (with and without JOIN)", func() {
				// Create hosts - mix of insights and non-insights
				seedHbiTable(db, TestTable, false, "1ed2df3f-db4c-4002-8e89-c63d21a55e49")
				seedHbiTable(db, TestTable, true, "a77d5711-b670-4ead-97e1-c091624c5f22", "2c201892-f907-414c-ad85-a455f71a90c0")

				// Create profiles
				seedProfileTable("org1", "1ed2df3f-db4c-4002-8e89-c63d21a55e49", "", "RHEL")
				seedProfileTable("org1", "a77d5711-b670-4ead-97e1-c091624c5f22", "edge", "RHEL")
				seedProfileTable("org1", "2c201892-f907-414c-ad85-a455f71a90c0", "", "RHEL")

				// Filter: non-edge hosts with insights_id
				filters := []map[string]string{
					{
						"name":  "insightsOnly",
						"where": "insights_id != '00000000-0000-0000-0000-000000000000'::uuid",
					},
					{
						"name":  "nonEdge",
						"join":  fmt.Sprintf("LEFT JOIN %s sps ON sps.org_id = h.org_id AND sps.host_id = h.id", profileTable),
						"where": "sps.host_type IS NULL",
					},
				}

				// Should find only the non-edge insights host
				count, err := db.CountHosts(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(1)))
			})

			It("Correctly joins across multiple org_ids (simulating partition pruning)", func() {
				// Create hosts in different "partitions" (org_ids)
				// This simulates how production tables are partitioned by org_id
				seedHbiTableWithOrg(db, TestTable, "org1", true, "a77d5711-b670-4ead-97e1-c091624c5f22")
				seedHbiTableWithOrg(db, TestTable, "org2", true, "2c201892-f907-414c-ad85-a455f71a90c0")
				seedHbiTableWithOrg(db, TestTable, "org3", true, "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4")

				// Create profiles in matching org_ids
				// In production, the JOIN condition (sps.org_id = h.org_id) enables partition pruning
				seedProfileTable("org1", "a77d5711-b670-4ead-97e1-c091624c5f22", "edge", "RHEL")
				seedProfileTable("org2", "2c201892-f907-414c-ad85-a455f71a90c0", "", "RHEL")
				seedProfileTable("org3", "8dbfff32-b59e-40e5-b784-bdcbff7d8ac4", "edge", "CentOS")

				// Filter for non-edge hosts - should only match org2's host
				// The org_id-first JOIN order enables PostgreSQL to prune partitions efficiently
				filters := []map[string]string{{
					"name":  "nonEdge",
					"join":  fmt.Sprintf("LEFT JOIN %s sps ON sps.org_id = h.org_id AND sps.host_id = h.id", profileTable),
					"where": "sps.host_type IS NULL",
				}}

				count, err := db.CountHosts(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(1)))

				ids, err := db.GetHostIds(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(ids).To(HaveLen(1))
				Expect(ids[0]).To(Equal("2c201892-f907-414c-ad85-a455f71a90c0"))
			})

			It("Ensures org_id mismatch returns no results (partition isolation)", func() {
				// Create host in org1
				seedHbiTableWithOrg(db, TestTable, "org1", true, "a77d5711-b670-4ead-97e1-c091624c5f22")

				// Create profile in org2 with same host_id (should NOT match due to org_id mismatch)
				// This simulates partition isolation - data in different partitions won't join incorrectly
				seedProfileTable("org2", "a77d5711-b670-4ead-97e1-c091624c5f22", "edge", "RHEL")

				// Filter for edge hosts - should return 0 because org_id doesn't match
				filters := []map[string]string{{
					"name":  "edge",
					"join":  fmt.Sprintf("LEFT JOIN %s sps ON sps.org_id = h.org_id AND sps.host_id = h.id", profileTable),
					"where": "sps.host_type = 'edge'",
				}}

				count, err := db.CountHosts(TestTable, false, filters)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(int64(0)))
			})
		})
	})
})
