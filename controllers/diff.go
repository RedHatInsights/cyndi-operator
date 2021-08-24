package controllers

import (
	"fmt"
	"strings"

	"github.com/RedHatInsights/cyndi-operator/controllers/utils"

	"github.com/google/go-cmp/cmp"
)

type DiffReporter struct {
	path  cmp.Path
	diffs []string
}

func (r *DiffReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *DiffReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		r.diffs = append(r.diffs, fmt.Sprintf("%#v:\n\t-: %+v\n\t+: %+v\n", r.path, vx, vy))
	}
}

func (r *DiffReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *DiffReporter) String() string {
	return strings.Join(r.diffs, "\n")
}

// normalizes different number types (e.g. float64 vs int64) by converting them to their string representation
var NumberNormalizer = cmp.FilterValues(func(x, y interface{}) bool {
	return utils.IsNumber(x) || utils.IsNumber(y)
}, cmp.Transformer("NormalizeNumbers", func(in interface{}) string {
	return fmt.Sprintf("%v", in)
}))
