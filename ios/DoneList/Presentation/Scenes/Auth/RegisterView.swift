import SwiftUI

struct RegisterView: View {
    @Binding var showRegister: Bool
    @StateObject private var viewModel = RegisterViewModel()
    @EnvironmentObject private var appState: AppState

    var body: some View {
        ScrollView {
            VStack(spacing: 24) {
                headerSection
                formSection
                registerButton
            }
            .padding(24)
        }
        .navigationTitle("회원가입")
        .navigationBarTitleDisplayMode(.inline)
        .alert("오류", isPresented: $viewModel.showError) {
            Button("확인", role: .cancel) {}
        } message: {
            Text(viewModel.errorMessage)
        }
        .onChange(of: viewModel.registerSuccess) { _, success in
            if success, let user = viewModel.user {
                appState.signIn(user: user)
            }
        }
    }

    private var headerSection: some View {
        VStack(spacing: 8) {
            Text("계정 만들기")
                .font(.title)
                .fontWeight(.bold)

            Text("DoneList와 함께 완료한 일을 기록하세요")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
    }

    private var formSection: some View {
        VStack(spacing: 16) {
            TextField("이메일", text: $viewModel.email)
                .textFieldStyle(.roundedBorder)
                .textContentType(.emailAddress)
                .keyboardType(.emailAddress)
                .autocapitalization(.none)

            TextField("이름 (선택)", text: $viewModel.displayName)
                .textFieldStyle(.roundedBorder)
                .textContentType(.name)

            SecureField("비밀번호", text: $viewModel.password)
                .textFieldStyle(.roundedBorder)
                .textContentType(.newPassword)

            SecureField("비밀번호 확인", text: $viewModel.confirmPassword)
                .textFieldStyle(.roundedBorder)
                .textContentType(.newPassword)

            passwordStrengthIndicator
        }
    }

    private var passwordStrengthIndicator: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("비밀번호 요구사항:")
                .font(.caption)
                .foregroundStyle(.secondary)

            HStack(spacing: 4) {
                requirementBadge("8자 이상", met: viewModel.password.count >= 8)
                requirementBadge("영문", met: viewModel.password.range(of: "[A-Za-z]", options: .regularExpression) != nil)
                requirementBadge("숫자", met: viewModel.password.range(of: "[0-9]", options: .regularExpression) != nil)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func requirementBadge(_ text: String, met: Bool) -> some View {
        Text(text)
            .font(.caption2)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(met ? Color.green.opacity(0.2) : Color.gray.opacity(0.2))
            .foregroundStyle(met ? .green : .secondary)
            .cornerRadius(4)
    }

    private var registerButton: some View {
        Button {
            Task { await viewModel.register() }
        } label: {
            if viewModel.isLoading {
                ProgressView()
                    .tint(.white)
            } else {
                Text("회원가입")
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
}

@MainActor
final class RegisterViewModel: ObservableObject {
    @Published var email = ""
    @Published var displayName = ""
    @Published var password = ""
    @Published var confirmPassword = ""
    @Published var isLoading = false
    @Published var showError = false
    @Published var errorMessage = ""
    @Published var registerSuccess = false
    @Published var user: User?

    var isValid: Bool {
        !email.isEmpty && email.isValidEmail &&
        !password.isEmpty && password.count >= 8 &&
        password == confirmPassword
    }

    func register() async {
        guard isValid else { return }

        isLoading = true
        defer { isLoading = false }

        do {
            try await Task.sleep(nanoseconds: 1_000_000_000)
            user = User(
                id: UUID(),
                email: email,
                displayName: displayName.isEmpty ? nil : displayName,
                avatarURL: nil,
                role: .user,
                subscriptionTier: .free,
                appMode: .personal,
                createdAt: Date(),
                updatedAt: Date()
            )
            registerSuccess = true
        } catch let error as AppError {
            errorMessage = error.localizedDescription
            showError = true
        } catch {
            errorMessage = "회원가입에 실패했습니다"
            showError = true
        }
    }
}

#Preview {
    NavigationStack {
        RegisterView(showRegister: .constant(true))
            .environmentObject(AppState())
    }
}
