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

func TestFactorialCache(t *testing.T) {
	var matrix = []struct {
		input  int
		output float64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 6},
		{4, 24},
		{18, 6402373705728000},
		{19, 121645100408832000},
	}

	fc := NewFactorialCache()

	for _, tt := range matrix {
		result := fc.Fact(tt.input)
		if result != tt.output {
			t.Errorf("expected %0.0f, got %0.0f", tt.output, result)
		}
	}
}
