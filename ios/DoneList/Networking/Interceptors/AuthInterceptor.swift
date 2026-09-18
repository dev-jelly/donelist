import Foundation

protocol RequestInterceptor: Sendable {
    func intercept(_ request: inout URLRequest) async throws
}

actor AuthInterceptor: RequestInterceptor {
    private let tokenManager: TokenManager
    private let tokenRefresher: TokenRefresher?
    private let logger = Logger.shared

    private var isRefreshing = false
    private var pendingRequests: [CheckedContinuation<Void, Error>] = []

    init(tokenManager: TokenManager, tokenRefresher: TokenRefresher? = nil) {
        self.tokenManager = tokenManager
        self.tokenRefresher = tokenRefresher
    }

    func intercept(_ request: inout URLRequest) async throws {
        if await tokenManager.isAccessTokenExpired() {
            try await refreshTokenIfNeeded()
        }

        guard let accessToken = try await tokenManager.getAccessToken() else {
            throw AppError.unauthorized
        }

        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
    }

    private func refreshTokenIfNeeded() async throws {
        if isRefreshing {
            try await withCheckedThrowingContinuation { continuation in
                pendingRequests.append(continuation)
            }
            return
        }

        isRefreshing = true
        defer {
            isRefreshing = false
            let pending = pendingRequests
            pendingRequests = []
            for continuation in pending {
                continuation.resume()
            }
        }

        guard let refresher = tokenRefresher else {
            throw AppError.tokenExpired
        }

        guard let refreshToken = try await tokenManager.getRefreshToken() else {
            throw AppError.tokenExpired
        }

        do {
            let newTokens = try await refresher.refresh(refreshToken: refreshToken)
            try await tokenManager.saveTokens(
                accessToken: newTokens.accessToken,
                refreshToken: newTokens.refreshToken,
                expiresAt: newTokens.expiresAt
            )
            logger.info("Token refreshed successfully", category: .auth)
        } catch {
            logger.error("Token refresh failed", error: error, category: .auth)
            throw AppError.tokenExpired
        }
    }
}

protocol TokenRefresher: Sendable {
    func refresh(refreshToken: String) async throws -> TokenPair
}

struct TokenPair: Codable, Sendable {
    let accessToken: String
    let refreshToken: String
    let expiresAt: Date
}
