package util

import "strings"

func RecursiveGetRidOfTwoSpaces(str string) string {
	str = strings.ReplaceAll(str, "  ", " ")
	if strings.Contains(str, "  ") {
		str = RecursiveGetRidOfTwoSpaces(str)
	}

	return str
}

func NormalizeStr(searchQuery string) string {
	searchQuery = strings.ReplaceAll(searchQuery, "\t", " ")
	searchQuery = strings.ReplaceAll(searchQuery, "\n", " ")
	searchQuery = strings.ReplaceAll(searchQuery, "\"", " ")
	searchQuery = RecursiveGetRidOfTwoSpaces(searchQuery)
	return searchQuery
}

func IsStrPtrNotEmpty(name *string) bool {
	if name != nil {
		if str := NormalizeStr(*name); len(str) != 0 && str != " " {
			return true
		}
	}

	return false
}
