import Foundation
import UserNotifications

@MainActor
final class NotificationService: NSObject, ObservableObject {
    static let shared = NotificationService()

    @Published private(set) var badgeCount: Int = 0
    @Published var pendingNotification: DLNotification?

    private let logger = Logger.shared
    private let deepLinkRouter: DeepLinkRouter

    private override init() {
        self.deepLinkRouter = DeepLinkRouter.shared
        super.init()
        UNUserNotificationCenter.current().delegate = self
    }

    // MARK: - Badge Management

    func updateBadgeCount(_ count: Int) async {
        badgeCount = count
        try? await UNUserNotificationCenter.current().setBadgeCount(count)
    }

    func clearBadge() async {
        await updateBadgeCount(0)
    }

    // MARK: - Notification Categories

    func registerNotificationCategories() {
        let checkinAction = UNNotificationAction(
            identifier: Constants.Notification.checkinActionId,
            title: "체크인하기",
            options: [.foreground]
        )

        let snoozeAction = UNNotificationAction(
            identifier: Constants.Notification.snoozeActionId,
            title: "15분 후 다시 알림",
            options: []
        )

        let checkinCategory = UNNotificationCategory(
            identifier: Constants.Notification.checkinReminderCategory,
            actions: [checkinAction, snoozeAction],
            intentIdentifiers: [],
            options: []
        )

        UNUserNotificationCenter.current().setNotificationCategories([checkinCategory])
        logger.info("Notification categories registered", category: .general)
    }

    // MARK: - Local Notifications

    func scheduleCheckinReminder(at date: Date, message: String) async throws {
        let content = UNMutableNotificationContent()
        content.title = "체크인 알림"
        content.body = message
        content.sound = .default
        content.categoryIdentifier = Constants.Notification.checkinReminderCategory

        let components = Calendar.current.dateComponents([.hour, .minute], from: date)
        let trigger = UNCalendarNotificationTrigger(dateMatching: components, repeats: false)

        let request = UNNotificationRequest(
            identifier: UUID().uuidString,
            content: content,
            trigger: trigger
        )

        try await UNUserNotificationCenter.current().add(request)
        logger.info("Checkin reminder scheduled for \(date)", category: .general)
    }

    func cancelAllPendingNotifications() {
        UNUserNotificationCenter.current().removeAllPendingNotificationRequests()
        logger.info("All pending notifications cancelled", category: .general)
    }
}

// MARK: - UNUserNotificationCenterDelegate

extension NotificationService: UNUserNotificationCenterDelegate {
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        logger.info("Received notification in foreground: \(notification.request.identifier)", category: .general)
        processNotification(notification.request.content.userInfo)
        return [.banner, .sound, .badge]
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        logger.info("User tapped notification: \(response.notification.request.identifier)", category: .general)

        let userInfo = response.notification.request.content.userInfo

        switch response.actionIdentifier {
        case Constants.Notification.checkinActionId:
            await handleCheckinAction(userInfo: userInfo)
        case Constants.Notification.snoozeActionId:
            await handleSnoozeAction(userInfo: userInfo)
        case UNNotificationDefaultActionIdentifier:
            await handleDefaultAction(userInfo: userInfo)
        default:
            break
        }
    }

    private func processNotification(_ userInfo: [AnyHashable: Any]) {
        if let notification = parseNotification(userInfo) {
            pendingNotification = notification
        }
    }

    private func parseNotification(_ userInfo: [AnyHashable: Any]) -> DLNotification? {
        guard let type = userInfo["type"] as? String else { return nil }

        return DLNotification(
            id: UUID(),
            type: NotificationType(rawValue: type) ?? .general,
            title: userInfo["title"] as? String ?? "",
            body: userInfo["body"] as? String ?? "",
            data: userInfo,
            receivedAt: Date()
        )
    }

    private func handleCheckinAction(userInfo: [AnyHashable: Any]) async {
        deepLinkRouter.navigate(to: .checkin)
    }

    private func handleSnoozeAction(userInfo: [AnyHashable: Any]) async {
        let snoozeDate = Date().adding(minutes: 15)
        try? await scheduleCheckinReminder(at: snoozeDate, message: "체크인 시간입니다!")
    }

    private func handleDefaultAction(userInfo: [AnyHashable: Any]) async {
        if let deepLink = userInfo["deep_link"] as? String {
            deepLinkRouter.handleDeepLink(deepLink)
        }
    }
}

// MARK: - Notification Models

struct DLNotification: Identifiable, Sendable {
    let id: UUID
    let type: NotificationType
    let title: String
    let body: String
    let data: [AnyHashable: Any]
    let receivedAt: Date

    init(id: UUID, type: NotificationType, title: String, body: String, data: [AnyHashable: Any], receivedAt: Date) {
        self.id = id
        self.type = type
        self.title = title
        self.body = body
        self.data = [:]  // Can't store arbitrary dictionary in Sendable
        self.receivedAt = receivedAt
    }
}

enum NotificationType: String, Codable, Sendable {
    case checkinReminder = "checkin_reminder"
    case streakAlert = "streak_alert"
    case achievement = "achievement"
    case general = "general"
}
