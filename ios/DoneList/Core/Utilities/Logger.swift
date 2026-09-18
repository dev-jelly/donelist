import Foundation
import OSLog

final class Logger {
    static let shared = Logger()

    private let subsystem = Bundle.main.bundleIdentifier ?? "com.donelist"

    private lazy var generalLogger = OSLog(subsystem: subsystem, category: "general")
    private lazy var networkLogger = OSLog(subsystem: subsystem, category: "network")
    private lazy var authLogger = OSLog(subsystem: subsystem, category: "auth")
    private lazy var syncLogger = OSLog(subsystem: subsystem, category: "sync")
    private lazy var uiLogger = OSLog(subsystem: subsystem, category: "ui")

    enum Category {
        case general, network, auth, sync, ui
    }

    private init() {}

    func debug(_ message: String, category: Category = .general, file: String = #file, function: String = #function, line: Int = #line) {
        log(message, type: .debug, category: category, file: file, function: function, line: line)
    }

    func info(_ message: String, category: Category = .general, file: String = #file, function: String = #function, line: Int = #line) {
        log(message, type: .info, category: category, file: file, function: function, line: line)
    }

    func warning(_ message: String, category: Category = .general, file: String = #file, function: String = #function, line: Int = #line) {
        log(message, type: .default, category: category, file: file, function: function, line: line)
    }

    func error(_ message: String, error: Error? = nil, category: Category = .general, file: String = #file, function: String = #function, line: Int = #line) {
        var fullMessage = message
        if let error = error {
            fullMessage += " | Error: \(error.localizedDescription)"
        }
        log(fullMessage, type: .error, category: category, file: file, function: function, line: line)
    }

    private func log(_ message: String, type: OSLogType, category: Category, file: String, function: String, line: Int) {
        let logger = osLog(for: category)
        let fileName = (file as NSString).lastPathComponent
        let logMessage = "[\(fileName):\(line)] \(function) - \(message)"

        os_log("%{public}@", log: logger, type: type, logMessage)

        #if DEBUG
        let emoji: String
        switch type {
        case .debug: emoji = "🔍"
        case .info: emoji = "ℹ️"
        case .default: emoji = "⚠️"
        case .error: emoji = "❌"
        case .fault: emoji = "💥"
        default: emoji = "📝"
        }
        print("\(emoji) [\(category)] \(logMessage)")
        #endif
    }

    private func osLog(for category: Category) -> OSLog {
        switch category {
        case .general: return generalLogger
        case .network: return networkLogger
        case .auth: return authLogger
        case .sync: return syncLogger
        case .ui: return uiLogger
        }
    }
}
