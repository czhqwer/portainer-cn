package containers

import "testing"

func TestFirstDatabaseValue(t *testing.T) {
	tests := []struct {
		name string
		row  map[string]string
		want string
	}{
		{
			name: "database column",
			row:  map[string]string{"Database": "crmeb"},
			want: "crmeb",
		},
		{
			name: "table column before type column",
			row: map[string]string{
				"Tables_in_crmeb": "eb_user",
				"Table_type":      "BASE TABLE",
			},
			want: "eb_user",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := firstDatabaseValue(test.row); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
