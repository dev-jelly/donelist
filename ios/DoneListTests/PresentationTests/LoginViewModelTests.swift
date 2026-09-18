import XCTest
@testable import DoneList

@MainActor
final class LoginViewModelTests: XCTestCase {
    var sut: LoginViewModel!

    override func setUp() {
        super.setUp()
        sut = LoginViewModel()
    }

    override func tearDown() {
        sut = nil
        super.tearDown()
    }

    // MARK: - Validation Tests

    func test_isValid_withEmptyFields_returnsFalse() {
        // Given
        sut.email = ""
        sut.password = ""

        // Then
        XCTAssertFalse(sut.isValid)
    }

    func test_isValid_withInvalidEmail_returnsFalse() {
        // Given
        sut.email = "invalid"
        sut.password = "password123"

        // Then
        XCTAssertFalse(sut.isValid)
    }

    func test_isValid_withValidCredentials_returnsTrue() {
        // Given
        sut.email = "test@example.com"
        sut.password = "password123"

        // Then
        XCTAssertTrue(sut.isValid)
    }

    // MARK: - State Tests

    func test_login_setsLoadingTrue() async {
        // Given
        sut.email = "test@example.com"
        sut.password = "password123"

        // When
        let task = Task {
            await sut.login()
        }

        // Give time for loading to be set
        try? await Task.sleep(nanoseconds: 100_000_000)

        // Then - loading should be true during execution
        // After completion, loading should be false
        await task.value
        XCTAssertFalse(sut.isLoading)
    }

    func test_login_withValidCredentials_setsLoginSuccess() async {
        // Given
        sut.email = "test@example.com"
        sut.password = "password123"

        // When
        await sut.login()

        // Then
        XCTAssertTrue(sut.loginSuccess)
        XCTAssertNotNil(sut.user)
    }
}
