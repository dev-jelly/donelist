import Foundation
import UserNotifications
import UIKit

@MainActor
final class APNsTokenManager: ObservableObject {
    static let shared = APNsTokenManager()

    @Published private(set) var deviceToken: String?
    @Published private(set) var authorizationStatus: UNAuthorizationStatus = .notDetermined

    private let keychain = KeychainService()
    private let logger = Logger.shared
    private let deviceTokenKey = "com.donelist.apns.deviceToken"

    private init() {
        loadSavedToken()
    }

    private func loadSavedToken() {
        deviceToken = try? keychain.getString(forKey: deviceTokenKey)
    }

    // MARK: - Permission Request

    func requestAuthorization() async throws {
        let center = UNUserNotificationCenter.current()

        let granted = try await center.requestAuthorization(options: [
            .alert, .badge, .sound, .providesAppNotificationSettings
        ])

        if granted {
            logger.info("Push notification permission granted", category: .general)
            await checkAuthorizationStatus()
            await registerForRemoteNotifications()
        } else {
            logger.warning("Push notification permission denied", category: .general)
            authorizationStatus = .denied
        }
    }

    func checkAuthorizationStatus() async {
        let center = UNUserNotificationCenter.current()
        let settings = await center.notificationSettings()
        authorizationStatus = settings.authorizationStatus
        logger.info("Notification authorization status: \(settings.authorizationStatus.rawValue)", category: .general)
    }

    // MARK: - Device Token Management

    func registerForRemoteNotifications() async {
        await UIApplication.shared.registerForRemoteNotifications()
    }

    func didRegisterForRemoteNotifications(deviceToken: Data) {
        let tokenString = deviceToken.map { String(format: "%02.2hhx", $0) }.joined()
        self.deviceToken = tokenString
        try? keychain.save(tokenString, forKey: deviceTokenKey)
        logger.info("APNs device token received: \(tokenString.prefix(20))...", category: .general)

        Task {
            await registerTokenWithServer(tokenString)
        }
    }

    func didFailToRegisterForRemoteNotifications(error: Error) {
        logger.error("Failed to register for remote notifications", error: error, category: .general)
    }

    // MARK: - Server Registration

    private func registerTokenWithServer(_ token: String) async {
        // TODO: Call API to register device token
        logger.info("Device token registered with server", category: .general)
    }

    func unregisterTokenFromServer() async {
        guard let token = deviceToken else { return }
        // TODO: Call API to unregister device token
        logger.info("Device token unregistered from server", category: .general)
    }
}
