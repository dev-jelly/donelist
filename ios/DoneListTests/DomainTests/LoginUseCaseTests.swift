import XCTest
@testable import DoneList

final class LoginUseCaseTests: XCTestCase {
    var sut: LoginUseCase!
    var mockAuthRepository: MockAuthRepository!
    var mockTokenManager: MockTokenManager!

    override func setUp() {
        super.setUp()
        mockAuthRepository = MockAuthRepository()
        mockTokenManager = MockTokenManager()
        sut = LoginUseCase(authRepository: mockAuthRepository, tokenManager: mockTokenManager)
    }

    override func tearDown() {
        sut = nil
        mockAuthRepository = nil
        mockTokenManager = nil
        super.tearDown()
    }

    // MARK: - Validation Tests

    func test_login_withEmptyEmail_throwsValidationError() async {
        // Given
        let email = ""
        let password = "password123"

        // When/Then
        do {
            _ = try await sut.execute(email: email, password: password)
            XCTFail("Expected validation error")
        } catch let error as AppError {
            XCTAssertEqual(error, .validationError("이메일을 입력해주세요"))
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    func test_login_withInvalidEmail_throwsValidationError() async {
        // Given
        let email = "invalid-email"
        let password = "password123"

        // When/Then
        do {
            _ = try await sut.execute(email: email, password: password)
            XCTFail("Expected validation error")
        } catch let error as AppError {
            XCTAssertEqual(error, .validationError("올바른 이메일 형식이 아닙니다"))
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    func test_login_withEmptyPassword_throwsValidationError() async {
        // Given
        let email = "test@example.com"
        let password = ""

        // When/Then
        do {
            _ = try await sut.execute(email: email, password: password)
            XCTFail("Expected validation error")
        } catch let error as AppError {
            XCTAssertEqual(error, .validationError("비밀번호를 입력해주세요"))
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    // MARK: - Success Tests

    func test_login_withValidCredentials_returnsAuthResult() async throws {
        // Given
        let email = "test@example.com"
        let password = "password123"
        let expectedUser = User(
            id: UUID(),
            email: email,
            displayName: "Test User",
            avatarURL: nil,
            role: .user,
            subscriptionTier: .free,
            appMode: .personal,
            createdAt: Date(),
            updatedAt: Date()
        )
        mockAuthRepository.loginResult = .success(AuthResult(
            user: expectedUser,
            tokenPair: TokenPair(accessToken: "token", refreshToken: "refresh", expiresAt: Date().addingTimeInterval(3600))
        ))

        // When
        let result = try await sut.execute(email: email, password: password)

        // Then
        XCTAssertEqual(result.user.email, email)
        XCTAssertTrue(mockTokenManager.saveTokensCalled)
        XCTAssertTrue(mockAuthRepository.cacheUserCalled)
    }

    // MARK: - Failure Tests

    func test_login_withIncorrectCredentials_throwsAuthError() async {
        // Given
        let email = "test@example.com"
        let password = "wrongpassword"
        mockAuthRepository.loginResult = .failure(AppError.unauthorized)

        // When/Then
        do {
            _ = try await sut.execute(email: email, password: password)
            XCTFail("Expected authentication error")
        } catch let error as AppError {
            XCTAssertNotNil(error.errorDescription)
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }
}

// MARK: - Mocks

final class MockAuthRepository: AuthRepositoryProtocol, @unchecked Sendable {
    var loginResult: Result<AuthResult, Error> = .failure(AppError.unknown(""))
    var cacheUserCalled = false

    func register(email: String, password: String, displayName: String?) async throws -> AuthResult {
        fatalError("Not implemented")
    }

    func login(email: String, password: String) async throws -> AuthResult {
        switch loginResult {
        case .success(let result): return result
        case .failure(let error): throw error
        }
    }

    func refreshToken(_ refreshToken: String) async throws -> TokenPair {
        fatalError("Not implemented")
    }

    func logout(refreshToken: String) async throws {}

    func getCurrentUser() async throws -> User {
        fatalError("Not implemented")
    }

    func getCachedUser() async -> User? { nil }

    func cacheUser(_ user: User) async throws {
        cacheUserCalled = true
    }

    func clearCachedUser() async throws {}
}

final class MockTokenManager: TokenManager, @unchecked Sendable {
    var saveTokensCalled = false
    var storedAccessToken: String?
    var storedRefreshToken: String?
    var tokenExpired = false

    func saveAccessToken(_ token: String) async throws {
        storedAccessToken = token
    }

    func saveRefreshToken(_ token: String) async throws {
        storedRefreshToken = token
    }

    func getAccessToken() async throws -> String? {
        storedAccessToken
    }

    func getRefreshToken() async throws -> String? {
        storedRefreshToken
    }

    func saveTokens(accessToken: String, refreshToken: String, expiresAt: Date) async throws {
        saveTokensCalled = true
        storedAccessToken = accessToken
        storedRefreshToken = refreshToken
    }

    func isAccessTokenExpired() async -> Bool {
        tokenExpired
    }

    func getTokenExpiryDate() async -> Date? {
        nil
    }

    func clearTokens() async throws {
        storedAccessToken = nil
        storedRefreshToken = nil
    }
}
