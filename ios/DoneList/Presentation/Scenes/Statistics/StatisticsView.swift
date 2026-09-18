import SwiftUI

struct StatisticsView: View {
    var body: some View {
        NavigationStack {
            VStack {
                Text("Statistics View")
                    .font(.largeTitle)
            }
            .navigationTitle("Statistics")
        }
    }
}

#Preview {
    StatisticsView()
}
