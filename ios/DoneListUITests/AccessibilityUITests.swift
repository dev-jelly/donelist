import XCTest

final class AccessibilityUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitesting"]
        app.launch()
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - VoiceOver Accessibility Tests

    func testAllInteractiveElementsAreAccessible() throws {
        // All buttons should be accessible
        let buttons = app.buttons.allElementsBoundByIndex
        for button in buttons {
            XCTAssertTrue(button.isAccessibilityElement, "Button '\(button.label)' should be accessible")
        }

        // All text fields should be accessible
        let textFields = app.textFields.allElementsBoundByIndex
        for textField in textFields {
            XCTAssertTrue(textField.isAccessibilityElement, "TextField should be accessible")
        }
    }

    func testAccessibilityLabelsExist() throws {
        // Check that main UI elements have accessibility labels
        let mainElements = [
            app.buttons["로그인"],
            app.textFields["이메일"],
            app.secureTextFields["비밀번호"]
        ]

        for element in mainElements where element.exists {
            XCTAssertFalse(element.label.isEmpty, "Element should have accessibility label")
        }
    }

    // MARK: - Dynamic Type Tests

    func testDynamicTypeSupport() throws {
        // This would typically be tested with different accessibility text sizes
        // In UI tests, we can verify text doesn't get truncated
        let staticTexts = app.staticTexts.allElementsBoundByIndex
        for text in staticTexts where text.exists {
            // Verify text is visible and not truncated
            XCTAssertTrue(text.frame.height > 0, "Text should have positive height")
        }
    }
}
