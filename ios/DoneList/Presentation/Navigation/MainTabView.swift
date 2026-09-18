import SwiftUI

struct MainTabView: View {
    @State private var selectedTab: Tab = .today

    enum Tab: String, CaseIterable {
        case today = "Today"
        case timeline = "Timeline"
        case calendar = "Calendar"
        case statistics = "Statistics"
        case settings = "Settings"

        var icon: String {
            switch self {
            case .today: return "checkmark.circle.fill"
            case .timeline: return "list.bullet"
            case .calendar: return "calendar"
            case .statistics: return "chart.bar.fill"
            case .settings: return "gearshape.fill"
            }
        }
    }

    var body: some View {
        TabView(selection: $selectedTab) {
            TodayView()
                .tabItem {
                    Label(Tab.today.rawValue, systemImage: Tab.today.icon)
                }
                .tag(Tab.today)

            TimelineView()
                .tabItem {
                    Label(Tab.timeline.rawValue, systemImage: Tab.timeline.icon)
                }
                .tag(Tab.timeline)

            CalendarTabView()
                .tabItem {
                    Label(Tab.calendar.rawValue, systemImage: Tab.calendar.icon)
                }
                .tag(Tab.calendar)

            StatisticsView()
                .tabItem {
                    Label(Tab.statistics.rawValue, systemImage: Tab.statistics.icon)
                }
                .tag(Tab.statistics)

            SettingsView()
                .tabItem {
                    Label(Tab.settings.rawValue, systemImage: Tab.settings.icon)
                }
                .tag(Tab.settings)
        }
    }
}

#Preview {
    MainTabView()
}
