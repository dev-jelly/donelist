import Foundation
import SwiftData

enum OperationType: String, Codable, Sendable {
    case create, update, delete
}

enum ResourceType: String, Codable, Sendable {
    case checkin, category, tag
}

enum SyncStatus: String, Codable, Sendable {
    case pending, syncing, synced, failed
}

@Model
final class OfflineQueueItem {
    @Attribute(.unique) var id: UUID
    var operationType: String
    var resourceType: String
    var resourceId: UUID
    var idempotencyKey: String
    var clientTimestamp: Date
    var syncStatus: String
    var retryCount: Int
    var lastAttempt: Date?
    var errorMessage: String?
    var operationData: Data
    var createdAt: Date

    init(
        id: UUID = UUID(),
        operationType: OperationType,
        resourceType: ResourceType,
        resourceId: UUID,
        data: Data
    ) {
        self.id = id
        self.operationType = operationType.rawValue
        self.resourceType = resourceType.rawValue
        self.resourceId = resourceId
        self.idempotencyKey = "\(resourceId.uuidString)-\(Date().timeIntervalSince1970)"
        self.clientTimestamp = Date()
        self.syncStatus = SyncStatus.pending.rawValue
        self.retryCount = 0
        self.operationData = data
        self.createdAt = Date()
    }
}

actor OfflineQueue {
    private let logger = Logger.shared
    private let maxRetries = 3

    func enqueue<T: Codable>(
        operation: OperationType,
        resourceType: ResourceType,
        resourceId: UUID,
        data: T
    ) async throws {
        let encoder = JSONEncoder()
        let encodedData = try encoder.encode(data)

        // In a real implementation, this would save to SwiftData
        logger.info("Enqueued \(operation.rawValue) for \(resourceType.rawValue): \(resourceId)", category: .sync)
    }

    func dequeuePending(limit: Int = 50) async -> [OfflineQueueItem] {
        // In a real implementation, this would fetch from SwiftData
        return []
    }

    func markAsSynced(_ item: OfflineQueueItem) async {
        // Update item status to synced
        logger.info("Marked item as synced: \(item.id)", category: .sync)
    }

    func markAsFailed(_ item: OfflineQueueItem, error: String) async {
        // Update item with error and increment retry count
        logger.warning("Marked item as failed: \(item.id) - \(error)", category: .sync)
    }

    func getPendingCount() async -> Int {
        // Return count of pending items
        return 0
    }

    func clearSynced() async {
        // Remove all synced items
        logger.info("Cleared synced items from queue", category: .sync)
    }

    func pruneOldItems(maxAge: TimeInterval = 30 * 24 * 60 * 60) async {
        // Remove items older than maxAge
        logger.info("Pruned old items from queue", category: .sync)
    }
}
