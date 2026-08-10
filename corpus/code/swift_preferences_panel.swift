struct PreferencesPanel {
    let categories: [PreferenceCategory]
    let close: () -> Void

    func preferencesPanel() -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Preferences").font(.headline)
                Spacer()
                Button("Done", action: close)
            }
            ForEach(categories) { category in
                HStack(spacing: 8) {
                    Image(systemName: category.symbol)
                    VStack(alignment: .leading) {
                        Text(category.title).font(.subheadline)
                        Text(category.detail).foregroundStyle(.secondary)
                    }
                    Spacer()
                }
            }
        }
        .padding(16)
        .background(.quaternary, in: RoundedRectangle(cornerRadius: 12))
    }
}
