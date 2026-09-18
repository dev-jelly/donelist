import XCTest

final class LoginUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitesting", "--logout"]
        app.launch()
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - Login View Tests

    func testLoginViewElements() throws {
        // Logo and title
        XCTAssertTrue(app.staticTexts["DoneList"].waitForExistence(timeout: 5))

        // Text fields
        let emailField = app.textFields["이메일"]
        let passwordField = app.secureTextFields["비밀번호"]

        XCTAssertTrue(emailField.exists)
        XCTAssertTrue(passwordField.exists)

        // Login button
        let loginButton = app.buttons["로그인"]
        XCTAssertTrue(loginButton.exists)

        // Register link
        let registerLink = app.buttons.matching(NSPredicate(format: "label CONTAINS '회원가입'")).firstMatch
        XCTAssertTrue(registerLink.exists)
    }

    func testLoginWithEmptyFields() throws {
        let loginButton = app.buttons["로그인"]

        // Button should be disabled with empty fields
        XCTAssertFalse(loginButton.isEnabled)
    }

    func testLoginFieldsInput() throws {
        let emailField = app.textFields["이메일"]
        let passwordField = app.secureTextFields["비밀번호"]

        // Enter email
        emailField.tap()
        emailField.typeText("test@example.com")

        // Enter password
        passwordField.tap()
        passwordField.typeText("password123")

        // Login button should be enabled
        let loginButton = app.buttons["로그인"]
        XCTAssertTrue(loginButton.isEnabled)
    }

    func testNavigationToRegister() throws {
        let registerLink = app.buttons.matching(NSPredicate(format: "label CONTAINS '회원가입'")).firstMatch
        registerLink.tap()

        // Should show register view
        XCTAssertTrue(app.staticTexts["계정 만들기"].waitForExistence(timeout: 2))
    }

    // MARK: - Accessibility Tests

    func testLoginViewAccessibility() throws {
        // Email field should be accessible
        let emailField = app.textFields["이메일"]
        XCTAssertTrue(emailField.isAccessibilityElement)

        // Password field should be accessible
        let passwordField = app.secureTextFields["비밀번호"]
        XCTAssertTrue(passwordField.isAccessibilityElement)

        // Login button should be accessible
        let loginButton = app.buttons["로그인"]
        XCTAssertTrue(loginButton.isAccessibilityElement)
    }
}
