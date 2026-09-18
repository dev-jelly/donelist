import SwiftUI

struct MonthGridView: View {
    let calendar: MonthlyCalendar?
    @Binding var selectedDate: Date
    let onDayTap: (CalendarDay) -> Void
    let onPreviousMonth: () -> Void
    let onNextMonth: () -> Void

    private let weekdays = ["월", "화", "수", "목", "금", "토", "일"]

    var body: some View {
        VStack(spacing: 16) {
            // Month navigation
            HStack {
                Button(action: onPreviousMonth) {
                    Image(systemName: "chevron.left")
                }

                Spacer()

                if let cal = calendar {
                    Text("\(cal.year)년 \(cal.month)월")
                        .font(.headline)
                }

                Spacer()

                Button(action: onNextMonth) {
                    Image(systemName: "chevron.right")
                }
            }
            .padding(.horizontal)

            // Weekday headers
            HStack(spacing: 0) {
                ForEach(weekdays, id: \.self) { day in
                    Text(day)
                        .font(.caption)
                        .fontWeight(.medium)
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity)
                }
            }

            // Calendar grid
            if let weeks = calendar?.weeks {
                VStack(spacing: 4) {
                    ForEach(weeks) { week in
                        HStack(spacing: 4) {
                            ForEach(week.days) { day in
                                DayCell(day: day, onTap: { onDayTap(day) })
                            }
                        }
                    }
                }
            }
        }
        .padding()
    }
}

struct DayCell: View {
    let day: CalendarDay
    let onTap: () -> Void

    private let intensityColors: [Color] = [
        .gray.opacity(0.1),
        .green.opacity(0.3),
        .green.opacity(0.5),
        .green.opacity(0.7),
        .green
    ]

    var body: some View {
        Button(action: onTap) {
            VStack(spacing: 2) {
                if !day.date.isEmpty {
                    Text(dayNumber)
                        .font(.system(size: 14, weight: day.isToday ? .bold : .regular))
                        .foregroundStyle(day.isCurrentMonth ? .primary : .tertiary)
                }
            }
            .frame(maxWidth: .infinity)
            .frame(height: 44)
            .background(backgroundColor)
            .cornerRadius(8)
            .overlay {
                if day.isToday {
                    RoundedRectangle(cornerRadius: 8)
                        .stroke(Color.blue, lineWidth: 2)
                }
            }
        }
        .buttonStyle(.plain)
        .disabled(day.date.isEmpty)
    }

    private var dayNumber: String {
        guard let lastPart = day.date.split(separator: "-").last else { return "" }
        return String(Int(lastPart) ?? 0)
    }

    private var backgroundColor: Color {
        guard day.isCurrentMonth else { return .clear }
        return intensityColors[day.colorIntensity]
    }
}
