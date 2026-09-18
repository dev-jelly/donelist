import SwiftUI

struct TimelineView: View {
    var body: some View {
        NavigationStack {
            VStack {
                Text("Timeline View")
                    .font(.largeTitle)
            }
            .navigationTitle("Timeline")
        }
    }
}

#Preview {
    TimelineView()
}
