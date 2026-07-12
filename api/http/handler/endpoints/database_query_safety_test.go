package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequiresDatabaseCommandConfirmation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		connectionType string
		query          string
		expected       bool
	}{
		{
			name:           "allows SQL select",
			connectionType: "mysql",
			query:          "SELECT * FROM users",
			expected:       false,
		},
		{
			name:           "allows SQL show",
			connectionType: "mysql",
			query:          "SHOW TABLES",
			expected:       false,
		},
		{
			name:           "requires confirmation for SQL insert",
			connectionType: "mysql",
			query:          "INSERT INTO users (name) VALUES ('Ada')",
			expected:       true,
		},
		{
			name:           "requires confirmation for SQL create table",
			connectionType: "mysql",
			query:          "CREATE TABLE audit_log (id INT)",
			expected:       true,
		},
		{
			name:           "requires confirmation for SQL drop table",
			connectionType: "postgres",
			query:          "DROP TABLE audit_log",
			expected:       true,
		},
		{
			name:           "allows Redis get",
			connectionType: "redis",
			query:          "GET session:1",
			expected:       false,
		},
		{
			name:           "allows Redis scan",
			connectionType: "redis",
			query:          "SCAN 0 MATCH session:*",
			expected:       false,
		},
		{
			name:           "allows Redis hash scan",
			connectionType: "redis",
			query:          "HSCAN user:1 0 COUNT 100",
			expected:       false,
		},
		{
			name:           "requires confirmation for Redis set",
			connectionType: "redis",
			query:          "SET session:1 value",
			expected:       true,
		},
		{
			name:           "requires confirmation for Redis expiration change",
			connectionType: "redis",
			query:          "EXPIRE session:1 60",
			expected:       true,
		},
		{
			name:           "requires confirmation for unknown Redis command",
			connectionType: "redis",
			query:          "CUSTOMCOMMAND key",
			expected:       true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, requiresDatabaseCommandConfirmation(testCase.connectionType, testCase.query))
		})
	}
}
