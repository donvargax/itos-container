package disk

func lockMode(exclusive bool) int {
	if exclusive {
		return 2
	}
	return 1
}
