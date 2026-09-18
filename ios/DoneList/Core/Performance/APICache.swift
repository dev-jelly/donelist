import Foundation

/// API 응답 캐싱
actor APICache {
    static let shared = APICache()

    private struct CacheEntry {
        let data: Data
        let timestamp: Date
        let expiresIn: TimeInterval

        var isExpired: Bool {
            Date().timeIntervalSince(timestamp) > expiresIn
        }
    }

    private var cache: [String: CacheEntry] = [:]
    private let maxEntries = 100

    private init() {}

    func get<T: Decodable>(_ key: String, as type: T.Type) -> T? {
        guard let entry = cache[key], !entry.isExpired else {
            cache.removeValue(forKey: key)
            return nil
        }

        return try? JSONDecoder().decode(type, from: entry.data)
    }

    func set<T: Encodable>(_ value: T, for key: String, expiresIn: TimeInterval = Constants.Cache.defaultExpiry) {
        guard let data = try? JSONEncoder().encode(value) else { return }

        // Evict oldest entries if needed
        if cache.count >= maxEntries {
            let sortedKeys = cache.sorted { $0.value.timestamp < $1.value.timestamp }
            for (key, _) in sortedKeys.prefix(10) {
                cache.removeValue(forKey: key)
            }
        }

        cache[key] = CacheEntry(data: data, timestamp: Date(), expiresIn: expiresIn)
    }

    func invalidate(_ key: String) {
        cache.removeValue(forKey: key)
    }

    func invalidateAll() {
        cache.removeAll()
    }

    func invalidateExpired() {
        for (key, entry) in cache where entry.isExpired {
            cache.removeValue(forKey: key)
        }
    }
}

/// 캐시 가능한 요청을 위한 프로토콜
protocol CacheableRequest {
    var cacheKey: String { get }
    var cacheExpiry: TimeInterval { get }
}

extension CacheableRequest {
    var cacheExpiry: TimeInterval { Constants.Cache.defaultExpiry }
}
