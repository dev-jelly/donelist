import Foundation

protocol TokenManager: Sendable {
    func saveAccessToken(_ token: String) async throws
    func saveRefreshToken(_ token: String) async throws
    func getAccessToken() async throws -> String?
    func getRefreshToken() async throws -> String?
    func saveTokens(accessToken: String, refreshToken: String, expiresAt: Date) async throws
    func isAccessTokenExpired() async -> Bool
    func getTokenExpiryDate() async -> Date?
    func clearTokens() async throws
}

struct TokenExpiry: Codable {
    let expiresAt: Date
    let savedAt: Date
}

actor TokenManagerImpl: TokenManager {
    private let keychain: KeychainService
    private let logger = Logger.shared

    private var cachedAccessToken: String?
    private var cachedRefreshToken: String?
    private var cachedExpiry: TokenExpiry?

    init(keychain: KeychainService = KeychainService()) {
        self.keychain = keychain
    }

    func saveAccessToken(_ token: String) async throws {
        try keychain.save(token, forKey: Constants.Keychain.accessTokenKey)
        cachedAccessToken = token
        logger.info("Access token saved", category: .auth)
    }

    func saveRefreshToken(_ token: String) async throws {
        try keychain.save(token, forKey: Constants.Keychain.refreshTokenKey)
        cachedRefreshToken = token
        logger.info("Refresh token saved", category: .auth)
    }

    func getAccessToken() async throws -> String? {
        if let cached = cachedAccessToken {
            return cached
        }

        do {
            let token = try keychain.getString(forKey: Constants.Keychain.accessTokenKey)
            cachedAccessToken = token
            return token
        } catch KeychainService.KeychainError.itemNotFound {
            return nil
        }
    }

    func getRefreshToken() async throws -> String? {
        if let cached = cachedRefreshToken {
            return cached
        }

        do {
            let token = try keychain.getString(forKey: Constants.Keychain.refreshTokenKey)
            cachedRefreshToken = token
            return token
        } catch KeychainService.KeychainError.itemNotFound {
            return nil
        }
    }

    func saveTokens(accessToken: String, refreshToken: String, expiresAt: Date) async throws {
        try await saveAccessToken(accessToken)
        try await saveRefreshToken(refreshToken)

        let expiry = TokenExpiry(expiresAt: expiresAt, savedAt: Date())
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let data = try encoder.encode(expiry)
        try keychain.save(data, forKey: Constants.Keychain.tokenExpiryKey)
        cachedExpiry = expiry

        logger.info("Token pair saved, expires at: \(expiresAt.iso8601String)", category: .auth)
    }

    func isAccessTokenExpired() async -> Bool {
        guard let expiry = await getTokenExpiry() else {
            return true
        }
        let buffer: TimeInterval = 5 * 60 // 5 minutes buffer
        return Date().addingTimeInterval(buffer) >= expiry.expiresAt
    }

    func getTokenExpiryDate() async -> Date? {
        await getTokenExpiry()?.expiresAt
    }

    private func getTokenExpiry() async -> TokenExpiry? {
        if let cached = cachedExpiry {
            return cached
        }

        do {
            let data = try keychain.getData(forKey: Constants.Keychain.tokenExpiryKey)
            let decoder = JSONDecoder()
            decoder.dateDecodingStrategy = .iso8601
            let expiry = try decoder.decode(TokenExpiry.self, from: data)
            cachedExpiry = expiry
            return expiry
        } catch {
            return nil
        }
    }

    func clearTokens() async throws {
        try keychain.delete(forKey: Constants.Keychain.accessTokenKey)
        try keychain.delete(forKey: Constants.Keychain.refreshTokenKey)
        try keychain.delete(forKey: Constants.Keychain.tokenExpiryKey)

        cachedAccessToken = nil
        cachedRefreshToken = nil
        cachedExpiry = nil

        logger.info("All tokens cleared", category: .auth)
    }
}
