import SwiftUI
import Combine

@MainActor
final class AppState: ObservableObject {
    @Published var isAuthenticated: Bool = false
    @Published var currentUser: User?
    @Published var isOnline: Bool = true
    @Published var pendingSyncCount: Int = 0

    private var cancellables = Set<AnyCancellable>()

    init() {
        setupNetworkMonitoring()
    }

    private func setupNetworkMonitoring() {
        // Network monitoring will be set up in Sync module
    }

    func signIn(user: User) {
        self.currentUser = user
        self.isAuthenticated = true
    }

    func signOut() {
        self.currentUser = nil
        self.isAuthenticated = false
    }
}
