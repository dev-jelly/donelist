import XCTest

final class DoneListUITests: XCTestCase {
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

    // MARK: - Launch Tests

    func testLaunch() throws {
        // App should launch and show either login or main tab view
        let loginView = app.staticTexts["DoneList"]
        let tabBar = app.tabBars.firstMatch

        XCTAssertTrue(loginView.exists || tabBar.exists, "App should show login or main view")
    }
}
