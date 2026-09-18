import SwiftUI

// MARK: - View Extensions for VoiceOver

extension View {
    /// VoiceOver 레이블, 힌트, 값을 한 번에 설정
    func accessibleElement(
        label: String,
        hint: String? = nil,
        value: String? = nil,
        traits: AccessibilityTraits = []
    ) -> some View {
        self
            .accessibilityLabel(label)
            .accessibilityHint(hint ?? "")
            .accessibilityValue(value ?? "")
            .accessibilityAddTraits(traits)
    }

    /// 버튼 접근성 설정
    func accessibleButton(_ label: String, hint: String? = nil) -> some View {
        accessibleElement(label: label, hint: hint, traits: .isButton)
    }

    /// 헤더 접근성 설정
    func accessibleHeader(_ label: String) -> some View {
        accessibleElement(label: label, traits: .isHeader)
    }

    /// 이미지 접근성 설정 (장식용 이미지는 숨김)
    func accessibleImage(_ label: String?, isDecorative: Bool = false) -> some View {
        if isDecorative {
            return AnyView(self.accessibilityHidden(true))
        } else {
            return AnyView(self.accessibilityLabel(label ?? ""))
        }
    }

    /// 선택 가능한 요소 접근성 설정
    func accessibleSelectable(_ label: String, isSelected: Bool, hint: String? = nil) -> some View {
        self
            .accessibilityLabel(label)
            .accessibilityHint(hint ?? (isSelected ? "선택됨" : "선택하려면 두 번 탭하세요"))
            .accessibilityAddTraits(isSelected ? .isSelected : [])
    }

    /// 그룹 요소로 결합
    func accessibilityGrouped() -> some View {
        self.accessibilityElement(children: .combine)
    }
}

// MARK: - Reduce Motion Support

struct ReduceMotionModifier: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let animation: Animation

    func body(content: Content) -> some View {
        content.animation(reduceMotion ? nil : animation, value: UUID())
    }
}

extension View {
    /// 움직임 줄이기 설정 시 애니메이션 비활성화
    func adaptiveAnimation(_ animation: Animation = .default) -> some View {
        modifier(ReduceMotionModifier(animation: animation))
    }

    /// 조건부 애니메이션 적용
    func conditionalAnimation<V: Equatable>(_ animation: Animation?, value: V) -> some View {
        self.modifier(ConditionalAnimationModifier(animation: animation, value: value))
    }
}

struct ConditionalAnimationModifier<V: Equatable>: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let animation: Animation?
    let value: V

    func body(content: Content) -> some View {
        content.animation(reduceMotion ? nil : animation, value: value)
    }
}

// MARK: - Reduce Transparency Support

struct ReduceTransparencyModifier: ViewModifier {
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    let opaqueBackground: Color
    let transparentBackground: Color

    func body(content: Content) -> some View {
        content.background(reduceTransparency ? opaqueBackground : transparentBackground)
    }
}

extension View {
    /// 투명도 줄이기 설정 지원
    func adaptiveBackground(opaque: Color, transparent: Color) -> some View {
        modifier(ReduceTransparencyModifier(opaqueBackground: opaque, transparentBackground: transparent))
    }
}

// MARK: - Bold Text Support

struct BoldTextModifier: ViewModifier {
    @Environment(\.legibilityWeight) private var legibilityWeight
    let normalWeight: Font.Weight
    let boldWeight: Font.Weight

    func body(content: Content) -> some View {
        content.fontWeight(legibilityWeight == .bold ? boldWeight : normalWeight)
    }
}

extension View {
    /// 굵은 텍스트 설정 지원
    func adaptiveFontWeight(normal: Font.Weight = .regular, bold: Font.Weight = .semibold) -> some View {
        modifier(BoldTextModifier(normalWeight: normal, boldWeight: bold))
    }
}

// MARK: - Accessibility Announcements

@MainActor
final class AccessibilityAnnouncer {
    static let shared = AccessibilityAnnouncer()

    private init() {}

    /// VoiceOver 사용자에게 메시지 알림
    func announce(_ message: String, priority: UIAccessibility.Notification = .announcement) {
        UIAccessibility.post(notification: priority, argument: message)
    }

    /// 스크린 변경 알림
    func announceScreenChange(_ message: String? = nil) {
        UIAccessibility.post(notification: .screenChanged, argument: message)
    }

    /// 레이아웃 변경 알림
    func announceLayoutChange(_ element: Any? = nil) {
        UIAccessibility.post(notification: .layoutChanged, argument: element)
    }
}

// MARK: - Accessibility Identifier Helper

extension View {
    /// 테스트를 위한 접근성 식별자 설정
    func testID(_ identifier: String) -> some View {
        self.accessibilityIdentifier(identifier)
    }
}
