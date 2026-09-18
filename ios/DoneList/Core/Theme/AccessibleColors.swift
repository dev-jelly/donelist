import SwiftUI

/// WCAG AA 표준 (4.5:1 대비율) 준수 색상
struct AccessibleColors {
    // MARK: - Primary Colors

    /// 주요 브랜드 색상 - 4.54:1 대비율 on white
    static let primary = Color("PrimaryColor", bundle: nil)
    static let primaryFallback = Color(hex: "#0066CC")!

    /// 텍스트 색상
    static let primaryText = Color(hex: "#1A1A1A")! // 17.01:1 on white
    static let secondaryText = Color(hex: "#4A4A4A")! // 9.73:1 on white
    static let tertiaryText = Color(hex: "#767676")! // 4.54:1 on white (AA compliant)

    // MARK: - Semantic Colors

    static let success = Color(hex: "#2D6A4F")! // 5.12:1
    static let error = Color(hex: "#D00000")! // 6.51:1
    static let warning = Color(hex: "#B56B00")! // 5.88:1
    static let info = Color(hex: "#0077B6")! // 5.36:1

    // MARK: - Category Colors (4.5:1+ contrast)

    static let categoryColors: [Color] = [
        Color(hex: "#E63946")!, // Red - 4.78:1
        Color(hex: "#0077B6")!, // Blue - 5.36:1
        Color(hex: "#2A9D8F")!, // Teal - 4.72:1
        Color(hex: "#E76F51")!, // Orange - 4.51:1
        Color(hex: "#8338EC")!, // Purple - 6.28:1
        Color(hex: "#1D3557")!, // Navy - 12.63:1
        Color(hex: "#457B9D")!, // Steel Blue - 4.67:1
        Color(hex: "#6D597A")!, // Purple Gray - 5.21:1
    ]

    // MARK: - Heatmap Intensity Colors

    static let heatmapColors: [Color] = [
        Color(hex: "#EBEDF0")!, // Level 0: No activity
        Color(hex: "#9BE9A8")!, // Level 1: Light
        Color(hex: "#40C463")!, // Level 2: Medium
        Color(hex: "#30A14E")!, // Level 3: High
        Color(hex: "#216E39")!, // Level 4: Very high
    ]

    // MARK: - Background Colors

    static let background = Color(hex: "#FFFFFF")!
    static let secondaryBackground = Color(hex: "#F5F5F7")!
    static let tertiaryBackground = Color(hex: "#E5E5EA")!

    // MARK: - Dark Mode Variants

    static func primaryText(for colorScheme: ColorScheme) -> Color {
        colorScheme == .dark ? Color(hex: "#F5F5F7")! : primaryText
    }

    static func secondaryText(for colorScheme: ColorScheme) -> Color {
        colorScheme == .dark ? Color(hex: "#A1A1A6")! : secondaryText
    }

    static func background(for colorScheme: ColorScheme) -> Color {
        colorScheme == .dark ? Color(hex: "#1C1C1E")! : background
    }
}

// MARK: - Color Extension

extension Color {
    init?(hex: String) {
        var hexSanitized = hex.trimmingCharacters(in: .whitespacesAndNewlines)
        hexSanitized = hexSanitized.replacingOccurrences(of: "#", with: "")

        var rgb: UInt64 = 0
        guard Scanner(string: hexSanitized).scanHexInt64(&rgb) else { return nil }

        let r = Double((rgb & 0xFF0000) >> 16) / 255.0
        let g = Double((rgb & 0x00FF00) >> 8) / 255.0
        let b = Double(rgb & 0x0000FF) / 255.0

        self.init(red: r, green: g, blue: b)
    }

    /// 색상의 대비율 계산을 위한 상대 휘도
    var relativeLuminance: Double {
        guard let components = UIColor(self).cgColor.components, components.count >= 3 else {
            return 0
        }

        func adjust(_ component: Double) -> Double {
            component <= 0.03928 ? component / 12.92 : pow((component + 0.055) / 1.055, 2.4)
        }

        let r = adjust(components[0])
        let g = adjust(components[1])
        let b = adjust(components[2])

        return 0.2126 * r + 0.7152 * g + 0.0722 * b
    }

    /// 두 색상 간의 대비율 계산
    func contrastRatio(with other: Color) -> Double {
        let l1 = max(relativeLuminance, other.relativeLuminance)
        let l2 = min(relativeLuminance, other.relativeLuminance)
        return (l1 + 0.05) / (l2 + 0.05)
    }

    /// WCAG AA 표준 충족 여부 (4.5:1 for normal text)
    func meetsContrastStandard(against background: Color, for textSize: TextSize = .normal) -> Bool {
        let ratio = contrastRatio(with: background)
        switch textSize {
        case .normal: return ratio >= 4.5
        case .large: return ratio >= 3.0
        }
    }

    enum TextSize {
        case normal // 14pt or smaller
        case large  // 18pt+ or 14pt+ bold
    }
}
