import XCTest
@testable import DoneList

final class KeychainServiceTests: XCTestCase {
    var sut: KeychainService!
    let testService = "com.donelist.test"

    override func setUp() {
        super.setUp()
        sut = KeychainService(service: testService)
    }

    override func tearDown() {
        try? sut.deleteAll()
        sut = nil
        super.tearDown()
    }

    func test_saveAndGetString_succeeds() throws {
        // Given
        let key = "testKey"
        let value = "testValue"

        // When
        try sut.save(value, forKey: key)
        let retrieved = try sut.getString(forKey: key)

        // Then
        XCTAssertEqual(retrieved, value)
    }

    func test_saveAndGetData_succeeds() throws {
        // Given
        let key = "dataKey"
        let value = "testData".data(using: .utf8)!

        // When
        try sut.save(value, forKey: key)
        let retrieved = try sut.getData(forKey: key)

        // Then
        XCTAssertEqual(retrieved, value)
    }

    func test_getString_nonExistentKey_throwsItemNotFound() {
        // Given
        let key = "nonExistentKey"

        // When/Then
        XCTAssertThrowsError(try sut.getString(forKey: key)) { error in
            guard case KeychainService.KeychainError.itemNotFound = error else {
                XCTFail("Expected itemNotFound error")
                return
            }
        }
    }

    func test_delete_removesItem() throws {
        // Given
        let key = "deleteKey"
        try sut.save("value", forKey: key)

        // When
        try sut.delete(forKey: key)

        // Then
        XCTAssertThrowsError(try sut.getString(forKey: key))
    }

    func test_save_updatesExistingValue() throws {
        // Given
        let key = "updateKey"
        try sut.save("original", forKey: key)

        // When
        try sut.save("updated", forKey: key)
        let retrieved = try sut.getString(forKey: key)

        // Then
        XCTAssertEqual(retrieved, "updated")
    }
}
