import SwiftUI

struct TagInputField: View {
    @Binding var tags: [Tag]
    @State private var inputText = ""
    @State private var suggestions: [Tag] = []
    @FocusState private var isFocused: Bool

    let onAutocomplete: (String) async -> [Tag]
    let maxTags: Int

    init(tags: Binding<[Tag]>, maxTags: Int = 10, onAutocomplete: @escaping (String) async -> [Tag]) {
        self._tags = tags
        self.maxTags = maxTags
        self.onAutocomplete = onAutocomplete
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Selected tags
            if !tags.isEmpty {
                FlowLayout(spacing: 6) {
                    ForEach(tags) { tag in
                        HStack(spacing: 4) {
                            Text("#\(tag.name)")
                                .font(.caption)
                            Image(systemName: "xmark.circle.fill")
                                .font(.caption)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(Color.blue.opacity(0.15))
                        .foregroundStyle(.blue)
                        .cornerRadius(12)
                        .onTapGesture {
                            withAnimation { tags.removeAll { $0.id == tag.id } }
                        }
                    }
                }
            }

            // Input field
            if tags.count < maxTags {
                TextField("태그 추가...", text: $inputText)
                    .textFieldStyle(.roundedBorder)
                    .focused($isFocused)
                    .onChange(of: inputText) { _, newValue in
                        Task {
                            if newValue.count >= 2 {
                                suggestions = await onAutocomplete(newValue)
                            } else {
                                suggestions = []
                            }
                        }
                    }
                    .onSubmit {
                        addTag(inputText)
                    }

                // Suggestions
                if !suggestions.isEmpty && isFocused {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 8) {
                            ForEach(suggestions) { tag in
                                TagChip(tag: tag, isSelected: false) {
                                    addTag(tag.name)
                                }
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }
            }

            Text("\(tags.count)/\(maxTags) 태그")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private func addTag(_ name: String) {
        let trimmed = name.trimmed
        guard !trimmed.isEmpty, tags.count < maxTags else { return }
        guard !tags.contains(where: { $0.name.lowercased() == trimmed.lowercased() }) else { return }

        let newTag = Tag(id: UUID(), userId: UUID(), name: trimmed, slug: trimmed.lowercased(), usageCount: 0, createdAt: Date(), updatedAt: Date())
        withAnimation {
            tags.append(newTag)
            inputText = ""
            suggestions = []
        }
    }
}

#Preview {
    TagInputField(tags: .constant([]), maxTags: 5) { _ in [] }
        .padding()
}
