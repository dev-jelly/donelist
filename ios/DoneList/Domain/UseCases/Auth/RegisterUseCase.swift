import Foundation

final class RegisterUseCase: Sendable {
    private let authRepository: AuthRepositoryProtocol
    private let tokenManager: TokenManager
    private let logger = Logger.shared

    init(authRepository: AuthRepositoryProtocol, tokenManager: TokenManager) {
        self.authRepository = authRepository
        self.tokenManager = tokenManager
    }

    func execute(email: String, password: String, confirmPassword: String, displayName: String?) async throws -> AuthResult {
        try validateInputs(email: email, password: password, confirmPassword: confirmPassword, displayName: displayName)

        do {
            let result = try await authRepository.register(
                email: email.lowercased().trimmed,
                password: password,
                displayName: displayName?.trimmed
            )

            try await tokenManager.saveTokens(
                accessToken: result.tokenPair.accessToken,
                refreshToken: result.tokenPair.refreshToken,
                expiresAt: result.tokenPair.expiresAt
            )

            try await authRepository.cacheUser(result.user)

            logger.info("Registration successful for user: \(result.user.id)", category: .auth)
            return result
        } catch {
            logger.error("Registration failed", error: error, category: .auth)
            throw mapAuthError(error)
        }
    }

    private func validateInputs(email: String, password: String, confirmPassword: String, displayName: String?) throws {
        guard !email.isEmpty else { throw AppError.validationError("이메일을 입력해주세요") }
        guard email.isValidEmail else { throw AppError.validationError("올바른 이메일 형식이 아닙니다") }
        guard !password.isEmpty else { throw AppError.validationError("비밀번호를 입력해주세요") }
        guard password.count >= 8 else { throw AppError.validationError("비밀번호는 최소 8자 이상이어야 합니다") }
        guard password.count <= 72 else { throw AppError.validationError("비밀번호는 최대 72자까지 가능합니다") }
        guard password == confirmPassword else { throw AppError.validationError("비밀번호가 일치하지 않습니다") }

        let hasLetter = password.range(of: "[A-Za-z]", options: .regularExpression) != nil
        let hasNumber = password.range(of: "[0-9]", options: .regularExpression) != nil
        let hasSpecial = password.range(of: "[^A-Za-z0-9]", options: .regularExpression) != nil
        guard [hasLetter, hasNumber, hasSpecial].filter({ $0 }).count >= 2 else {
            throw AppError.validationError("비밀번호는 영문, 숫자, 특수문자 중 2가지 이상을 포함해야 합니다")
        }

        if let name = displayName, !name.isEmpty, name.count > 100 {
            throw AppError.validationError("이름은 최대 100자까지 가능합니다")
        }
    }

    private func mapAuthError(_ error: Error) -> AppError {
        if let appError = error as? AppError { return appError }
        let desc = error.localizedDescription
        if desc.contains("duplicate") || desc.contains("exists") {
            return .authenticationError("이미 등록된 이메일입니다")
        }
        if desc.contains("network") {
            return .networkError("네트워크 연결을 확인해주세요")
        }
        return .unknown("회원가입에 실패했습니다")
    }
}
