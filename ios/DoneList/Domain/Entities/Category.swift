import Foundation
import SwiftUI

struct Category: Identifiable, Codable, Equatable, Sendable {
    let id: UUID
    let userId: UUID
    let name: String
    let color: String?
    let icon: String?
    let createdAt: Date
    let updatedAt: Date

    var displayColor: Color {
        guard let hex = color else { return .blue }
        return Color(hex: hex) ?? .blue
    }
}

struct CreateCategoryInput: Codable, Sendable {
    let name: String
    let color: String?
    let icon: String?
}

struct UpdateCategoryInput: Codable, Sendable {
    let name: String?
    let color: String?
    let icon: String?
}

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

    var hexString: String {
        guard let components = UIColor(self).cgColor.components, components.count >= 3 else {
            return "#000000"
        }
        let r = Int(components[0] * 255)
        let g = Int(components[1] * 255)
        let b = Int(components[2] * 255)
        return String(format: "#%02X%02X%02X", r, g, b)
    }
}
