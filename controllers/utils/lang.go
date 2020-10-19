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
