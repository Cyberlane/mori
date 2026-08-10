func clampValue(_ value: Int, minimum: Int, maximum: Int) -> Int {
    if value < minimum { return minimum }
    if value > maximum { return maximum }
    return value
}

func sumValues(_ values: [Int]) -> Int {
    var total = 0
    for value in values { total += value }
    return total
}
