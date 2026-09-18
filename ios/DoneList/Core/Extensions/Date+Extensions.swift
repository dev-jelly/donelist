import Foundation

extension Date {
    // MARK: - ISO8601 Formatting
    var iso8601String: String {
        ISO8601DateFormatter.shared.string(from: self)
    }

    static func from(iso8601 string: String) -> Date? {
        ISO8601DateFormatter.shared.date(from: string)
    }

    // MARK: - Relative Time
    func relativeString(from referenceDate: Date = Date()) -> String {
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .full
        formatter.locale = Locale(identifier: "ko_KR")
        return formatter.localizedString(for: self, relativeTo: referenceDate)
    }

    func shortRelativeString(from referenceDate: Date = Date()) -> String {
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .short
        formatter.locale = Locale(identifier: "ko_KR")
        return formatter.localizedString(for: self, relativeTo: referenceDate)
    }

    // MARK: - Date Comparison
    func isSameDay(as other: Date, calendar: Calendar = .current) -> Bool {
        calendar.isDate(self, inSameDayAs: other)
    }

    var isToday: Bool { Calendar.current.isDateInToday(self) }
    var isYesterday: Bool { Calendar.current.isDateInYesterday(self) }
    var isThisWeek: Bool { Calendar.current.isDate(self, equalTo: Date(), toGranularity: .weekOfYear) }

    // MARK: - Date Manipulation
    var startOfDay: Date { Calendar.current.startOfDay(for: self) }

    var endOfDay: Date {
        Calendar.current.date(bySettingHour: 23, minute: 59, second: 59, of: self) ?? self
    }

    var startOfWeek: Date {
        var calendar = Calendar.current
        calendar.firstWeekday = 2
        return calendar.dateInterval(of: .weekOfYear, for: self)?.start ?? self
    }

    var startOfMonth: Date {
        Calendar.current.dateInterval(of: .month, for: self)?.start ?? self
    }

    func adding(days: Int) -> Date {
        Calendar.current.date(byAdding: .day, value: days, to: self) ?? self
    }

    func adding(hours: Int) -> Date {
        Calendar.current.date(byAdding: .hour, value: hours, to: self) ?? self
    }

    func adding(minutes: Int) -> Date {
        Calendar.current.date(byAdding: .minute, value: minutes, to: self) ?? self
    }

    // MARK: - Checkin Interval Validation
    func isValidCheckinInterval(since lastCheckin: Date) -> (isValid: Bool, remainingWaitTime: TimeInterval?) {
        let elapsed = self.timeIntervalSince(lastCheckin)
        let minutes = Int(elapsed / 60)

        if elapsed < 7200 { // 2 hours
            let validIntervals = [15, 30, 45]
            if validIntervals.contains(minutes) {
                return (true, nil)
            } else {
                let nextValid = validIntervals.first { $0 > minutes } ?? 120
                let nextAllowedTime = lastCheckin.adding(minutes: nextValid)
                let remaining = nextAllowedTime.timeIntervalSince(self)
                return (false, remaining)
            }
        } else {
            if minutes % 120 == 0 {
                return (true, nil)
            } else {
                let nextMultiple = ((minutes / 120) + 1) * 120
                let nextAllowedTime = lastCheckin.adding(minutes: nextMultiple)
                let remaining = nextAllowedTime.timeIntervalSince(self)
                return (false, remaining)
            }
        }
    }
}

private extension ISO8601DateFormatter {
    static let shared: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()
}
