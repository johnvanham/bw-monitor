import SwiftUI

struct FilterSheet: View {
    @State private var filter: Filter
    let onApply: (Filter) -> Void
    @Environment(\.dismiss) private var dismiss

    init(filter: Filter, onApply: @escaping (Filter) -> Void) {
        _filter = State(initialValue: filter)
        self.onApply = onApply
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Search") {
                    HStack {
                        Image(systemName: "network")
                            .foregroundStyle(.secondary)
                        TextField("IP address (substring)", text: $filter.ip)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                    }

                    HStack {
                        Image(systemName: "globe")
                            .foregroundStyle(.secondary)
                        TextField("Country code (e.g. GB,US)", text: $filter.country)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.characters)
                    }

                    HStack {
                        Image(systemName: "server.rack")
                            .foregroundStyle(.secondary)
                        TextField("Server name (substring)", text: $filter.server)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                    }
                }

                Section("Date Range") {
                    Toggle("From Date", isOn: Binding(
                        get: { filter.dateFrom != nil },
                        set: { filter.dateFrom = $0 ? Date().addingTimeInterval(-86400) : nil }
                    ))
                    if let from = Binding(optionalBinding: $filter.dateFrom) {
                        DatePicker("From", selection: from, displayedComponents: [.date, .hourAndMinute])
                    }

                    Toggle("To Date", isOn: Binding(
                        get: { filter.dateTo != nil },
                        set: { filter.dateTo = $0 ? Date() : nil }
                    ))
                    if let to = Binding(optionalBinding: $filter.dateTo) {
                        DatePicker("To", selection: to, displayedComponents: [.date, .hourAndMinute])
                    }
                }

                if filter.isActive {
                    Section {
                        Button("Clear All Filters", role: .destructive) {
                            filter.clear()
                        }
                    }
                }
            }
            .navigationTitle("Filter")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Apply") {
                        onApply(filter)
                        dismiss()
                    }
                    .bold()
                }
            }
        }
    }
}

// Helper to create a Binding from an optional value
extension Binding {
    init?(optionalBinding: Binding<Value?>) {
        guard optionalBinding.wrappedValue != nil else { return nil }
        self.init(
            get: { optionalBinding.wrappedValue! },
            set: { optionalBinding.wrappedValue = $0 }
        )
    }
}
