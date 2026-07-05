package useractivity

import (
	"net/http"
	"strconv"
	"strings"
)

type queryOptions struct {
	offset   int
	limit    int
	sortBy   string
	sortDesc bool
	keyword  string
	after    int64
	before   int64
	contexts map[int]bool
	types    map[int]bool
}

func parseQuery(r *http.Request) queryOptions {
	query := r.URL.Query()

	return queryOptions{
		offset:   intParam(query.Get("offset"), 0),
		limit:    intParam(query.Get("limit"), 10),
		sortBy:   query.Get("sortBy"),
		sortDesc: boolParam(query.Get("sortDesc")),
		keyword:  strings.ToLower(strings.TrimSpace(query.Get("keyword"))),
		after:    int64Param(query.Get("after"), 0),
		before:   int64Param(query.Get("before"), 0),
		contexts: intSet(query["contexts"]),
		types:    intSet(query["types"]),
	}
}

func intParam(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}

	return parsed
}

func int64Param(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return fallback
	}

	return parsed
}

func boolParam(value string) bool {
	parsed, _ := strconv.ParseBool(value)
	return parsed
}

func intSet(values []string) map[int]bool {
	result := map[int]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			parsed, err := strconv.Atoi(strings.TrimSpace(part))
			if err == nil {
				result[parsed] = true
			}
		}
	}

	return result
}

func paginate[T any](items []T, offset int, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}

	if limit == 0 {
		return items[offset:]
	}

	end := offset + limit
	if end > len(items) {
		end = len(items)
	}

	return items[offset:end]
}

