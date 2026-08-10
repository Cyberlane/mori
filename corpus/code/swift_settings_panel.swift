struct SettingsPanel {
    let sections: [SettingsSection]
    let dismiss: () -> Void

    func settingsPanel() -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Settings").font(.headline)
                Spacer()
                Button("Close", action: dismiss)
            }
            ForEach(sections) { section in
                HStack(spacing: 8) {
                    Image(systemName: section.symbol)
                    VStack(alignment: .leading) {
                        Text(section.title).font(.subheadline)
                        Text(section.detail).foregroundStyle(.secondary)
                    }
                    Spacer()
                }
            }
        }
        .padding(16)
        .background(.quaternary, in: RoundedRectangle(cornerRadius: 12))
    }
}
