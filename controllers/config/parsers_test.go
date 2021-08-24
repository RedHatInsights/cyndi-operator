package config

import (
	"context"
	"fmt"
	"time"

	"github.com/RedHatInsights/cyndi-operator/test"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parsers", func() {
	var namespace string

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		err := test.Client.Create(context.TODO(), ns)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Secrets", func() {
		It("Parses a secret", func() {
			var expected = DBParams{
				Host:     "host",
				Port:     "5432",
				Name:     "database",
				User:     "username",
				Password: "password",
			}

			secret := &corev1.Secret{
				Type: corev1.SecretTypeOpaque,
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "namespace",
				},
				Data: map[string][]byte{
					"db.host":     []byte(expected.Host),
					"db.port":     []byte(expected.Port),
					"db.name":     []byte(expected.Name),
					"db.user":     []byte(expected.User),
					"db.password": []byte(expected.Password),
				},
			}

			actual, err := ParseDBSecret(secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		})

		DescribeTable("Detects missing field",
			func(key string) {
				secret := &corev1.Secret{
					Type: corev1.SecretTypeOpaque,
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "namespace",
					},
					Data: map[string][]byte{
						"db.host":     []byte("host"),
						"db.port":     []byte("5432"),
						"db.name":     []byte("database"),
						"db.user":     []byte("username"),
						"db.password": []byte("password"),
					},
				}

				delete(secret.Data, key)

				_, err := ParseDBSecret(secret)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Sprintf("%s missing from test secret", key)))
			},
			Entry("db.host", "db.host"),
			Entry("db.port", "db.port"),
			Entry("db.name", "db.name"),
			Entry("db.user", "db.user"),
			Entry("db.password", "db.password"),
		)
	})
})
