import SwiftUI

struct ContentView: View {
    @State private var viewModel = MonitorViewModel()

    var body: some View {
        Group {
            if viewModel.connectionState.isConnected {
                MainTabView(viewModel: viewModel)
            } else {
                ConnectionView(viewModel: viewModel)
            }
        }
        .animation(.default, value: viewModel.connectionState.isConnected)
    }
}

struct MainTabView: View {
    @Bindable var viewModel: MonitorViewModel
    @State private var selectedTab = 0

    var body: some View {
        TabView(selection: $selectedTab) {
            NavigationStack {
                ReportsListView(viewModel: viewModel)
            }
            .tabItem {
                Label("Reports", systemImage: "shield.lefthalf.filled")
            }
            .tag(0)

            NavigationStack {
                BansListView(viewModel: viewModel)
            }
            .tabItem {
                Label("Bans", systemImage: "hand.raised.fill")
            }
            .tag(1)
        }
        .tint(.teal)
    }
}
