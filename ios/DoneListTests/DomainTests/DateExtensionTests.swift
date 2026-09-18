import XCTest
@testable import DoneList

final class DateExtensionTests: XCTestCase {

    // MARK: - ISO8601 Tests

    func test_iso8601String_returnsCorrectFormat() {
        // Given
        let date = Date(timeIntervalSince1970: 0)

        // When
        let result = date.iso8601String

        // Then
        XCTAssertTrue(result.contains("1970-01-01"))
    }

    func test_fromISO8601_parsesCorrectly() {
        // Given
        let dateString = "2024-01-15T10:30:00.000Z"

        // When
        let date = Date.from(iso8601: dateString)

        // Then
        XCTAssertNotNil(date)
    }

    // MARK: - Day Comparison Tests

    func test_isToday_returnsTrueForToday() {
        // Given
        let today = Date()

        // Then
        XCTAssertTrue(today.isToday)
    }

    func test_isToday_returnsFalseForYesterday() {
        // Given
        let yesterday = Date().adding(days: -1)

        // Then
        XCTAssertFalse(yesterday.isToday)
    }

    func test_isYesterday_returnsTrueForYesterday() {
        // Given
        let yesterday = Date().adding(days: -1)

        // Then
        XCTAssertTrue(yesterday.isYesterday)
    }

    func test_isSameDay_returnsTrueForSameDay() {
        // Given
        let date1 = Date()
        let date2 = Date().adding(hours: 2)

        // Then
        XCTAssertTrue(date1.isSameDay(as: date2))
    }

    // MARK: - Date Manipulation Tests

    func test_startOfDay_returnsCorrectTime() {
        // Given
        let date = Date()

        // When
        let startOfDay = date.startOfDay

        // Then
        let components = Calendar.current.dateComponents([.hour, .minute, .second], from: startOfDay)
        XCTAssertEqual(components.hour, 0)
        XCTAssertEqual(components.minute, 0)
        XCTAssertEqual(components.second, 0)
    }

    func test_addingDays_correctlyAdds() {
        // Given
        let date = Date()

        // When
        let futureDate = date.adding(days: 5)

        // Then
        let daysDifference = Calendar.current.dateComponents([.day], from: date, to: futureDate).day
        XCTAssertEqual(daysDifference, 5)
    }

    // MARK: - Checkin Interval Tests

    func test_isValidCheckinInterval_at15Minutes_returnsTrue() {
        // Given
        let lastCheckin = Date()
        let now = lastCheckin.adding(minutes: 15)

        // When
        let result = now.isValidCheckinInterval(since: lastCheckin)

        // Then
        XCTAssertTrue(result.isValid)
        XCTAssertNil(result.remainingWaitTime)
    }

    func test_isValidCheckinInterval_at10Minutes_returnsFalse() {
        // Given
        let lastCheckin = Date()
        let now = lastCheckin.adding(minutes: 10)

        // When
        let result = now.isValidCheckinInterval(since: lastCheckin)

        // Then
        XCTAssertFalse(result.isValid)
        XCTAssertNotNil(result.remainingWaitTime)
    }

    func test_isValidCheckinInterval_at2Hours_returnsTrue() {
        // Given
        let lastCheckin = Date()
        let now = lastCheckin.adding(minutes: 120)

        // When
        let result = now.isValidCheckinInterval(since: lastCheckin)

        // Then
        XCTAssertTrue(result.isValid)
    }
}
