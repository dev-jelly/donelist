package color

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		name    string
		hex     string
		wantErr bool
		wantHex string
		wantRGB RGB
	}{
		{
			name:    "Valid 6-digit hex",
			hex:     "#FF5733",
			wantErr: false,
			wantHex: "#FF5733",
			wantRGB: RGB{R: 255, G: 87, B: 51},
		},
		{
			name:    "Valid 6-digit hex lowercase",
			hex:     "#ff5733",
			wantErr: false,
			wantHex: "#FF5733",
			wantRGB: RGB{R: 255, G: 87, B: 51},
		},
		{
			name:    "Valid 3-digit hex",
			hex:     "#F57",
			wantErr: false,
			wantHex: "#FF5577",
			wantRGB: RGB{R: 255, G: 85, B: 119},
		},
		{
			name:    "Missing hash",
			hex:     "FF5733",
			wantErr: true,
		},
		{
			name:    "Too long",
			hex:     "#FF57331",
			wantErr: true,
		},
		{
			name:    "Invalid characters",
			hex:     "#GG5733",
			wantErr: true,
		},
		{
			name:    "Too short",
			hex:     "#FF",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color, err := ParseHexColor(tt.hex)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, color)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, color)
				assert.Equal(t, tt.wantHex, color.Hex)
				assert.Equal(t, tt.wantRGB, color.RGB)
			}
		})
	}
}

func TestParseHSLColor(t *testing.T) {
	tests := []struct {
		name    string
		hsl     string
		wantErr bool
		wantHex string
	}{
		{
			name:    "Valid HSL with commas",
			hsl:     "hsl(120, 100%, 50%)",
			wantErr: false,
			wantHex: "#00FF00",
		},
		{
			name:    "Valid HSL without commas",
			hsl:     "hsl(120 100% 50%)",
			wantErr: false,
			wantHex: "#00FF00",
		},
		{
			name:    "Valid HSL red",
			hsl:     "hsl(0, 100%, 50%)",
			wantErr: false,
			wantHex: "#FF0000",
		},
		{
			name:    "Valid HSL blue",
			hsl:     "hsl(240, 100%, 50%)",
			wantErr: false,
			wantHex: "#0000FF",
		},
		{
			name:    "Valid HSL gray",
			hsl:     "hsl(0, 0%, 50%)",
			wantErr: false,
			wantHex: "#808080",
		},
		{
			name:    "Invalid format",
			hsl:     "hsl(120, 100, 50)",
			wantErr: true,
		},
		{
			name:    "Hue out of range",
			hsl:     "hsl(400, 100%, 50%)",
			wantErr: true,
		},
		{
			name:    "Saturation out of range",
			hsl:     "hsl(120, 150%, 50%)",
			wantErr: true,
		},
		{
			name:    "Lightness out of range",
			hsl:     "hsl(120, 100%, 150%)",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color, err := ParseHSLColor(tt.hsl)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, color)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, color)
				assert.Equal(t, tt.wantHex, color.Hex)
			}
		})
	}
}

func TestValidateColor(t *testing.T) {
	tests := []struct {
		name    string
		color   string
		wantErr bool
	}{
		{
			name:    "Valid hex color",
			color:   "#FF5733",
			wantErr: false,
		},
		{
			name:    "Valid HSL color",
			color:   "hsl(120, 100%, 50%)",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			color:   "rgb(255, 87, 51)",
			wantErr: true,
		},
		{
			name:    "Empty string",
			color:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color, err := ValidateColor(tt.color)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, color)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, color)
			}
		})
	}
}

func TestCalculateContrastRatio(t *testing.T) {
	white, _ := ParseHexColor("#FFFFFF")
	black, _ := ParseHexColor("#000000")
	gray, _ := ParseHexColor("#808080")

	tests := []struct {
		name           string
		color1         *Color
		color2         *Color
		minRatio       float64
		maxRatio       float64
		description    string
	}{
		{
			name:        "Black on white - maximum contrast",
			color1:      black,
			color2:      white,
			minRatio:    20.9,
			maxRatio:    21.1,
			description: "Should be 21:1",
		},
		{
			name:        "White on white - minimum contrast",
			color1:      white,
			color2:      white,
			minRatio:    0.9,
			maxRatio:    1.1,
			description: "Should be 1:1",
		},
		{
			name:        "Gray on white",
			color1:      gray,
			color2:      white,
			minRatio:    3.0,
			maxRatio:    4.0,
			description: "Should be approximately 3.5:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := CalculateContrastRatio(tt.color1, tt.color2)
			assert.GreaterOrEqual(t, ratio, tt.minRatio, tt.description)
			assert.LessOrEqual(t, ratio, tt.maxRatio, tt.description)
		})
	}
}

func TestMeetsWCAGAA(t *testing.T) {
	white, _ := ParseHexColor("#FFFFFF")
	black, _ := ParseHexColor("#000000")
	darkGray, _ := ParseHexColor("#595959")
	veryLightGray, _ := ParseHexColor("#949494") // This should have ~3:1 ratio for large text

	tests := []struct {
		name        string
		foreground  *Color
		background  *Color
		isLargeText bool
		want        bool
	}{
		{
			name:        "Black on white - normal text",
			foreground:  black,
			background:  white,
			isLargeText: false,
			want:        true,
		},
		{
			name:        "Dark gray on white - normal text",
			foreground:  darkGray,
			background:  white,
			isLargeText: false,
			want:        true,
		},
		{
			name:        "Very light gray on white - normal text",
			foreground:  veryLightGray,
			background:  white,
			isLargeText: false,
			want:        false,
		},
		{
			name:        "Very light gray on white - large text",
			foreground:  veryLightGray,
			background:  white,
			isLargeText: true,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MeetsWCAGAA(tt.foreground, tt.background, tt.isLargeText)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestGetContrastingTextColor(t *testing.T) {
	tests := []struct {
		name       string
		background string
		want       string
	}{
		{
			name:       "Dark background needs white text",
			background: "#000000",
			want:       "#FFFFFF",
		},
		{
			name:       "Light background needs black text",
			background: "#FFFFFF",
			want:       "#000000",
		},
		{
			name:       "Blue background needs white text",
			background: "#2563EB",
			want:       "#FFFFFF",
		},
		{
			name:       "Light blue background needs black text",
			background: "#BFDBFE",
			want:       "#000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg, _ := ParseHexColor(tt.background)
			result := GetContrastingTextColor(bg)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestIsSimilarColor(t *testing.T) {
	red1, _ := ParseHexColor("#FF0000")
	red2, _ := ParseHexColor("#FF1111")
	blue, _ := ParseHexColor("#0000FF")

	tests := []struct {
		name      string
		color1    *Color
		color2    *Color
		threshold float64
		want      bool
	}{
		{
			name:      "Very similar reds",
			color1:    red1,
			color2:    red2,
			threshold: 0.1,
			want:      true,
		},
		{
			name:      "Very different colors",
			color1:    red1,
			color2:    blue,
			threshold: 0.1,
			want:      false,
		},
		{
			name:      "Same color",
			color1:    red1,
			color2:    red1,
			threshold: 0.1,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimilarColor(tt.color1, tt.color2, tt.threshold)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestGenerateRecommendedPalette(t *testing.T) {
	t.Run("Generate recommendations with no existing colors", func(t *testing.T) {
		recommendations := GenerateRecommendedPalette([]string{}, 5)
		assert.GreaterOrEqual(t, len(recommendations), 1)
		assert.LessOrEqual(t, len(recommendations), 5)

		// All recommendations should be valid colors
		for _, rec := range recommendations {
			_, err := ParseHexColor(rec)
			assert.NoError(t, err)
		}
	})

	t.Run("Generate recommendations avoiding existing colors", func(t *testing.T) {
		existing := []string{"#2563EB", "#DC2626"}
		recommendations := GenerateRecommendedPalette(existing, 3)

		// Should not include colors too similar to existing ones
		for _, rec := range recommendations {
			for _, exist := range existing {
				recColor, _ := ParseHexColor(rec)
				existColor, _ := ParseHexColor(exist)
				isSimilar := IsSimilarColor(recColor, existColor, 0.15)
				assert.False(t, isSimilar, "Recommendation should not be similar to existing color")
			}
		}
	})

	t.Run("All recommendations meet WCAG AA", func(t *testing.T) {
		recommendations := GenerateRecommendedPalette([]string{}, 5)
		white, _ := ParseHexColor("#FFFFFF")

		for _, rec := range recommendations {
			recColor, _ := ParseHexColor(rec)
			meetsStandard := MeetsWCAGAA(recColor, white, false)
			assert.True(t, meetsStandard, "Recommendation should meet WCAG AA")
		}
	})
}

func TestRGBToHSLConversion(t *testing.T) {
	tests := []struct {
		name string
		rgb  RGB
		hsl  HSL
	}{
		{
			name: "Red",
			rgb:  RGB{R: 255, G: 0, B: 0},
			hsl:  HSL{H: 0, S: 100, L: 50},
		},
		{
			name: "Green",
			rgb:  RGB{R: 0, G: 255, B: 0},
			hsl:  HSL{H: 120, S: 100, L: 50},
		},
		{
			name: "Blue",
			rgb:  RGB{R: 0, G: 0, B: 255},
			hsl:  HSL{H: 240, S: 100, L: 50},
		},
		{
			name: "White",
			rgb:  RGB{R: 255, G: 255, B: 255},
			hsl:  HSL{H: 0, S: 0, L: 100},
		},
		{
			name: "Black",
			rgb:  RGB{R: 0, G: 0, B: 0},
			hsl:  HSL{H: 0, S: 0, L: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rgbToHSL(tt.rgb)

			// Allow small floating point differences
			assert.InDelta(t, tt.hsl.H, result.H, 1.0)
			assert.InDelta(t, tt.hsl.S, result.S, 1.0)
			assert.InDelta(t, tt.hsl.L, result.L, 1.0)
		})
	}
}

func TestHSLToRGBConversion(t *testing.T) {
	tests := []struct {
		name string
		hsl  HSL
		rgb  RGB
	}{
		{
			name: "Red",
			hsl:  HSL{H: 0, S: 100, L: 50},
			rgb:  RGB{R: 255, G: 0, B: 0},
		},
		{
			name: "Green",
			hsl:  HSL{H: 120, S: 100, L: 50},
			rgb:  RGB{R: 0, G: 255, B: 0},
		},
		{
			name: "Blue",
			hsl:  HSL{H: 240, S: 100, L: 50},
			rgb:  RGB{R: 0, G: 0, B: 255},
		},
		{
			name: "White",
			hsl:  HSL{H: 0, S: 0, L: 100},
			rgb:  RGB{R: 255, G: 255, B: 255},
		},
		{
			name: "Black",
			hsl:  HSL{H: 0, S: 0, L: 0},
			rgb:  RGB{R: 0, G: 0, B: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hslToRGB(tt.hsl)

			// Allow small rounding differences
			assert.InDelta(t, tt.rgb.R, result.R, 1.0)
			assert.InDelta(t, tt.rgb.G, result.G, 1.0)
			assert.InDelta(t, tt.rgb.B, result.B, 1.0)
		})
	}
}

func TestRGBToHexConversion(t *testing.T) {
	tests := []struct {
		name string
		rgb  RGB
		hex  string
	}{
		{
			name: "Red",
			rgb:  RGB{R: 255, G: 0, B: 0},
			hex:  "#FF0000",
		},
		{
			name: "Green",
			rgb:  RGB{R: 0, G: 255, B: 0},
			hex:  "#00FF00",
		},
		{
			name: "Blue",
			rgb:  RGB{R: 0, G: 0, B: 255},
			hex:  "#0000FF",
		},
		{
			name: "Custom color",
			rgb:  RGB{R: 255, G: 87, B: 51},
			hex:  "#FF5733",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rgbToHex(tt.rgb)
			assert.Equal(t, tt.hex, result)
		})
	}
}

func TestCalculateRelativeLuminance(t *testing.T) {
	tests := []struct {
		name     string
		rgb      RGB
		expected float64
	}{
		{
			name:     "Black has zero luminance",
			rgb:      RGB{R: 0, G: 0, B: 0},
			expected: 0.0,
		},
		{
			name:     "White has maximum luminance",
			rgb:      RGB{R: 255, G: 255, B: 255},
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateRelativeLuminance(tt.rgb)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestRoundTripConversion(t *testing.T) {
	// Test that converting hex -> RGB -> HSL -> RGB -> hex preserves the color
	original := "#FF5733"
	color1, err := ParseHexColor(original)
	assert.NoError(t, err)

	// Convert to HSL and back to RGB
	rgb := hslToRGB(color1.HSL)
	hex := rgbToHex(rgb)

	// Should be very close to original (may have minor rounding differences)
	color2, err := ParseHexColor(hex)
	assert.NoError(t, err)

	assert.InDelta(t, color1.RGB.R, color2.RGB.R, 2.0)
	assert.InDelta(t, color1.RGB.G, color2.RGB.G, 2.0)
	assert.InDelta(t, color1.RGB.B, color2.RGB.B, 2.0)
}

func BenchmarkParseHexColor(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseHexColor("#FF5733")
	}
}

func BenchmarkParseHSLColor(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseHSLColor("hsl(120, 100%, 50%)")
	}
}

func BenchmarkCalculateContrastRatio(b *testing.B) {
	color1, _ := ParseHexColor("#FF5733")
	color2, _ := ParseHexColor("#FFFFFF")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateContrastRatio(color1, color2)
	}
}

func BenchmarkGenerateRecommendedPalette(b *testing.B) {
	existing := []string{"#2563EB", "#DC2626", "#059669"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateRecommendedPalette(existing, 5)
	}
}
