import SwiftUI

// MARK: - Dynamic Type Support

extension Font {
    /// Dynamic Type을 지원하는 시스템 폰트
    static func scaled(_ style: TextStyle, design: Design = .default, weight: Weight = .regular) -> Font {
        .system(style, design: design, weight: weight)
    }

    /// 앱 전용 스케일 폰트
    enum AppFont {
        case largeTitle
        case title
        case headline
        case body
        case callout
        case subheadline
        case footnote
        case caption

        var font: Font {
            switch self {
            case .largeTitle: return .scaled(.largeTitle, weight: .bold)
            case .title: return .scaled(.title2, weight: .semibold)
            case .headline: return .scaled(.headline, weight: .semibold)
            case .body: return .scaled(.body)
            case .callout: return .scaled(.callout)
            case .subheadline: return .scaled(.subheadline)
            case .footnote: return .scaled(.footnote)
            case .caption: return .scaled(.caption)
            }
        }
    }
}

// MARK: - Scaled Metric Helpers

struct ScaledFontModifier: ViewModifier {
    @ScaledMetric private var size: CGFloat
    let weight: Font.Weight

    init(size: CGFloat, relativeTo textStyle: Font.TextStyle = .body, weight: Font.Weight = .regular) {
        self._size = ScaledMetric(wrappedValue: size, relativeTo: textStyle)
        self.weight = weight
    }

    func body(content: Content) -> some View {
        content.font(.system(size: size, weight: weight))
    }
}

extension View {
    /// 커스텀 크기 폰트에 Dynamic Type 적용
    func scaledFont(size: CGFloat, relativeTo textStyle: Font.TextStyle = .body, weight: Font.Weight = .regular) -> some View {
        modifier(ScaledFontModifier(size: size, relativeTo: textStyle, weight: weight))
    }
}

// MARK: - Scaled Spacing

struct ScaledSpacing {
    @ScaledMetric(relativeTo: .body) static var xs: CGFloat = 4
    @ScaledMetric(relativeTo: .body) static var sm: CGFloat = 8
    @ScaledMetric(relativeTo: .body) static var md: CGFloat = 16
    @ScaledMetric(relativeTo: .body) static var lg: CGFloat = 24
    @ScaledMetric(relativeTo: .body) static var xl: CGFloat = 32
}

// MARK: - Scaled Icon Size

struct ScaledIconSize {
    @ScaledMetric(relativeTo: .body) static var small: CGFloat = 16
    @ScaledMetric(relativeTo: .body) static var medium: CGFloat = 24
    @ScaledMetric(relativeTo: .body) static var large: CGFloat = 32
    @ScaledMetric(relativeTo: .body) static var xlarge: CGFloat = 48
}
