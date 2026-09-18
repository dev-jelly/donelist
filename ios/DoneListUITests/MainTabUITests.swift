import XCTest

final class MainTabUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitesting", "--authenticated"]
        app.launch()
    }

    override func tearDownWithError() throws {
        app = nil
    }

    func testTabBarExists() throws {
        let tabBar = app.tabBars.firstMatch
        XCTAssertTrue(tabBar.waitForExistence(timeout: 5))
    }

    func testAllTabsExist() throws {
        let tabBar = app.tabBars.firstMatch
        XCTAssertTrue(tabBar.waitForExistence(timeout: 5))

        XCTAssertTrue(tabBar.buttons["Today"].exists)
        XCTAssertTrue(tabBar.buttons["Timeline"].exists)
        XCTAssertTrue(tabBar.buttons["Calendar"].exists)
        XCTAssertTrue(tabBar.buttons["Statistics"].exists)
        XCTAssertTrue(tabBar.buttons["Settings"].exists)
    }

    func testTabNavigation() throws {
        let tabBar = app.tabBars.firstMatch
        XCTAssertTrue(tabBar.waitForExistence(timeout: 5))

        // Navigate to Timeline
        tabBar.buttons["Timeline"].tap()
        XCTAssertTrue(app.navigationBars["타임라인"].waitForExistence(timeout: 2) || app.staticTexts["타임라인"].exists)

        // Navigate to Calendar
        tabBar.buttons["Calendar"].tap()
        XCTAssertTrue(app.navigationBars["캘린더"].waitForExistence(timeout: 2) || app.staticTexts["캘린더"].exists)

        // Navigate to Statistics
        tabBar.buttons["Statistics"].tap()
        XCTAssertTrue(app.navigationBars["통계"].waitForExistence(timeout: 2) || app.staticTexts["통계"].exists)

        // Navigate to Settings
        tabBar.buttons["Settings"].tap()
        XCTAssertTrue(app.navigationBars["설정"].waitForExistence(timeout: 2) || app.staticTexts["설정"].exists)
    }
}
