import SwiftUI

struct CategoryManagementView: View {
    @StateObject private var viewModel = CategoryManagementViewModel()
    @State private var showAddSheet = false
    @State private var editingCategory: Category?

    var body: some View {
        List {
            ForEach(viewModel.categories) { category in
                CategoryRow(category: category)
                    .contentShape(Rectangle())
                    .onTapGesture {
                        editingCategory = category
                    }
            }
            .onDelete { indexSet in
                Task {
                    for index in indexSet {
                        await viewModel.deleteCategory(viewModel.categories[index])
                    }
                }
            }
        }
        .navigationTitle("카테고리 관리")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    showAddSheet = true
                } label: {
                    Image(systemName: "plus")
                }
            }
        }
        .sheet(isPresented: $showAddSheet) {
            CategoryEditSheet(category: nil) { name, color, icon in
                Task {
                    await viewModel.createCategory(name: name, color: color, icon: icon)
                }
            }
        }
        .sheet(item: $editingCategory) { category in
            CategoryEditSheet(category: category) { name, color, icon in
                Task {
                    await viewModel.updateCategory(category, name: name, color: color, icon: icon)
                }
            }
        }
        .task {
            await viewModel.loadCategories()
        }
    }
}

struct CategoryRow: View {
    let category: Category

    var body: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(category.displayColor)
                .frame(width: 32, height: 32)
                .overlay {
                    if let icon = category.icon {
                        Image(systemName: icon)
                            .font(.system(size: 14))
                            .foregroundStyle(.white)
                    }
                }

            Text(category.name)
                .font(.body)

            Spacer()
        }
        .padding(.vertical, 4)
    }
}

struct CategoryEditSheet: View {
    @Environment(\.dismiss) private var dismiss
    let category: Category?
    let onSave: (String, String?, String?) -> Void

    @State private var name: String = ""
    @State private var selectedColor: Color = .blue
    @State private var selectedIcon: String = "folder.fill"

    private let predefinedColors: [Color] = [
        .red, .orange, .yellow, .green, .mint, .teal, .cyan, .blue, .indigo, .purple, .pink, .brown
    ]

    private let predefinedIcons = [
        "folder.fill", "star.fill", "heart.fill", "flag.fill", "bookmark.fill",
        "tag.fill", "briefcase.fill", "house.fill", "person.fill", "gear"
    ]

    var body: some View {
        NavigationStack {
            Form {
                Section("이름") {
                    TextField("카테고리 이름", text: $name)
                }

                Section("색상") {
                    LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 6), spacing: 12) {
                        ForEach(predefinedColors, id: \.self) { color in
                            Circle()
                                .fill(color)
                                .frame(width: 40, height: 40)
                                .overlay {
                                    if selectedColor == color {
                                        Image(systemName: "checkmark")
                                            .foregroundStyle(.white)
                                            .fontWeight(.bold)
                                    }
                                }
                                .onTapGesture { selectedColor = color }
                        }
                    }
                    .padding(.vertical, 8)
                }

                Section("아이콘") {
                    LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 5), spacing: 12) {
                        ForEach(predefinedIcons, id: \.self) { icon in
                            Image(systemName: icon)
                                .font(.title2)
                                .frame(width: 44, height: 44)
                                .background(selectedIcon == icon ? selectedColor.opacity(0.2) : Color.clear)
                                .cornerRadius(8)
                                .onTapGesture { selectedIcon = icon }
                        }
                    }
                    .padding(.vertical, 8)
                }
            }
            .navigationTitle(category == nil ? "새 카테고리" : "카테고리 수정")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("취소") { dismiss() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("저장") {
                        onSave(name, selectedColor.hexString, selectedIcon)
                        dismiss()
                    }
                    .disabled(name.isEmpty)
                }
            }
            .onAppear {
                if let category = category {
                    name = category.name
                    selectedColor = category.displayColor
                    selectedIcon = category.icon ?? "folder.fill"
                }
            }
        }
    }
}

@MainActor
final class CategoryManagementViewModel: ObservableObject {
    @Published var categories: [Category] = []
    @Published var isLoading = false
    @Published var errorMessage: String?

    func loadCategories() async {
        isLoading = true
        defer { isLoading = false }

        // Mock data - TODO: Replace with repository
        categories = [
            Category(id: UUID(), userId: UUID(), name: "업무", color: "#3B82F6", icon: "briefcase.fill", createdAt: Date(), updatedAt: Date()),
            Category(id: UUID(), userId: UUID(), name: "개인", color: "#10B981", icon: "person.fill", createdAt: Date(), updatedAt: Date()),
            Category(id: UUID(), userId: UUID(), name: "운동", color: "#F59E0B", icon: "figure.run", createdAt: Date(), updatedAt: Date())
        ]
    }

    func createCategory(name: String, color: String?, icon: String?) async {
        let newCategory = Category(
            id: UUID(), userId: UUID(), name: name, color: color, icon: icon,
            createdAt: Date(), updatedAt: Date()
        )
        categories.append(newCategory)
    }

    func updateCategory(_ category: Category, name: String, color: String?, icon: String?) async {
        if let index = categories.firstIndex(where: { $0.id == category.id }) {
            categories[index] = Category(
                id: category.id, userId: category.userId, name: name, color: color, icon: icon,
                createdAt: category.createdAt, updatedAt: Date()
            )
        }
    }

    func deleteCategory(_ category: Category) async {
        categories.removeAll { $0.id == category.id }
    }
}

#Preview {
    NavigationStack {
        CategoryManagementView()
    }
}
