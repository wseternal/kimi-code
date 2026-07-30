// Package theme provides color and style definitions for the TUI.
package theme

// Color represents a terminal color.
type Color string

const (
	ColorPrimary    Color = "#7C3AED" // purple
	ColorSecondary  Color = "#06B6D4" // cyan
	ColorSuccess    Color = "#10B981" // green
	ColorWarning    Color = "#F59E0B" // amber
	ColorError      Color = "#EF4444" // red
	ColorMuted      Color = "#6B7280" // gray
	ColorBackground Color = "#1F2937" // dark gray
	ColorForeground Color = "#F9FAFB" // light gray
	ColorBorder     Color = "#374151" // medium gray
	ColorAccent     Color = "#8B5CF6" // light purple
)

// Style represents a text style.
type Style string

const (
	StyleBold      Style = "bold"
	StyleItalic    Style = "italic"
	StyleUnderline Style = "underline"
	StyleDim       Style = "dim"
)

// Theme defines the complete color scheme.
type Theme struct {
	Name       string `json:"name"`
	Primary    Color  `json:"primary"`
	Secondary  Color  `json:"secondary"`
	Success    Color  `json:"success"`
	Warning    Color  `json:"warning"`
	Error      Color  `json:"error"`
	Muted      Color  `json:"muted"`
	Background Color  `json:"background"`
	Foreground Color  `json:"foreground"`
	Border     Color  `json:"border"`
	Accent     Color  `json:"accent"`
}

// DefaultTheme returns the default dark theme.
func DefaultTheme() *Theme {
	return &Theme{
		Name:       "dark",
		Primary:    ColorPrimary,
		Secondary:  ColorSecondary,
		Success:    ColorSuccess,
		Warning:    ColorWarning,
		Error:      ColorError,
		Muted:      ColorMuted,
		Background: ColorBackground,
		Foreground: ColorForeground,
		Border:     ColorBorder,
		Accent:     ColorAccent,
	}
}

// LightTheme returns a light theme.
func LightTheme() *Theme {
	return &Theme{
		Name:       "light",
		Primary:    "#6D28D9",
		Secondary:  "#0891B2",
		Success:    "#059669",
		Warning:    "#D97706",
		Error:      "#DC2626",
		Muted:      "#9CA3AF",
		Background: "#FFFFFF",
		Foreground: "#111827",
		Border:     "#E5E7EB",
		Accent:     "#7C3AED",
	}
}
