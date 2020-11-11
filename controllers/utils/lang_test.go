package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lang", func() {
	Describe("Difference", func() {
		It("Computes difference properly", func() {
			a := []string{"a", "b", "c", "d"}
			b := []string{"a", "b", "d", "e"}

			diff := Difference(a, b)
			Expect(diff).To(HaveLen(1))
			Expect(diff[0]).To(Equal("c"))
		})

		It("Computes difference properly (2)", func() {
			a := []string{"a", "b", "c", "d"}
			b := []string{"a", "b", "d", "e"}

			diff := Difference(b, a)
			Expect(diff).To(HaveLen(1))
			Expect(diff[0]).To(Equal("e"))
		})
	})

	Describe("Omit", func() {
		It("Leaves out given keys", func() {
			value := make(map[string]string)
			value["foo"] = "bar"
			value["baz"] = "abcd"

			result := Omit(value, "baz")
			Expect(result).To(HaveKey("foo"))
			Expect(result).ToNot(HaveKey("baz"))
		})
	})
})
