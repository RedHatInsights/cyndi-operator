package utils

/*

Things that should have been in golang's standard library

*/

func ContainsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func Abs(x int64) int64 {
	if x < 0 {
		return -x
	}

	return x
}

type void struct{}

func Difference(a, b []string) (diff []string) {
	bMap := make(map[string]void, len(b))
	diff = []string{}

	for _, key := range b {
		bMap[key] = void{}
	}

	// find missing values in a
	for _, key := range a {
		if _, ok := bMap[key]; !ok {
			diff = append(diff, key)
		}
	}

	return diff
}

func Min(x, y int) int {
	if x < y {
		return x
	}

	return y
}
