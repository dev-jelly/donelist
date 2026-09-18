import XCTest

final class SettingsUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitesting", "--authenticated"]
        app.launch()

        // Navigate to Settings tab
        let tabBar = app.tabBars.firstMatch
        if tabBar.waitForExistence(timeout: 5) {
            tabBar.buttons["Settings"].tap()
        }
    }

    override func tearDownWithError() throws {
        app = nil
    }

    func testSettingsViewElements() throws {
        XCTAssertTrue(app.navigationBars["설정"].waitForExistence(timeout: 2) || app.staticTexts["설정"].exists)

        // Check for sections
        XCTAssertTrue(app.staticTexts["관리"].exists || app.cells.staticTexts["카테고리 관리"].exists)
    }

    func testNavigateToCategoryManagement() throws {
        let categoryCell = app.cells.staticTexts["카테고리 관리"]
        if categoryCell.waitForExistence(timeout: 2) {
            categoryCell.tap()
            XCTAssertTrue(app.navigationBars["카테고리 관리"].waitForExistence(timeout: 2))
        }
    }

    func testNavigateToTagManagement() throws {
        let tagCell = app.cells.staticTexts["태그 관리"]
        if tagCell.waitForExistence(timeout: 2) {
            tagCell.tap()
            XCTAssertTrue(app.navigationBars["태그 관리"].waitForExistence(timeout: 2))
        }
    }

    func testNavigateToNotificationSettings() throws {
        let notificationCell = app.cells.staticTexts["알림 설정"]
        if notificationCell.waitForExistence(timeout: 2) {
            notificationCell.tap()
            XCTAssertTrue(app.navigationBars["알림 설정"].waitForExistence(timeout: 2))
        }
    }

    func testLogoutButton() throws {
        let logoutButton = app.buttons["로그아웃"]
        XCTAssertTrue(logoutButton.waitForExistence(timeout: 2))
    }
}
