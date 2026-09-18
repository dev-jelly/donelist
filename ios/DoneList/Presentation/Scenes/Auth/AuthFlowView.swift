import SwiftUI

struct AuthFlowView: View {
    @State private var showRegister = false

    var body: some View {
        NavigationStack {
            LoginView(showRegister: $showRegister)
                .navigationDestination(isPresented: $showRegister) {
                    RegisterView(showRegister: $showRegister)
                }
        }
    }
}

#Preview {
    AuthFlowView()
}
