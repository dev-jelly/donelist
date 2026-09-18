import SwiftUI

struct LoginView: View {
    @Binding var showRegister: Bool
    @StateObject private var viewModel = LoginViewModel()
    @EnvironmentObject private var appState: AppState

    var body: some View {
        ScrollView {
            VStack(spacing: 32) {
                headerSection
                formSection
                loginButton
                registerLink
            }
            .padding(24)
        }
        .navigationBarHidden(true)
        .alert("오류", isPresented: $viewModel.showError) {
            Button("확인", role: .cancel) {}
        } message: {
            Text(viewModel.errorMessage)
        }
        .onChange(of: viewModel.loginSuccess) { _, success in
            if success, let user = viewModel.user {
                appState.signIn(user: user)
            }
        }
    }

    private var headerSection: some View {
        VStack(spacing: 12) {
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 80))
                .foregroundStyle(.blue)

            Text("DoneList")
                .font(.largeTitle)
                .fontWeight(.bold)

            Text("오늘 완료한 일을 기록하세요")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .padding(.top, 40)
    }

    private var formSection: some View {
        VStack(spacing: 16) {
            TextField("이메일", text: $viewModel.email)
                .textFieldStyle(.roundedBorder)
                .textContentType(.emailAddress)
                .keyboardType(.emailAddress)
                .autocapitalization(.none)

            SecureField("비밀번호", text: $viewModel.password)
                .textFieldStyle(.roundedBorder)
                .textContentType(.password)
        }
    }

    private var loginButton: some View {
        Button {
            Task { await viewModel.login() }
        } label: {
            if viewModel.isLoading {
                ProgressView()
                    .tint(.white)
            } else {
                Text("로그인")
                    .fontWeight(.semibold)
            }
        }
        .frame(maxWidth: .infinity)
        .frame(height: 50)
        .background(Color.blue)
        .foregroundStyle(.white)
        .cornerRadius(12)
        .disabled(viewModel.isLoading || !viewModel.isValid)
        .opacity(viewModel.isValid ? 1 : 0.6)
    }

    private var registerLink: some View {
        Button {
            showRegister = true
        } label: {
            Text("계정이 없으신가요? ")
                .foregroundStyle(.secondary)
            + Text("회원가입")
                .foregroundStyle(.blue)
                .fontWeight(.semibold)
        }
        .font(.subheadline)
    }
}

@MainActor
final class LoginViewModel: ObservableObject {
    @Published var email = ""
    @Published var password = ""
    @Published var isLoading = false
    @Published var showError = false
    @Published var errorMessage = ""
    @Published var loginSuccess = false
    @Published var user: User?

    var isValid: Bool {
        !email.isEmpty && email.isValidEmail && !password.isEmpty
    }

    // TODO: Inject via DI
    private var loginUseCase: LoginUseCase?

    func login() async {
        guard isValid else { return }

        isLoading = true
        defer { isLoading = false }

        do {
            // TODO: Use actual use case
            // let result = try await loginUseCase?.execute(email: email, password: password)
            // user = result?.user
            // loginSuccess = true

            // Temporary mock
            try await Task.sleep(nanoseconds: 1_000_000_000)
            user = User(
                id: UUID(),
                email: email,
                displayName: nil,
                avatarURL: nil,
                role: .user,
                subscriptionTier: .free,
                appMode: .personal,
                createdAt: Date(),
                updatedAt: Date()
            )
            loginSuccess = true
        } catch let error as AppError {
            errorMessage = error.localizedDescription
            showError = true
        } catch {
            errorMessage = "로그인에 실패했습니다"
            showError = true
        }
    }
}

#Preview {
    LoginView(showRegister: .constant(false))
        .environmentObject(AppState())
}
