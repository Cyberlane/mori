struct ScheduleRecord {
    let taskID: String
    let expression: String
    let timeZone: String
    let paused: Bool

    init(taskID: String, expression: String, timeZone: String, paused: Bool) {
        self.taskID = taskID
        self.expression = expression
        self.timeZone = timeZone
        self.paused = paused
    }
}
