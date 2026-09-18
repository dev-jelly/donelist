import Foundation

enum ConflictResolution: Sendable {
    case useLocal
    case useServer
    case merge
}

struct ConflictInfo<T: Sendable>: Sendable {
    let localVersion: T
    let serverVersion: T
    let localTimestamp: Date
    let serverTimestamp: Date
}

protocol ConflictResolvable: Sendable {
    var updatedAt: Date { get }
}

actor ConflictResolver {
    private let logger = Logger.shared

    // Last Write Wins strategy
    func resolve<T: ConflictResolvable>(_ conflict: ConflictInfo<T>) -> ConflictResolution {
        logger.info("Resolving conflict using Last Write Wins", category: .sync)

        if conflict.localTimestamp > conflict.serverTimestamp {
            logger.debug("Local version wins (local: \(conflict.localTimestamp), server: \(conflict.serverTimestamp))", category: .sync)
            return .useLocal
        } else {
            logger.debug("Server version wins (local: \(conflict.localTimestamp), server: \(conflict.serverTimestamp))", category: .sync)
            return .useServer
        }
    }

    func resolveCheckinConflict(local: Checkin, server: Checkin) async -> Checkin {
        let resolution = resolve(ConflictInfo(
            localVersion: local,
            serverVersion: server,
            localTimestamp: local.updatedAt,
            serverTimestamp: server.updatedAt
        ))

        switch resolution {
        case .useLocal:
            return local
        case .useServer:
            return server
        case .merge:
            // For checkins, we typically use Last Write Wins, but could implement custom merge logic
            return local.updatedAt > server.updatedAt ? local : server
        }
    }
}

extension Checkin: ConflictResolvable {}
extension Category: ConflictResolvable {}
extension Tag: ConflictResolvable {}
