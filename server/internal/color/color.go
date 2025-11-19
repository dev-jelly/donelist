package color

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// DefaultColors provides a curated palette of WCAG AA compliant colors
var DefaultColors = []string{
	"#2563EB", // Blue
	"#DC2626", // Red
	"#059669", // Green
	"#D97706", // Orange
	"#7C3AED", // Purple
	"#DB2777", // Pink
	"#0891B2", // Cyan
	"#65A30D", // Lime
}

// DefaultFallbackColor is used when validation fails
const DefaultFallbackColor = "#6B7280" // Gray

// RGB represents an RGB color
type RGB struct {
	R uint8
	G uint8
	B uint8
}

// HSL represents an HSL color
type HSL struct {
	H float64 // Hue: 0-360
	S float64 // Saturation: 0-100
	L float64 // Lightness: 0-100
}

// Color represents a validated color with multiple format support
type Color struct {
	Hex string
	RGB RGB
	HSL HSL
}

// ParseHexColor parses a hex color string and returns a Color struct
func ParseHexColor(hex string) (*Color, error) {
	// Normalize hex color
	hex = strings.TrimSpace(hex)
	if !strings.HasPrefix(hex, "#") {
		return nil, fmt.Errorf("hex color must start with #")
	}

	// Handle both 3-digit and 6-digit hex
	hexDigits := hex[1:]
	if len(hexDigits) == 3 {
		// Expand 3-digit hex to 6-digit
		hexDigits = string([]byte{
			hexDigits[0], hexDigits[0],
			hexDigits[1], hexDigits[1],
			hexDigits[2], hexDigits[2],
		})
	} else if len(hexDigits) != 6 {
		return nil, fmt.Errorf("hex color must be 3 or 6 digits")
	}

	// Validate hex characters
	matched, _ := regexp.MatchString("^[0-9A-Fa-f]{6}$", hexDigits)
	if !matched {
		return nil, fmt.Errorf("invalid hex color format")
	}

	// Parse RGB values
	r, _ := strconv.ParseUint(hexDigits[0:2], 16, 8)
	g, _ := strconv.ParseUint(hexDigits[2:4], 16, 8)
	b, _ := strconv.ParseUint(hexDigits[4:6], 16, 8)

	rgb := RGB{R: uint8(r), G: uint8(g), B: uint8(b)}
	hsl := rgbToHSL(rgb)

	return &Color{
		Hex: "#" + strings.ToUpper(hexDigits),
		RGB: rgb,
		HSL: hsl,
	}, nil
}

// ParseHSLColor parses an HSL color string (e.g., "hsl(120, 100%, 50%)")
func ParseHSLColor(hslStr string) (*Color, error) {
	hslStr = strings.TrimSpace(hslStr)

	// Match hsl(h, s%, l%) or hsl(h s% l%)
	re := regexp.MustCompile(`^hsl\(\s*(\d+(?:\.\d+)?)\s*,?\s*(\d+(?:\.\d+)?)%\s*,?\s*(\d+(?:\.\d+)?)%\s*\)$`)
	matches := re.FindStringSubmatch(hslStr)

	if matches == nil {
		return nil, fmt.Errorf("invalid HSL color format, expected hsl(h, s%%, l%%)")
	}

	h, _ := strconv.ParseFloat(matches[1], 64)
	s, _ := strconv.ParseFloat(matches[2], 64)
	l, _ := strconv.ParseFloat(matches[3], 64)

	// Validate ranges
	if h < 0 || h >= 360 {
		return nil, fmt.Errorf("hue must be between 0 and 359")
	}
	if s < 0 || s > 100 {
		return nil, fmt.Errorf("saturation must be between 0 and 100")
	}
	if l < 0 || l > 100 {
		return nil, fmt.Errorf("lightness must be between 0 and 100")
	}

	hsl := HSL{H: h, S: s, L: l}
	rgb := hslToRGB(hsl)
	hex := rgbToHex(rgb)

	return &Color{
		Hex: hex,
		RGB: rgb,
		HSL: hsl,
	}, nil
}

// ValidateColor validates a color string in either hex or HSL format
func ValidateColor(colorStr string) (*Color, error) {
	colorStr = strings.TrimSpace(colorStr)

	if strings.HasPrefix(colorStr, "#") {
		return ParseHexColor(colorStr)
	} else if strings.HasPrefix(strings.ToLower(colorStr), "hsl(") {
		return ParseHSLColor(colorStr)
	}

	return nil, fmt.Errorf("color must be in hex (#RRGGBB) or HSL (hsl(h, s%%, l%%)) format")
}

// CalculateContrastRatio calculates the WCAG contrast ratio between two colors
// Returns a ratio between 1 and 21
func CalculateContrastRatio(color1, color2 *Color) float64 {
	l1 := calculateRelativeLuminance(color1.RGB)
	l2 := calculateRelativeLuminance(color2.RGB)

	// Ensure l1 is the lighter color
	if l2 > l1 {
		l1, l2 = l2, l1
	}

	return (l1 + 0.05) / (l2 + 0.05)
}

// MeetsWCAGAA checks if the contrast ratio meets WCAG AA standards
// WCAG AA requires:
// - 4.5:1 for normal text
// - 3:1 for large text (18pt+ or 14pt+ bold)
func MeetsWCAGAA(foreground, background *Color, isLargeText bool) bool {
	ratio := CalculateContrastRatio(foreground, background)

	if isLargeText {
		return ratio >= 3.0
	}
	return ratio >= 4.5
}

// MeetsWCAGAAA checks if the contrast ratio meets WCAG AAA standards
// WCAG AAA requires:
// - 7:1 for normal text
// - 4.5:1 for large text
func MeetsWCAGAAA(foreground, background *Color, isLargeText bool) bool {
	ratio := CalculateContrastRatio(foreground, background)

	if isLargeText {
		return ratio >= 4.5
	}
	return ratio >= 7.0
}

// GetContrastingTextColor returns either black or white depending on which
// provides better contrast with the background
func GetContrastingTextColor(background *Color) string {
	white := &Color{Hex: "#FFFFFF", RGB: RGB{255, 255, 255}}
	black := &Color{Hex: "#000000", RGB: RGB{0, 0, 0}}

	whiteRatio := CalculateContrastRatio(white, background)
	blackRatio := CalculateContrastRatio(black, background)

	if whiteRatio > blackRatio {
		return "#FFFFFF"
	}
	return "#000000"
}

// IsSimilarColor checks if two colors are too similar (within a threshold)
// Returns true if colors are too similar (should be prevented for uniqueness)
func IsSimilarColor(color1, color2 *Color, threshold float64) bool {
	// Calculate Euclidean distance in RGB space
	rDiff := float64(color1.RGB.R) - float64(color2.RGB.R)
	gDiff := float64(color1.RGB.G) - float64(color2.RGB.G)
	bDiff := float64(color1.RGB.B) - float64(color2.RGB.B)

	distance := math.Sqrt(rDiff*rDiff + gDiff*gDiff + bDiff*bDiff)

	// Max distance in RGB space is sqrt(255^2 * 3) ≈ 441
	// Normalize to 0-1 range
	normalizedDistance := distance / 441.0

	return normalizedDistance < threshold
}

// GenerateRecommendedPalette generates a palette of colors that are visually
// distinct and meet WCAG AA standards against white background
func GenerateRecommendedPalette(existingColors []string, count int) []string {
	recommendations := []string{}
	whiteBackground, _ := ParseHexColor("#FFFFFF")

	// Filter existing colors to parsed format
	existingParsed := []*Color{}
	for _, c := range existingColors {
		if parsed, err := ValidateColor(c); err == nil {
			existingParsed = append(existingParsed, parsed)
		}
	}

	// Start with default colors that aren't too similar to existing ones
	for _, defaultColor := range DefaultColors {
		if len(recommendations) >= count {
			break
		}

		parsedDefault, _ := ParseHexColor(defaultColor)

		// Check if meets WCAG AA
		if !MeetsWCAGAA(parsedDefault, whiteBackground, false) {
			continue
		}

		// Check if too similar to existing colors
		tooSimilar := false
		for _, existing := range existingParsed {
			if IsSimilarColor(parsedDefault, existing, 0.15) {
				tooSimilar = true
				break
			}
		}

		// Check if too similar to already recommended colors
		for _, recommended := range recommendations {
			recParsed, _ := ParseHexColor(recommended)
			if IsSimilarColor(parsedDefault, recParsed, 0.15) {
				tooSimilar = true
				break
			}
		}

		if !tooSimilar {
			recommendations = append(recommendations, defaultColor)
		}
	}

	return recommendations
}

// Helper functions

func rgbToHSL(rgb RGB) HSL {
	r := float64(rgb.R) / 255.0
	g := float64(rgb.G) / 255.0
	b := float64(rgb.B) / 255.0

	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	delta := max - min

	h := 0.0
	s := 0.0
	l := (max + min) / 2.0

	if delta != 0 {
		if l < 0.5 {
			s = delta / (max + min)
		} else {
			s = delta / (2.0 - max - min)
		}

		switch max {
		case r:
			h = ((g - b) / delta)
			if g < b {
				h += 6.0
			}
		case g:
			h = ((b - r) / delta) + 2.0
		case b:
			h = ((r - g) / delta) + 4.0
		}
		h *= 60.0
	}

	return HSL{H: h, S: s * 100.0, L: l * 100.0}
}

func hslToRGB(hsl HSL) RGB {
	h := hsl.H
	s := hsl.S / 100.0
	l := hsl.L / 100.0

	c := (1.0 - math.Abs(2.0*l-1.0)) * s
	x := c * (1.0 - math.Abs(math.Mod(h/60.0, 2.0)-1.0))
	m := l - c/2.0

	var r, g, b float64

	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	return RGB{
		R: uint8(math.Round((r + m) * 255.0)),
		G: uint8(math.Round((g + m) * 255.0)),
		B: uint8(math.Round((b + m) * 255.0)),
	}
}

func rgbToHex(rgb RGB) string {
	return fmt.Sprintf("#%02X%02X%02X", rgb.R, rgb.G, rgb.B)
}

func calculateRelativeLuminance(rgb RGB) float64 {
	// Convert RGB to relative values
	r := float64(rgb.R) / 255.0
	g := float64(rgb.G) / 255.0
	b := float64(rgb.B) / 255.0

	// Apply gamma correction
	gammaCorrect := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}

	r = gammaCorrect(r)
	g = gammaCorrect(g)
	b = gammaCorrect(b)

	// Calculate relative luminance using WCAG formula
	return 0.2126*r + 0.7152*g + 0.0722*b
}
