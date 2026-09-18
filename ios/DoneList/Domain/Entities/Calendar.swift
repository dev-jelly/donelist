import Foundation

enum StartDay: String, Codable, Sendable {
    case sunday, monday
}

struct CalendarDay: Identifiable, Codable, Equatable, Sendable {
    var id: String { date }
    let date: String
    let checkinCount: Int
    let totalMinutes: Int
    let completionPercent: Double
    let colorIntensity: Int
    let isCurrentMonth: Bool
    let isToday: Bool
    let hasCheckins: Bool
}

struct CalendarWeek: Identifiable, Codable, Equatable, Sendable {
    let id: UUID
    let days: [CalendarDay]
}

struct MonthlySummary: Codable, Equatable, Sendable {
    let totalCheckins: Int
    let totalMinutes: Int
    let daysWithCheckins: Int
    let totalDaysInMonth: Int
    let averagePerDay: Double
    let completionRate: Double
    let mostProductiveDay: String?
    let mostProductiveCount: Int
    let currentStreak: Int
    let longestStreak: Int
}

struct CategoryBreakdown: Identifiable, Codable, Equatable, Sendable {
    let id: UUID?
    let categoryName: String
    let count: Int
    let percentage: Double
}

struct MonthlyCalendar: Codable, Equatable, Sendable {
    let year: Int
    let month: Int
    let monthName: String
    let startDay: StartDay
    let weeks: [CalendarWeek]
    let summary: MonthlySummary
    let categories: [CategoryBreakdown]
    let previousMonth: String
    let nextMonth: String
    let generatedAt: Date
    let cacheExpiration: Date
}

struct HeatmapDay: Identifiable, Codable, Equatable, Sendable {
    var id: String { date }
    let date: String
    let checkinCount: Int
    let totalMinutes: Int
    let completionPercent: Double
    let colorIntensity: Int
}

struct HeatmapData: Codable, Equatable, Sendable {
    let startDate: String
    let endDate: String
    let days: [HeatmapDay]
    let totalDays: Int
    let activeDays: Int
    let totalCheckins: Int
    let generatedAt: Date
}
