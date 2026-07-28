package response

import "testing"

func TestParsePage(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int32
		wantErr bool
	}{
		{name: "default", raw: "", want: 1},
		{name: "valid", raw: "2", want: 2},
		{name: "invalid text", raw: "abc", wantErr: true},
		{name: "invalid zero", raw: "0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePage(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected page: got %d want %d", got, tt.want)
			}
		})
	}
}

func TestParsePageSize(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int32
		wantErr bool
	}{
		{name: "default", raw: "", want: 10},
		{name: "valid", raw: "20", want: 20},
		{name: "caps large value", raw: "200", want: 100},
		{name: "invalid text", raw: "abc", wantErr: true},
		{name: "invalid negative", raw: "-1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePageSize(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected page size: got %d want %d", got, tt.want)
			}
		})
	}
}
