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
	rows := redisValueRows([]any{"one", []byte("two")})

	want := []map[string]string{
		{"Index": "1", "Value": "one"},
		{"Index": "2", "Value": "two"},
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
