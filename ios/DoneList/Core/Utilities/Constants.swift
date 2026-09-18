import Foundation

enum Constants {
    enum API {
        static let timeout: TimeInterval = 30
        static let maxRetries = 3
        static let retryDelay: TimeInterval = 1.0
    }

    enum Sync {
        static let batchSize = 50
        static let maxQueueItems = 1000
        static let maxQueueAge: TimeInterval = 30 * 24 * 60 * 60 // 30 days
    }

    enum Checkin {
        static let maxContentLength = 500
        static let maxTagsPerCheckin = 10
        static let intervalMinutes = [15, 30, 45, 120]
    }

    enum Cache {
        static let defaultExpiry: TimeInterval = 5 * 60 // 5 minutes
        static let calendarExpiry: TimeInterval = 60 * 60 // 1 hour
    }

    enum Keychain {
        static let accessTokenKey = "donelist.auth.accessToken"
        static let refreshTokenKey = "donelist.auth.refreshToken"
        static let tokenExpiryKey = "donelist.auth.tokenExpiry"
        static let userIdKey = "donelist.auth.userId"
    }

    enum Notification {
        static let checkinReminderCategory = "CHECKIN_REMINDER"
        static let checkinActionId = "CHECKIN_ACTION"
        static let snoozeActionId = "SNOOZE_ACTION"
    }
}
