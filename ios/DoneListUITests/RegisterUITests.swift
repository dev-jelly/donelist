import XCTest

final class RegisterUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitesting", "--logout"]
        app.launch()

        // Navigate to register
        let registerLink = app.buttons.matching(NSPredicate(format: "label CONTAINS '회원가입'")).firstMatch
        if registerLink.waitForExistence(timeout: 5) {
            registerLink.tap()
        }
    }

    override func tearDownWithError() throws {
        app = nil
    }

    func testRegisterViewElements() throws {
        XCTAssertTrue(app.staticTexts["계정 만들기"].waitForExistence(timeout: 2))

        let emailField = app.textFields["이메일"]
        let nameField = app.textFields["이름 (선택)"]
        let passwordField = app.secureTextFields["비밀번호"]
        let confirmPasswordField = app.secureTextFields["비밀번호 확인"]

        XCTAssertTrue(emailField.exists)
        XCTAssertTrue(nameField.exists)
        XCTAssertTrue(passwordField.exists)
        XCTAssertTrue(confirmPasswordField.exists)

        let registerButton = app.buttons["회원가입"]
        XCTAssertTrue(registerButton.exists)
    }

    func testPasswordRequirementsDisplay() throws {
        XCTAssertTrue(app.staticTexts["비밀번호 요구사항:"].waitForExistence(timeout: 2))
        XCTAssertTrue(app.staticTexts["8자 이상"].exists)
        XCTAssertTrue(app.staticTexts["영문"].exists)
        XCTAssertTrue(app.staticTexts["숫자"].exists)
    }

    func testRegisterFormValidation() throws {
        let emailField = app.textFields["이메일"]
        let passwordField = app.secureTextFields["비밀번호"]
        let confirmPasswordField = app.secureTextFields["비밀번호 확인"]
        let registerButton = app.buttons["회원가입"]

        // Empty form - button disabled
        XCTAssertFalse(registerButton.isEnabled)

        // Fill email
        emailField.tap()
        emailField.typeText("test@example.com")

        // Fill password
        passwordField.tap()
        passwordField.typeText("Password123!")

        // Fill confirm password
        confirmPasswordField.tap()
        confirmPasswordField.typeText("Password123!")

        // Button should be enabled
        XCTAssertTrue(registerButton.isEnabled)
    }
}
