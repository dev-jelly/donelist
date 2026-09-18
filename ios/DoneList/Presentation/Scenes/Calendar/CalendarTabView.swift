import SwiftUI

struct CalendarTabView: View {
    @StateObject private var viewModel = CalendarViewModel()
    @State private var selectedView: CalendarViewType = .monthly
    @State private var selectedDate: Date = Date()
    @State private var showDayDetail = false

    enum CalendarViewType: String, CaseIterable {
        case monthly = "월별"
        case heatmap = "히트맵"
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // View type picker
                Picker("보기", selection: $selectedView) {
                    ForEach(CalendarViewType.allCases, id: \.self) { type in
                        Text(type.rawValue).tag(type)
                    }
                }
                .pickerStyle(.segmented)
                .padding()

                // Calendar content
                switch selectedView {
                case .monthly:
                    MonthGridView(
                        calendar: viewModel.monthlyCalendar,
                        selectedDate: $selectedDate,
                        onDayTap: { day in
                            selectedDate = dateFromString(day.date) ?? Date()
                            showDayDetail = true
                        },
                        onPreviousMonth: { Task { await viewModel.loadPreviousMonth() } },
                        onNextMonth: { Task { await viewModel.loadNextMonth() } }
                    )
                case .heatmap:
                    HeatmapView(data: viewModel.heatmapData)
                }

                // Summary section
                if let summary = viewModel.monthlyCalendar?.summary {
                    CalendarSummaryView(summary: summary)
                }
            }
            .navigationTitle("캘린더")
            .sheet(isPresented: $showDayDetail) {
                DayDetailSheet(date: selectedDate)
            }
            .task {
                await viewModel.loadCurrentMonth()
                await viewModel.loadHeatmap()
            }
        }
    }

    private func dateFromString(_ string: String) -> Date? {
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.date(from: string)
    }
}

@MainActor
final class CalendarViewModel: ObservableObject {
    @Published var monthlyCalendar: MonthlyCalendar?
    @Published var heatmapData: HeatmapData?
    @Published var isLoading = false
    @Published var currentYear: Int = Calendar.current.component(.year, from: Date())
    @Published var currentMonth: Int = Calendar.current.component(.month, from: Date())

    func loadCurrentMonth() async {
        await loadMonth(year: currentYear, month: currentMonth)
    }

    func loadPreviousMonth() async {
        if currentMonth == 1 {
            currentMonth = 12
            currentYear -= 1
        } else {
            currentMonth -= 1
        }
        await loadMonth(year: currentYear, month: currentMonth)
    }

    func loadNextMonth() async {
        if currentMonth == 12 {
            currentMonth = 1
            currentYear += 1
        } else {
            currentMonth += 1
        }
        await loadMonth(year: currentYear, month: currentMonth)
    }

    private func loadMonth(year: Int, month: Int) async {
        isLoading = true
        defer { isLoading = false }

        // Generate mock calendar data
        let calendar = Calendar.current
        var components = DateComponents()
        components.year = year
        components.month = month
        components.day = 1

        guard let firstDay = calendar.date(from: components) else { return }
        let range = calendar.range(of: .day, in: .month, for: firstDay)!
        let daysInMonth = range.count

        let dateFormatter = DateFormatter()
        dateFormatter.dateFormat = "yyyy-MM-dd"

        var weeks: [CalendarWeek] = []
        var currentWeek: [CalendarDay] = []

        // Fill leading days
        let weekday = calendar.component(.weekday, from: firstDay)
        let leadingDays = (weekday + 5) % 7 // Monday start
        for _ in 0..<leadingDays {
            currentWeek.append(CalendarDay(date: "", checkinCount: 0, totalMinutes: 0, completionPercent: 0, colorIntensity: 0, isCurrentMonth: false, isToday: false, hasCheckins: false))
        }

        // Fill month days
        for day in 1...daysInMonth {
            components.day = day
            if let date = calendar.date(from: components) {
                let dateString = dateFormatter.string(from: date)
                let isToday = calendar.isDateInToday(date)
                let checkinCount = Int.random(in: 0...5)

                currentWeek.append(CalendarDay(
                    date: dateString,
                    checkinCount: checkinCount,
                    totalMinutes: checkinCount * 30,
                    completionPercent: Double(checkinCount) / 5.0 * 100,
                    colorIntensity: min(checkinCount, 4),
                    isCurrentMonth: true,
                    isToday: isToday,
                    hasCheckins: checkinCount > 0
                ))

                if currentWeek.count == 7 {
                    weeks.append(CalendarWeek(id: UUID(), days: currentWeek))
                    currentWeek = []
                }
            }
        }

        // Fill trailing days
        while currentWeek.count < 7 && !currentWeek.isEmpty {
            currentWeek.append(CalendarDay(date: "", checkinCount: 0, totalMinutes: 0, completionPercent: 0, colorIntensity: 0, isCurrentMonth: false, isToday: false, hasCheckins: false))
        }
        if !currentWeek.isEmpty {
            weeks.append(CalendarWeek(id: UUID(), days: currentWeek))
        }

        let monthName = dateFormatter.monthSymbols[month - 1]

        monthlyCalendar = MonthlyCalendar(
            year: year,
            month: month,
            monthName: monthName,
            startDay: .monday,
            weeks: weeks,
            summary: MonthlySummary(
                totalCheckins: Int.random(in: 50...100),
                totalMinutes: Int.random(in: 1500...3000),
                daysWithCheckins: Int.random(in: 15...25),
                totalDaysInMonth: daysInMonth,
                averagePerDay: Double.random(in: 2...5),
                completionRate: Double.random(in: 60...90),
                mostProductiveDay: nil,
                mostProductiveCount: Int.random(in: 5...10),
                currentStreak: Int.random(in: 1...10),
                longestStreak: Int.random(in: 5...20)
            ),
            categories: [],
            previousMonth: "\(month == 1 ? year - 1 : year)-\(String(format: "%02d", month == 1 ? 12 : month - 1))",
            nextMonth: "\(month == 12 ? year + 1 : year)-\(String(format: "%02d", month == 12 ? 1 : month + 1))",
            generatedAt: Date(),
            cacheExpiration: Date().addingTimeInterval(3600)
        )
    }

    func loadHeatmap() async {
        // Mock heatmap data for last 365 days
        var days: [HeatmapDay] = []
        let dateFormatter = DateFormatter()
        dateFormatter.dateFormat = "yyyy-MM-dd"

        for i in 0..<365 {
            let date = Date().adding(days: -i)
            let count = Int.random(in: 0...5)
            days.append(HeatmapDay(
                date: dateFormatter.string(from: date),
                checkinCount: count,
                totalMinutes: count * 30,
                completionPercent: Double(count) / 5.0 * 100,
                colorIntensity: min(count, 4)
            ))
        }

        heatmapData = HeatmapData(
            startDate: dateFormatter.string(from: Date().adding(days: -364)),
            endDate: dateFormatter.string(from: Date()),
            days: days.reversed(),
            totalDays: 365,
            activeDays: days.filter { $0.checkinCount > 0 }.count,
            totalCheckins: days.reduce(0) { $0 + $1.checkinCount },
            generatedAt: Date()
        )
    }
}

extension Date {
    func adding(days: Int) -> Date {
        Calendar.current.date(byAdding: .day, value: days, to: self) ?? self
    }
}

#Preview {
    CalendarTabView()
}
