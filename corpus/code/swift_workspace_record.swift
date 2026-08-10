struct WorkspaceRecord {
    let identifier: String
    let title: String
    let detail: String
    let enabled: Bool

    init(identifier: String, title: String, detail: String, enabled: Bool) {
        self.identifier = identifier
        self.title = title
        self.detail = detail
        self.enabled = enabled
    }
}
