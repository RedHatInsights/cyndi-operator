package config

import (
	"context"
	"cyndi-operator/test"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestParsers(t *testing.T) {
	test.Setup(t, "Parsers")
}

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

		It("Detects missing field", func() {
			var expected = DBParams{
				Host:     "host",
				Port:     "5432",
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
					"db.user":     []byte(expected.User),
					"db.password": []byte(expected.Password),
				},
			}

			_, err := ParseDBSecret(secret)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("db.name missing from test secret"))
		})
	})
})
