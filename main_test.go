package main

import (
	"reflect"
	"testing"
)

func TestParseColorTriplets(t *testing.T) {
	var matrix = []struct {
		input  string
		colors []RGBAColor
		err    bool
	}{
		{"aa55aa55", []RGBAColor{{170, 85, 170, 255}}, false},
		{"aa55aa55,000000", []RGBAColor{{170, 85, 170, 255}, {0, 0, 0, 255}}, false},
		{"#aa55aa55,#000000", []RGBAColor{{170, 85, 170, 255}, {0, 0, 0, 255}}, false},
		{"#aa55aa55#000000", []RGBAColor{{170, 85, 170, 255}, {0, 0, 0, 255}}, false},
		{"aa55aa55,#000000#808080", []RGBAColor{{170, 85, 170, 255}, {0, 0, 0, 255}, {128, 128, 128, 255}}, false},
		{"nothex", []RGBAColor{}, true},
	}

	for _, tt := range matrix {
		c, err := ParseColorTriplets(tt.input)
		if err != nil {
			if tt.err {
				continue
			}
			t.Errorf("unexpected error for %s: %s", tt.input, err)
			continue
		}

		if !reflect.DeepEqual(c, tt.colors) {
			t.Errorf("expected %v, got %v", tt.colors, c)
		}
	}
}
