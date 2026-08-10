struct AccountPanel {
    let accounts: [Account]
    let disconnect: (Account) -> Void

    func accountPanel() -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Accounts").font(.headline)
                Spacer()
                Text("Local only").font(.caption)
            }
            ForEach(accounts) { account in
                HStack(spacing: 8) {
                    Image(systemName: account.serviceSymbol)
                    VStack(alignment: .leading) {
                        Text(account.displayName).font(.subheadline)
                        Text(account.scope).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button("Disconnect") { disconnect(account) }
                }
            }
        }
        .padding(16)
        .background(.quaternary, in: RoundedRectangle(cornerRadius: 12))
    }
}
