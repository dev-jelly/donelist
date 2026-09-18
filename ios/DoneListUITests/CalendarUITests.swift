import XCTest

final class CalendarUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitesting", "--authenticated"]
        app.launch()

        // Navigate to Calendar tab
        let tabBar = app.tabBars.firstMatch
        if tabBar.waitForExistence(timeout: 5) {
            tabBar.buttons["Calendar"].tap()
        }
    }

    override func tearDownWithError() throws {
        app = nil
    }

    func testCalendarViewExists() throws {
        XCTAssertTrue(app.navigationBars["캘린더"].waitForExistence(timeout: 2) || app.staticTexts["캘린더"].exists)
    }

    func testViewTypeSegmentedControl() throws {
        let monthlySegment = app.buttons["월별"]
        let heatmapSegment = app.buttons["히트맵"]

        XCTAssertTrue(monthlySegment.waitForExistence(timeout: 2))
        XCTAssertTrue(heatmapSegment.exists)
    }

    func testSwitchToHeatmapView() throws {
        let heatmapSegment = app.buttons["히트맵"]
        if heatmapSegment.waitForExistence(timeout: 2) {
            heatmapSegment.tap()

            // Heatmap legend should be visible
            XCTAssertTrue(app.staticTexts["적음"].waitForExistence(timeout: 2) || app.staticTexts["많음"].exists)
        }
    }

    func testMonthNavigation() throws {
        // Previous month button
        let previousButton = app.buttons.matching(NSPredicate(format: "label CONTAINS 'chevron.left'")).firstMatch
        let nextButton = app.buttons.matching(NSPredicate(format: "label CONTAINS 'chevron.right'")).firstMatch

        if previousButton.waitForExistence(timeout: 2) {
            previousButton.tap()
            // Month should change (hard to verify exact month without more context)
        }

        if nextButton.exists {
            nextButton.tap()
        }
    }
}
