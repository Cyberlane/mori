func accountPanel() -> some View { VStack { Text("Account"); Button("Delete") { deleteAccount() } } }
func projectPanel() -> some View { VStack { Text("Project"); Button("Archive") { archiveProject() } } }
