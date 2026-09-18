import Foundation

enum AppConfig {
    static let appName = "DoneList"
    static let appVersion = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1.0"
    static let buildNumber = Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "1"

    enum Environment: String {
        case development
        case staging
        case production

        var baseURL: String {
            switch self {
            case .development: return "http://localhost:8080"
            case .staging: return "https://staging-api.donelist.com"
            case .production: return "https://api.donelist.com"
            }
        }

        var apiVersion: String { "/api/v1" }
    }

    #if DEBUG
    static var current: Environment = .development
    #else
    static var current: Environment = .production
    #endif

    static var baseURL: String { current.baseURL }
    static var apiBaseURL: String { current.baseURL + current.apiVersion }
}
