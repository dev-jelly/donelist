import SwiftUI

struct HeatmapView: View {
    let data: HeatmapData?

    private let cellSize: CGFloat = 12
    private let spacing: CGFloat = 2
    private let weekdays = ["월", "", "수", "", "금", "", ""]

    private let intensityColors: [Color] = [
        .gray.opacity(0.15),
        .green.opacity(0.3),
        .green.opacity(0.5),
        .green.opacity(0.7),
        .green
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            // Legend
            HStack(spacing: 4) {
                Text("적음")
                    .font(.caption2)
                    .foregroundStyle(.secondary)

                ForEach(0..<5) { intensity in
                    RoundedRectangle(cornerRadius: 2)
                        .fill(intensityColors[intensity])
                        .frame(width: 12, height: 12)
                }

                Text("많음")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, alignment: .trailing)

            ScrollView(.horizontal, showsIndicators: false) {
                HStack(alignment: .top, spacing: spacing) {
                    // Weekday labels
                    VStack(spacing: spacing) {
                        ForEach(weekdays, id: \.self) { day in
                            Text(day)
                                .font(.system(size: 9))
                                .foregroundStyle(.secondary)
                                .frame(width: cellSize, height: cellSize)
                        }
                    }

                    // Heatmap grid
                    if let data = data {
                        let columns = groupByWeek(data.days)
                        ForEach(0..<columns.count, id: \.self) { colIndex in
                            VStack(spacing: spacing) {
                                ForEach(0..<7, id: \.self) { rowIndex in
                                    if rowIndex < columns[colIndex].count {
                                        let day = columns[colIndex][rowIndex]
                                        HeatmapCell(day: day, color: intensityColors[day.colorIntensity])
                                    } else {
                                        Color.clear
                                            .frame(width: cellSize, height: cellSize)
                                    }
                                }
                            }
                        }
                    }
                }
                .padding(.horizontal)
            }

            // Stats
            if let data = data {
                HStack(spacing: 24) {
                    StatItem(value: "\(data.totalCheckins)", label: "총 체크인")
                    StatItem(value: "\(data.activeDays)", label: "활성 일수")
                    StatItem(value: "\(Int(Double(data.activeDays) / Double(data.totalDays) * 100))%", label: "활성률")
                }
                .frame(maxWidth: .infinity)
            }
        }
        .padding()
    }

    private func groupByWeek(_ days: [HeatmapDay]) -> [[HeatmapDay]] {
        var columns: [[HeatmapDay]] = []
        var currentColumn: [HeatmapDay] = []

        for day in days {
            currentColumn.append(day)
            if currentColumn.count == 7 {
                columns.append(currentColumn)
                currentColumn = []
            }
        }

        if !currentColumn.isEmpty {
            columns.append(currentColumn)
        }

        return columns
    }
}

struct HeatmapCell: View {
    let day: HeatmapDay
    let color: Color

    var body: some View {
        RoundedRectangle(cornerRadius: 2)
            .fill(color)
            .frame(width: 12, height: 12)
    }
}

struct StatItem: View {
    let value: String
    let label: String

    var body: some View {
        VStack(spacing: 2) {
            Text(value)
                .font(.headline)
            Text(label)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }
}

#Preview {
    HeatmapView(data: nil)
}
