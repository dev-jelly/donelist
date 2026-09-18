import SwiftUI

struct TagManagementView: View {
    @StateObject private var viewModel = TagManagementViewModel()
    @State private var newTagName = ""

    var body: some View {
        List {
            Section {
                HStack {
                    TextField("새 태그", text: $newTagName)
                    Button {
                        Task {
                            await viewModel.createTag(name: newTagName)
                            newTagName = ""
                        }
                    } label: {
                        Image(systemName: "plus.circle.fill")
                            .foregroundStyle(.blue)
                    }
                    .disabled(newTagName.isEmpty)
                }
            }

            Section("내 태그") {
                ForEach(viewModel.tags) { tag in
                    HStack {
                        Text("#\(tag.name)")
                            .font(.body)
                        Spacer()
                        Text("\(tag.usageCount)회 사용")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                .onDelete { indexSet in
                    // TODO: Implement delete
                }
            }

            if !viewModel.popularTags.isEmpty {
                Section("인기 태그") {
                    FlowLayout(spacing: 8) {
                        ForEach(viewModel.popularTags) { tag in
                            TagChip(tag: tag, isSelected: false) {
                                // Add to my tags
                            }
                        }
                    }
                    .padding(.vertical, 4)
                }
            }
        }
        .navigationTitle("태그 관리")
        .task {
            await viewModel.loadTags()
        }
    }
}

struct TagChip: View {
    let tag: Tag
    let isSelected: Bool
    let onTap: () -> Void

    var body: some View {
        Text("#\(tag.name)")
            .font(.subheadline)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(isSelected ? Color.blue : Color.gray.opacity(0.15))
            .foregroundStyle(isSelected ? .white : .primary)
            .cornerRadius(16)
            .onTapGesture(perform: onTap)
    }
}

struct FlowLayout: Layout {
    var spacing: CGFloat = 8

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let result = FlowResult(in: proposal.width ?? 0, subviews: subviews, spacing: spacing)
        return result.size
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let result = FlowResult(in: bounds.width, subviews: subviews, spacing: spacing)
        for (index, subview) in subviews.enumerated() {
            subview.place(at: CGPoint(x: bounds.minX + result.positions[index].x, y: bounds.minY + result.positions[index].y), proposal: .unspecified)
        }
    }

    struct FlowResult {
        var size: CGSize = .zero
        var positions: [CGPoint] = []

        init(in width: CGFloat, subviews: Subviews, spacing: CGFloat) {
            var x: CGFloat = 0
            var y: CGFloat = 0
            var rowHeight: CGFloat = 0

            for subview in subviews {
                let size = subview.sizeThatFits(.unspecified)
                if x + size.width > width && x > 0 {
                    x = 0
                    y += rowHeight + spacing
                    rowHeight = 0
                }
                positions.append(CGPoint(x: x, y: y))
                rowHeight = max(rowHeight, size.height)
                x += size.width + spacing
            }

            self.size = CGSize(width: width, height: y + rowHeight)
        }
    }
}

@MainActor
final class TagManagementViewModel: ObservableObject {
    @Published var tags: [Tag] = []
    @Published var popularTags: [Tag] = []
    @Published var isLoading = false

    func loadTags() async {
        isLoading = true
        defer { isLoading = false }

        // Mock data
        tags = [
            Tag(id: UUID(), userId: UUID(), name: "집중", slug: "focus", usageCount: 15, createdAt: Date(), updatedAt: Date()),
            Tag(id: UUID(), userId: UUID(), name: "회의", slug: "meeting", usageCount: 8, createdAt: Date(), updatedAt: Date()),
            Tag(id: UUID(), userId: UUID(), name: "학습", slug: "learning", usageCount: 12, createdAt: Date(), updatedAt: Date())
        ]

        popularTags = [
            Tag(id: UUID(), userId: UUID(), name: "생산성", slug: "productivity", usageCount: 100, createdAt: Date(), updatedAt: Date()),
            Tag(id: UUID(), userId: UUID(), name: "아침루틴", slug: "morning-routine", usageCount: 85, createdAt: Date(), updatedAt: Date())
        ]
    }

    func createTag(name: String) async {
        let newTag = Tag(id: UUID(), userId: UUID(), name: name, slug: name.lowercased(), usageCount: 0, createdAt: Date(), updatedAt: Date())
        tags.insert(newTag, at: 0)
    }
}

#Preview {
    NavigationStack {
        TagManagementView()
    }
}
