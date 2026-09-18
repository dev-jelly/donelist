import Foundation

struct User: Identifiable, Codable, Equatable, Sendable {
    let id: UUID
    let email: String
    let displayName: String?
    let avatarURL: String?
    let role: UserRole
    let subscriptionTier: SubscriptionTier
    let appMode: AppMode
    let createdAt: Date
    let updatedAt: Date
}

enum UserRole: String, Codable, Sendable {
    case user
    case admin
    case superadmin
}

enum SubscriptionTier: String, Codable, Sendable {
    case free
    case premium
    case enterprise
}

enum AppMode: String, Codable, Sendable {
    case personal
    case team
}
