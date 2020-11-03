package config

import (
	"cyndi-operator/test"
	"fmt"
	"testing"

	cyndi "cyndi-operator/api/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func TestConfig(t *testing.T) {
	test.Setup(t, "Config")
}

func assertDefaults(config *CyndiConfiguration) {
	Expect(config.Topic).To(Equal(defaultTopic))
	Expect(config.ConnectCluster).To(Equal(defaultConnectCluster))
	Expect(config.ConnectorTemplate).To(Equal(defaultConnectorTemplate))
	Expect(config.ConnectorTasksMax).To(Equal(defaultConnectorTasksMax))
	Expect(config.ConnectorBatchSize).To(Equal(defaultConnectorBatchSize))
	Expect(config.ConnectorMaxAge).To(Equal(defaultConnectorMaxAge))
	Expect(config.DBTableInitScript).To(Equal(defaultDBTableInitScript))
	Expect(config.ValidationConfig).To(Equal(defaultValidationConfig))
	Expect(config.ValidationConfigInit).To(Equal(defaultValidationConfigInit))
}

var _ = Describe("Config", func() {
	It("Provides reasonable defaults", func() {
		config, err := BuildCyndiConfig(nil, nil)
		Expect(err).ToNot(HaveOccurred())
		assertDefaults(config)
	})

	It("Uses defaults with empty ConfigMap", func() {
		cm := &corev1.ConfigMap{
			Data: map[string]string{},
		}

		config, err := BuildCyndiConfig(nil, cm)
		Expect(err).ToNot(HaveOccurred())
		assertDefaults(config)
	})

	It("Overrides fields using ConfigMap", func() {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				"connector.topic":                      "platform.inventory.host-egress",
				"connect.cluster":                      "cluster01",
				"connector.config":                     "{}",
				"connector.tasks.max":                  "12",
				"connector.batch.size":                 "13",
				"connector.max.age":                    "14",
				"db.schema":                            "CREATE TABLE hosts ()",
				"standard.interval":                    "7200",
				"validation.interval":                  "51",
				"validation.attempts.threshold":        "52",
				"validation.percentage.threshold":      "53",
				"init.validation.interval":             "54",
				"init.validation.attempts.threshold":   "55",
				"init.validation.percentage.threshold": "56",
			},
		}

		config, err := BuildCyndiConfig(nil, cm)
		Expect(err).ToNot(HaveOccurred())

		Expect(config.Topic).To(Equal("platform.inventory.host-egress"))
		Expect(config.ConnectCluster).To(Equal("cluster01"))
		Expect(config.ConnectorTemplate).To(Equal("{}"))
		Expect(config.ConnectorTasksMax).To(Equal(int64(12)))
		Expect(config.ConnectorBatchSize).To(Equal(int64(13)))
		Expect(config.ConnectorMaxAge).To(Equal(int64(14)))
		Expect(config.DBTableInitScript).To(Equal("CREATE TABLE hosts ()"))
		Expect(config.StandardInterval).To(Equal(int64(7200)))
		Expect(config.ValidationConfig.Interval).To(Equal(int64(51)))
		Expect(config.ValidationConfig.AttemptsThreshold).To(Equal(int64(52)))
		Expect(config.ValidationConfig.PercentageThreshold).To(Equal(int64(53)))
		Expect(config.ValidationConfigInit.Interval).To(Equal(int64(54)))
		Expect(config.ValidationConfigInit.AttemptsThreshold).To(Equal(int64(55)))
		Expect(config.ValidationConfigInit.PercentageThreshold).To(Equal(int64(56)))
	})

	DescribeTable("Errors on invalid value",
		func(key string) {
			cm := &corev1.ConfigMap{
				Data: map[string]string{
					key: "abc",
				},
			}

			_, err := BuildCyndiConfig(nil, cm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf(`"%s" is not a valid value for "%s"`, "abc", key)))
		},
		Entry("connector.tasks.max", "connector.tasks.max"),
		Entry("connector.batch.size", "connector.batch.size"),
		Entry("connector.max.age", "connector.max.age"),
		Entry("standard.interval", "standard.interval"),
		Entry("validation.interval", "validation.interval"),
		Entry("validation.attempts.threshold", "validation.attempts.threshold"),
		Entry("validation.percentage.threshold", "validation.percentage.threshold"),
		Entry("init.validation.interval", "init.validation.interval"),
		Entry("init.validation.attempts.threshold", "init.validation.attempts.threshold"),
		Entry("init.validation.percentage.threshold", "init.validation.percentage.threshold"),
	)

	Describe("Override config on CR level", func() {
		It("Overrides ConnectCluster", func() {
			cm := &corev1.ConfigMap{
				Data: map[string]string{
					"connect.cluster": "cluster01",
				},
			}

			value := "cluster02"
			pipeline := cyndi.CyndiPipeline{
				Spec: cyndi.CyndiPipelineSpec{
					ConnectCluster: &value,
				},
			}

			config, err := BuildCyndiConfig(&pipeline, cm)
			Expect(err).ToNot(HaveOccurred())
			Expect(config.ConnectCluster).To(Equal("cluster02"))
		})

		It("Overrides MaxAge", func() {
			cm := &corev1.ConfigMap{
				Data: map[string]string{
					"connector.max.age": "10",
				},
			}

			value := int64(9)
			pipeline := cyndi.CyndiPipeline{
				Spec: cyndi.CyndiPipelineSpec{
					MaxAge: &value,
				},
			}

			config, err := BuildCyndiConfig(&pipeline, cm)
			Expect(err).ToNot(HaveOccurred())
			Expect(config.ConnectorMaxAge).To(Equal(int64(9)))
		})

		It("Overrides ValidationThreshold", func() {
			cm := &corev1.ConfigMap{
				Data: map[string]string{
					"validation.percentage.threshold":      "5",
					"init.validation.percentage.threshold": "6",
				},
			}

			value := int64(7)
			pipeline := cyndi.CyndiPipeline{
				Spec: cyndi.CyndiPipelineSpec{
					ValidationThreshold: &value,
				},
			}

			config, err := BuildCyndiConfig(&pipeline, cm)
			Expect(err).ToNot(HaveOccurred())
			Expect(config.ValidationConfig.PercentageThreshold).To(Equal(int64(7)))
			Expect(config.ValidationConfigInit.PercentageThreshold).To(Equal(int64(7)))
		})
	})
})
