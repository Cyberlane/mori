package fixture

func Receive(queue []int) (int, []int) {
	if len(queue) == 0 {
		return 0, queue
	}
	value := queue[0]
	queue = queue[1:]
	return value, queue
}
func Peek(queue []int) (int, []int) {
	if len(queue) == 0 {
		return 0, queue
	}
	value := queue[0]
	return value, queue
}
