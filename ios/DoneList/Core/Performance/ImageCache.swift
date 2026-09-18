import SwiftUI
import Combine

/// 이미지 캐싱 시스템
actor ImageCache {
    static let shared = ImageCache()

    private var cache: NSCache<NSString, UIImage> = {
        let cache = NSCache<NSString, UIImage>()
        cache.countLimit = 100
        cache.totalCostLimit = 50 * 1024 * 1024 // 50MB
        return cache
    }()

    private var inProgressLoads: [String: Task<UIImage?, Error>] = [:]

    private init() {}

    func image(for url: URL) async throws -> UIImage? {
        let key = url.absoluteString as NSString

        // Check memory cache
        if let cached = cache.object(forKey: key) {
            return cached
        }

        // Check if already loading
        if let existingTask = inProgressLoads[url.absoluteString] {
            return try await existingTask.value
        }

        // Start new load
        let task = Task<UIImage?, Error> {
            let (data, _) = try await URLSession.shared.data(from: url)
            guard let image = UIImage(data: data) else { return nil }
            cache.setObject(image, forKey: key, cost: data.count)
            return image
        }

        inProgressLoads[url.absoluteString] = task
        defer { inProgressLoads.removeValue(forKey: url.absoluteString) }

        return try await task.value
    }

    func clearCache() {
        cache.removeAllObjects()
    }

    func removeImage(for url: URL) {
        cache.removeObject(forKey: url.absoluteString as NSString)
    }
}

/// 비동기 이미지 로딩 뷰
struct CachedAsyncImage<Content: View, Placeholder: View>: View {
    let url: URL?
    let content: (Image) -> Content
    let placeholder: () -> Placeholder

    @State private var image: UIImage?
    @State private var isLoading = false

    init(
        url: URL?,
        @ViewBuilder content: @escaping (Image) -> Content,
        @ViewBuilder placeholder: @escaping () -> Placeholder
    ) {
        self.url = url
        self.content = content
        self.placeholder = placeholder
    }

    var body: some View {
        Group {
            if let image = image {
                content(Image(uiImage: image))
            } else {
                placeholder()
            }
        }
        .task(id: url) {
            await loadImage()
        }
    }

    private func loadImage() async {
        guard let url = url else { return }

        isLoading = true
        defer { isLoading = false }

        do {
            image = try await ImageCache.shared.image(for: url)
        } catch {
            Logger.shared.error("Failed to load image", error: error, category: .general)
        }
    }
}
