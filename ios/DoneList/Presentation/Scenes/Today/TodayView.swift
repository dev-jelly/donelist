import SwiftUI

struct TodayView: View {
    var body: some View {
        NavigationStack {
            VStack {
                Text("Today View")
                    .font(.largeTitle)
            }
            .navigationTitle("Today")
        }
    }
}

#Preview {
    TodayView()
}
