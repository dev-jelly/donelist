import Foundation

struct Tag: Identifiable, Codable, Equatable, Sendable {
    let id: UUID
    let userId: UUID
    let name: String
    let slug: String
    let usageCount: Int
    let createdAt: Date
    let updatedAt: Date
}

struct CreateTagInput: Codable, Sendable {
    let name: String
}

struct TagAutocompleteResponse: Codable, Sendable {
    let tags: [Tag]
    let query: String
    let count: Int
}
