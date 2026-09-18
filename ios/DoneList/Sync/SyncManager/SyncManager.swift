import Foundation
import Network
import Combine

@MainActor
final class SyncManager: ObservableObject {
    static let shared = SyncManager()

    @Published private(set) var isOnline: Bool = true
    @Published private(set) var isSyncing: Bool = false
    @Published private(set) var pendingCount: Int = 0
    @Published private(set) var lastSyncDate: Date?

    private let offlineQueue: OfflineQueue
    private let syncEngine: SyncEngine
    private let networkMonitor: NWPathMonitor
    private let monitorQueue = DispatchQueue(label: "com.donelist.networkMonitor")
    private var cancellables = Set<AnyCancellable>()
    private let logger = Logger.shared

    private init() {
        self.offlineQueue = OfflineQueue()
        self.syncEngine = SyncEngine(offlineQueue: offlineQueue)
        self.networkMonitor = NWPathMonitor()

        setupNetworkMonitoring()
        setupQueueObserver()
    }

    private func setupNetworkMonitoring() {
        networkMonitor.pathUpdateHandler = { [weak self] path in
            Task { @MainActor in
                let wasOnline = self?.isOnline ?? true
                self?.isOnline = path.status == .satisfied

                if !wasOnline && path.status == .satisfied {
                    self?.logger.info("Network restored, triggering sync", category: .sync)
                    await self?.syncPendingItems()
                }
            }
        }
        networkMonitor.start(queue: monitorQueue)
    }

    private func setupQueueObserver() {
        Task {
            pendingCount = await offlineQueue.getPendingCount()
        }
    }

    // MARK: - Public API

    func enqueue<T: Codable & Sendable>(
        operation: OperationType,
        resourceType: ResourceType,
        resourceId: UUID,
        data: T
    ) async throws {
        try await offlineQueue.enqueue(
            operation: operation,
            resourceType: resourceType,
            resourceId: resourceId,
            data: data
        )
        pendingCount = await offlineQueue.getPendingCount()

        if isOnline {
            await syncPendingItems()
        }
    }

    func syncPendingItems() async {
        guard !isSyncing && isOnline else { return }

        isSyncing = true
        defer { isSyncing = false }

        logger.info("Starting sync of pending items", category: .sync)

        do {
            let syncedCount = try await syncEngine.syncAll()
            pendingCount = await offlineQueue.getPendingCount()
            lastSyncDate = Date()

            logger.info("Sync completed: \(syncedCount) items synced", category: .sync)
        } catch {
            logger.error("Sync failed", error: error, category: .sync)
        }
    }

    func forceSync() async {
        await syncPendingItems()
    }
}
