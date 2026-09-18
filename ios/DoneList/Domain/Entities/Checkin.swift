import Foundation

struct Checkin: Identifiable, Codable, Equatable, Sendable {
    let id: UUID
    let userId: UUID
    let content: String
    let categoryId: UUID?
    let tags: [String]
    let checkinTime: Date
    let durationMinutes: Int?
    let createdAt: Date
    let updatedAt: Date
}

struct CreateCheckinInput: Codable, Sendable {
    let content: String
    let categoryId: UUID?
    let tags: [String]
    let checkinTime: Date?
    let durationMinutes: Int?
}
