import SwiftUI

struct NotificationSettingsView: View {
    @StateObject private var viewModel = NotificationSettingsViewModel()

    var body: some View {
        List {
            Section {
                Toggle("푸시 알림", isOn: $viewModel.pushEnabled)
                    .onChange(of: viewModel.pushEnabled) { _, enabled in
                        Task { await viewModel.togglePushNotifications(enabled) }
                    }
            } footer: {
                if viewModel.authorizationStatus == .denied {
                    Text("알림이 비활성화되어 있습니다. 설정 앱에서 알림을 활성화해주세요.")
                        .foregroundStyle(.red)
                }
            }

            if viewModel.pushEnabled {
                Section("체크인 알림") {
                    Toggle("체크인 리마인더", isOn: $viewModel.checkinRemindersEnabled)

                    if viewModel.checkinRemindersEnabled {
                        DatePicker(
                            "알림 시간",
                            selection: $viewModel.reminderTime,
                            displayedComponents: .hourAndMinute
                        )

                        Picker("반복", selection: $viewModel.reminderFrequency) {
                            Text("매일").tag(ReminderFrequency.daily)
                            Text("평일만").tag(ReminderFrequency.weekdays)
                            Text("주말만").tag(ReminderFrequency.weekends)
                        }
                    }
                }

                Section("기타 알림") {
                    Toggle("연속 기록 알림", isOn: $viewModel.streakAlertsEnabled)
                    Toggle("업적 알림", isOn: $viewModel.achievementAlertsEnabled)
                }
            }

            Section {
                Button("알림 설정 열기") {
                    viewModel.openSystemSettings()
                }
            }
        }
        .navigationTitle("알림 설정")
        .task {
            await viewModel.checkAuthorizationStatus()
        }
    }
}

enum ReminderFrequency: String, CaseIterable {
    case daily, weekdays, weekends
}

@MainActor
final class NotificationSettingsViewModel: ObservableObject {
    @Published var pushEnabled: Bool = false
    @Published var checkinRemindersEnabled: Bool = true
    @Published var reminderTime: Date = Calendar.current.date(from: DateComponents(hour: 9, minute: 0)) ?? Date()
    @Published var reminderFrequency: ReminderFrequency = .daily
    @Published var streakAlertsEnabled: Bool = true
    @Published var achievementAlertsEnabled: Bool = true
    @Published var authorizationStatus: UNAuthorizationStatus = .notDetermined

    private let apnsManager = APNsTokenManager.shared
    private let notificationService = NotificationService.shared

    func checkAuthorizationStatus() async {
        await apnsManager.checkAuthorizationStatus()
        authorizationStatus = apnsManager.authorizationStatus
        pushEnabled = authorizationStatus == .authorized
    }

    func togglePushNotifications(_ enabled: Bool) async {
        if enabled {
            do {
                try await apnsManager.requestAuthorization()
                await checkAuthorizationStatus()
                notificationService.registerNotificationCategories()
            } catch {
                pushEnabled = false
            }
        } else {
            notificationService.cancelAllPendingNotifications()
        }
    }

    func openSystemSettings() {
        if let url = URL(string: UIApplication.openSettingsURLString) {
            UIApplication.shared.open(url)
        }
    }
}

#Preview {
    NavigationStack {
        NotificationSettingsView()
    }
}
