import SwiftUI

struct DayDetailSheet: View {
    @Environment(\.dismiss) private var dismiss
    let date: Date
    @State private var checkins: [Checkin] = []

    var body: some View {
        NavigationStack {
            List {
                Section {
                    HStack {
                        VStack(alignment: .leading) {
                            Text(formattedDate)
                                .font(.headline)
                            Text("\(checkins.count)개의 체크인")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                        Text(totalTime)
                            .font(.title2)
                            .fontWeight(.semibold)
                    }
                }

                Section("체크인 기록") {
                    if checkins.isEmpty {
                        ContentUnavailableView("체크인 없음", systemImage: "checkmark.circle", description: Text("이 날에는 기록된 체크인이 없습니다"))
                    } else {
                        ForEach(checkins) { checkin in
                            CheckinRow(checkin: checkin)
                        }
                    }
                }
            }
            .navigationTitle("일별 상세")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("닫기") { dismiss() }
                }
            }
            .task {
                await loadCheckins()
            }
        }
    }

    private var formattedDate: String {
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy년 M월 d일 EEEE"
        formatter.locale = Locale(identifier: "ko_KR")
        return formatter.string(from: date)
    }

    private var totalTime: String {
        let totalMinutes = checkins.reduce(0) { $0 + ($1.durationMinutes ?? 0) }
        if totalMinutes >= 60 {
            return "\(totalMinutes / 60)시간 \(totalMinutes % 60)분"
        }
        return "\(totalMinutes)분"
    }

    private func loadCheckins() async {
        // Mock data
        checkins = [
            Checkin(id: UUID(), userId: UUID(), content: "아침 루틴 완료", categoryId: nil, tags: [], checkinTime: date, durationMinutes: 30, createdAt: date, updatedAt: date),
            Checkin(id: UUID(), userId: UUID(), content: "업무 집중", categoryId: nil, tags: [], checkinTime: date, durationMinutes: 45, createdAt: date, updatedAt: date)
        ]
    }
}

struct CheckinRow: View {
    let checkin: Checkin

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(checkin.content)
                .font(.body)

            HStack {
                if let duration = checkin.durationMinutes {
                    Label("\(duration)분", systemImage: "clock")
                }

                Text(timeString)
                    .foregroundStyle(.secondary)
            }
            .font(.caption)
            .foregroundStyle(.secondary)
        }
        .padding(.vertical, 4)
    }

    private var timeString: String {
        let formatter = DateFormatter()
        formatter.dateFormat = "HH:mm"
        return formatter.string(from: checkin.checkinTime)
    }
}

#Preview {
    DayDetailSheet(date: Date())
}
