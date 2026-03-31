import SwiftUI
import UniformTypeIdentifiers

struct ConnectionView: View {
    @Bindable var viewModel: MonitorViewModel
    @State private var showFilePicker = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    VStack(spacing: 12) {
                        Image(systemName: "shield.checkered")
                            .font(.system(size: 48))
                            .foregroundStyle(.teal)
                        Text("BW Monitor")
                            .font(.title.bold())
                        Text("BunkerWeb Security Monitor")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 20)
                    .listRowBackground(Color.clear)
                }

                Section("Kubeconfig") {
                    Button {
                        showFilePicker = true
                    } label: {
                        HStack {
                            Image(systemName: "doc.badge.plus")
                            if let name = viewModel.kubeConfigFileName {
                                Text(name)
                                    .foregroundStyle(.primary)
                            } else {
                                Text("Import Kubeconfig File")
                            }
                            Spacer()
                            if viewModel.kubeConfigYAML != nil {
                                Image(systemName: "checkmark.circle.fill")
                                    .foregroundStyle(.green)
                            }
                        }
                    }
                }

                Section("Settings") {
                    HStack {
                        Text("Namespace")
                        Spacer()
                        TextField("bunkerweb", text: $viewModel.namespace)
                            .multilineTextAlignment(.trailing)
                            .foregroundStyle(.secondary)
                    }

                    Stepper("Max Entries: \(viewModel.maxEntries)", value: $viewModel.maxEntries, in: 100...50000, step: 500)
                }

                Section {
                    Button {
                        viewModel.connect()
                    } label: {
                        HStack {
                            Spacer()
                            switch viewModel.connectionState {
                            case .connecting:
                                ProgressView()
                                    .padding(.trailing, 8)
                                Text("Connecting...")
                            default:
                                Image(systemName: "bolt.fill")
                                Text("Connect")
                            }
                            Spacer()
                        }
                        .font(.headline)
                    }
                    .disabled(viewModel.kubeConfigYAML == nil || viewModel.connectionState == .connecting)
                }

                if case .error(let msg) = viewModel.connectionState {
                    Section {
                        Label {
                            Text(msg)
                                .font(.caption)
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundStyle(.red)
                        }
                    }
                }
            }
            .navigationBarTitleDisplayMode(.inline)
            .fileImporter(
                isPresented: $showFilePicker,
                allowedContentTypes: [.data, .text, .yaml, .plainText],
                allowsMultipleSelection: false
            ) { result in
                if case .success(let urls) = result, let url = urls.first {
                    viewModel.importKubeConfig(from: url)
                }
            }
        }
    }
}

extension UTType {
    static var yaml: UTType { UTType(filenameExtension: "yaml") ?? .data }
}
