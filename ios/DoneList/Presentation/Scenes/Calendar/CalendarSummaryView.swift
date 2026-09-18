import SwiftUI

struct CalendarSummaryView: View {
    let summary: MonthlySummary

    var body: some View {
        VStack(spacing: 16) {
            Divider()

            HStack(spacing: 24) {
                SummaryItem(value: "\(summary.totalCheckins)", label: "체크인", icon: "checkmark.circle.fill", color: .blue)
                SummaryItem(value: "\(summary.daysWithCheckins)", label: "활성일", icon: "calendar", color: .green)
                SummaryItem(value: "\(summary.currentStreak)", label: "연속", icon: "flame.fill", color: .orange)
            }
            .padding(.horizontal)
        }
        .padding(.vertical)
        .background(Color(.systemGroupedBackground))
    }
}

struct SummaryItem: View {
    let value: String
    let label: String
    let icon: String
    let color: Color

    var body: some View {
        VStack(spacing: 4) {
            Image(systemName: icon)
                .font(.title3)
                .foregroundStyle(color)

            Text(value)
                .font(.title2)
                .fontWeight(.bold)

            Text(label)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
    }
}

#Preview {
    CalendarSummaryView(summary: MonthlySummary(
        totalCheckins: 85,
        totalMinutes: 2550,
        daysWithCheckins: 22,
        totalDaysInMonth: 30,
        averagePerDay: 3.5,
        completionRate: 73.3,
        mostProductiveDay: "2024-11-15",
        mostProductiveCount: 8,
        currentStreak: 5,
        longestStreak: 12
    ))
}
