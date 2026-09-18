import Foundation
import SwiftUI

enum DeepLink: Equatable {
    case checkin
    case timeline
    case calendar(date: Date?)
    case statistics
    case settings
    case checkinDetail(id: UUID)
    case categoryManagement
    case tagManagement

    static func from(url: URL) -> DeepLink? {
        guard url.scheme == "donelist" else { return nil }

        let path = url.host ?? url.path
        let queryItems = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems

        switch path {
        case "checkin":
            return .checkin
        case "timeline":
            return .timeline
        case "calendar":
            if let dateString = queryItems?.first(where: { $0.name == "date" })?.value,
               let date = Date.from(iso8601: dateString) {
                return .calendar(date: date)
            }
            return .calendar(date: nil)
        case "statistics":
            return .statistics
        case "settings":
            return .settings
        case "checkin-detail":
            if let idString = queryItems?.first(where: { $0.name == "id" })?.value,
               let id = UUID(uuidString: idString) {
                return .checkinDetail(id: id)
            }
            return nil
        default:
            return nil
        }
    }
}

@MainActor
final class DeepLinkRouter: ObservableObject {
    static let shared = DeepLinkRouter()

    @Published var currentDeepLink: DeepLink?
    @Published var navigationPath = NavigationPath()
    @Published var selectedTab: MainTabView.Tab = .today

    private let logger = Logger.shared

    private init() {}

    func navigate(to deepLink: DeepLink) {
        logger.info("Navigating to deep link: \(deepLink)", category: .general)

        currentDeepLink = deepLink

        switch deepLink {
        case .checkin, .timeline:
            selectedTab = .timeline
        case .calendar:
            selectedTab = .calendar
        case .statistics:
            selectedTab = .statistics
        case .settings, .categoryManagement, .tagManagement:
            selectedTab = .settings
        case .checkinDetail:
            selectedTab = .timeline
        }
    }

    func handleDeepLink(_ urlString: String) {
        guard let url = URL(string: urlString),
              let deepLink = DeepLink.from(url: url) else {
            logger.warning("Invalid deep link: \(urlString)", category: .general)
            return
        }

        navigate(to: deepLink)
    }

    func handleURL(_ url: URL) -> Bool {
        guard let deepLink = DeepLink.from(url: url) else {
            return false
        }

        navigate(to: deepLink)
        return true
    }

    func clearDeepLink() {
        currentDeepLink = nil
    }
}
