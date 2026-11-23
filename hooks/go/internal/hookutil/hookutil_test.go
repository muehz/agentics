package hookutil

import (
	"reflect"
	"testing"
)

func TestSplitCommaList(t *testing.T) {
	tests := []struct {
		value string
		want  []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{"a,b,", []string{"a", "b"}},
		{"a,,b", []string{"a", "", "b"}},
		{",a", []string{"", "a"}},
		{",", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := SplitCommaList(tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SplitCommaList(%q) = %#v, want %#v", tt.value, got, tt.want)
			}
		})
	}
}
