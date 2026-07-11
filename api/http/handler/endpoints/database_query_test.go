package endpoints

import (
	"reflect"
	"testing"
)

func TestSplitRedisCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    []string
		wantErr bool
	}{
		{
			name:    "splits simple command",
			command: "SET key value",
			want:    []string{"SET", "key", "value"},
		},
		{
			name:    "keeps quoted value together",
			command: `SET key "hello world"`,
			want:    []string{"SET", "key", "hello world"},
		},
		{
			name:    "returns error for unterminated quote",
			command: `GET "key`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := splitRedisCommand(test.command)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestRedisValueRows(t *testing.T) {
	rows := redisValueRows([]any{"one", []byte("two")}, "1m0s")

	want := []map[string]string{
		{"Index": "1", "Value": "one", "TTL": "1m0s"},
		{"Index": "2", "Value": "two", "TTL": "1m0s"},
	}

	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
}

func TestRedisCommandKey(t *testing.T) {
	if got := redisCommandKey([]string{"GET", "sys:config"}); got != "sys:config" {
		t.Fatalf("got %q, want sys:config", got)
	}
	if got := redisCommandKey([]string{"PING"}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestRedisHashPairsToRows(t *testing.T) {
	rows, columns := redisHashPairsToRows([]any{"name", "alice", "age", "18"}, "No expiration")
	wantColumns := []string{"Field", "Value", "TTL"}
	want := []map[string]string{
		{"Field": "name", "Value": "alice", "TTL": "No expiration"},
		{"Field": "age", "Value": "18", "TTL": "No expiration"},
	}
	if !reflect.DeepEqual(columns, wantColumns) {
		t.Fatalf("columns got %v, want %v", columns, wantColumns)
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
}

func TestRedisCommandResultRowsGet(t *testing.T) {
	rows, columns := redisCommandResultRows([]string{"GET", "sys:config"}, `{"ok":true}`, "string", "No expiration")
	wantColumns := []string{"Value", "TTL"}
	want := []map[string]string{
		{"Value": `{"ok":true}`, "TTL": "No expiration"},
	}
	if !reflect.DeepEqual(columns, wantColumns) {
		t.Fatalf("columns got %v, want %v", columns, wantColumns)
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
}

func TestRedisIndexedValueRows(t *testing.T) {
	rows, columns := redisIndexedValueRows([]any{"a", "b"}, "No expiration")
	wantColumns := []string{"Index", "Value", "TTL"}
	want := []map[string]string{
		{"Index": "0", "Value": "a", "TTL": "No expiration"},
		{"Index": "1", "Value": "b", "TTL": "No expiration"},
	}
	if !reflect.DeepEqual(columns, wantColumns) {
		t.Fatalf("columns got %v, want %v", columns, wantColumns)
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
}

func TestNormalizedQueryTimeout(t *testing.T) {
	if got := normalizedQueryTimeout(0); got != defaultQueryTimeout {
		t.Fatalf("got %d, want %d", got, defaultQueryTimeout)
	}

	if got := normalizedQueryTimeout(maxQueryTimeout + 1); got != maxQueryTimeout {
		t.Fatalf("got %d, want %d", got, maxQueryTimeout)
	}

	if got := normalizedQueryTimeout(60); got != 60 {
		t.Fatalf("got %d, want 60", got)
	}
}
