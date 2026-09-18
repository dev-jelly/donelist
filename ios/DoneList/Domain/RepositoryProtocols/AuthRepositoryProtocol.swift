import Foundation

protocol AuthRepositoryProtocol: Sendable {
    func register(email: String, password: String, displayName: String?) async throws -> AuthResult
    func login(email: String, password: String) async throws -> AuthResult
    func refreshToken(_ refreshToken: String) async throws -> TokenPair
    func logout(refreshToken: String) async throws
    func getCurrentUser() async throws -> User
    func getCachedUser() async -> User?
    func cacheUser(_ user: User) async throws
    func clearCachedUser() async throws
}
