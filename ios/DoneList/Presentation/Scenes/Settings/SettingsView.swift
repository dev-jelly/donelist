import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var appState: AppState
    @State private var showLogoutConfirmation = false

    var body: some View {
        NavigationStack {
            List {
                Section("계정") {
                    if let user = appState.currentUser {
                        HStack {
                            Circle()
                                .fill(Color.blue.gradient)
                                .frame(width: 50, height: 50)
                                .overlay {
                                    Text(user.email.prefix(1).uppercased())
                                        .font(.title2)
                                        .fontWeight(.bold)
                                        .foregroundStyle(.white)
                                }

                            VStack(alignment: .leading) {
                                Text(user.displayName ?? user.email)
                                    .font(.headline)
                                Text(user.email)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }

                Section("관리") {
                    NavigationLink {
                        CategoryManagementView()
                    } label: {
                        Label("카테고리 관리", systemImage: "folder.fill")
                    }

                    NavigationLink {
                        TagManagementView()
                    } label: {
                        Label("태그 관리", systemImage: "tag.fill")
                    }
                }

                Section("알림") {
                    NavigationLink {
                        NotificationSettingsView()
                    } label: {
                        Label("알림 설정", systemImage: "bell.fill")
                    }
                }

                Section("동기화") {
                    HStack {
                        Label("동기화 상태", systemImage: "arrow.triangle.2.circlepath")
                        Spacer()
                        if appState.isOnline {
                            Text("온라인")
                                .foregroundStyle(.green)
                        } else {
                            Text("오프라인")
                                .foregroundStyle(.orange)
                        }
                    }

                    if appState.pendingSyncCount > 0 {
                        HStack {
                            Label("대기 중인 항목", systemImage: "clock")
                            Spacer()
                            Text("\(appState.pendingSyncCount)개")
                                .foregroundStyle(.secondary)
                        }
                    }
                }

                Section("정보") {
                    HStack {
                        Label("버전", systemImage: "info.circle")
                        Spacer()
                        Text("\(AppConfig.appVersion) (\(AppConfig.buildNumber))")
                            .foregroundStyle(.secondary)
                    }
                }

                Section {
                    Button(role: .destructive) {
                        showLogoutConfirmation = true
                    } label: {
                        Label("로그아웃", systemImage: "rectangle.portrait.and.arrow.right")
                    }
                }
            }
            .navigationTitle("설정")
            .confirmationDialog("로그아웃", isPresented: $showLogoutConfirmation) {
                Button("로그아웃", role: .destructive) {
                    appState.signOut()
                }
                Button("취소", role: .cancel) {}
            } message: {
                Text("정말 로그아웃하시겠습니까?")
            }
        }
    }
}

#Preview {
    SettingsView()
        .environmentObject(AppState())
}
