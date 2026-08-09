func clampValue(_ value: Int, minimum: Int, maximum: Int) -> Int {
    if value < minimum {
        return minimum
    }
    if value > maximum {
        return maximum
    }
    return value
}
