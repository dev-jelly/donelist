import XCTest
@testable import DoneList

final class StringExtensionTests: XCTestCase {

    // MARK: - Email Validation Tests

    func test_isValidEmail_withValidEmail_returnsTrue() {
        XCTAssertTrue("test@example.com".isValidEmail)
        XCTAssertTrue("user.name@domain.co.kr".isValidEmail)
        XCTAssertTrue("user+tag@example.com".isValidEmail)
    }

    func test_isValidEmail_withInvalidEmail_returnsFalse() {
        XCTAssertFalse("invalid".isValidEmail)
        XCTAssertFalse("@example.com".isValidEmail)
        XCTAssertFalse("test@".isValidEmail)
        XCTAssertFalse("test@.com".isValidEmail)
        XCTAssertFalse("".isValidEmail)
    }

    // MARK: - Password Validation Tests

    func test_isStrongPassword_withStrongPassword_returnsTrue() {
        XCTAssertTrue("Abc123!@#".isStrongPassword)
        XCTAssertTrue("StrongP@ss1".isStrongPassword)
    }

    func test_isStrongPassword_withWeakPassword_returnsFalse() {
        XCTAssertFalse("short".isStrongPassword)
        XCTAssertFalse("onlylowercase".isStrongPassword)
        XCTAssertFalse("ONLYUPPERCASE".isStrongPassword)
        XCTAssertFalse("12345678".isStrongPassword)
    }

    // MARK: - String Utility Tests

    func test_isBlank_withWhitespace_returnsTrue() {
        XCTAssertTrue("".isBlank)
        XCTAssertTrue("   ".isBlank)
        XCTAssertTrue("\n\t".isBlank)
    }

    func test_isBlank_withContent_returnsFalse() {
        XCTAssertFalse("hello".isBlank)
        XCTAssertFalse(" hello ".isBlank)
    }

    func test_trimmed_removesWhitespace() {
        XCTAssertEqual("  hello  ".trimmed, "hello")
        XCTAssertEqual("\n\thello\n\t".trimmed, "hello")
    }

    func test_truncated_shorterString_returnsOriginal() {
        let string = "short"
        XCTAssertEqual(string.truncated(to: 10), "short")
    }

    func test_truncated_longerString_truncates() {
        let string = "this is a long string"
        XCTAssertEqual(string.truncated(to: 10), "this is a ...")
    }

    // MARK: - Base64 Tests

    func test_base64Encoded_encodesCorrectly() {
        let original = "Hello, World!"
        let encoded = original.base64Encoded
        XCTAssertEqual(encoded, "SGVsbG8sIFdvcmxkIQ==")
    }

    func test_base64Decoded_decodesCorrectly() {
        let encoded = "SGVsbG8sIFdvcmxkIQ=="
        let decoded = encoded.base64Decoded
        XCTAssertEqual(decoded, "Hello, World!")
    }
}
