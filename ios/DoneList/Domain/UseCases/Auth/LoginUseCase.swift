import Foundation

final class LoginUseCase: Sendable {
    private let authRepository: AuthRepositoryProtocol
    private let tokenManager: TokenManager
    private let logger = Logger.shared

    init(authRepository: AuthRepositoryProtocol, tokenManager: TokenManager) {
        self.authRepository = authRepository
        self.tokenManager = tokenManager
    }

    func execute(email: String, password: String) async throws -> AuthResult {
        guard !email.isEmpty else {
            throw AppError.validationError("이메일을 입력해주세요")
        }

        guard email.isValidEmail else {
            throw AppError.validationError("올바른 이메일 형식이 아닙니다")
        }

        guard !password.isEmpty else {
            throw AppError.validationError("비밀번호를 입력해주세요")
        }

        do {
            let result = try await authRepository.login(
                email: email.lowercased().trimmed,
                password: password
            )

            try await tokenManager.saveTokens(
                accessToken: result.tokenPair.accessToken,
                refreshToken: result.tokenPair.refreshToken,
                expiresAt: result.tokenPair.expiresAt
            )

            try await authRepository.cacheUser(result.user)

            logger.info("Login successful for user: \(result.user.id)", category: .auth)
            return result
        } catch {
            logger.error("Login failed", error: error, category: .auth)
            throw mapAuthError(error)
        }
    }

    private func mapAuthError(_ error: Error) -> AppError {
        if let appError = error as? AppError { return appError }
        let desc = error.localizedDescription
        if desc.contains("401") || desc.contains("invalid") {
            return .authenticationError("이메일 또는 비밀번호가 일치하지 않습니다")
        }
        if desc.contains("network") {
            return .networkError("네트워크 연결을 확인해주세요")
        }
        return .unknown("로그인에 실패했습니다")
    }
}
