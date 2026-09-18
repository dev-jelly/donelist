import Foundation

struct AuthResult: Codable, Equatable, Sendable {
    let user: User
    let tokenPair: TokenPair
}
