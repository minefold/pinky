package main

// used to get port numbers to bind to
func NewIntPool(min int, count int, step int, reserved []int) chan int {
	pool := make(chan int, count+1) // +1 means the last write doesn't block

	// fill pool
	for i := 0; i < count; i++ {
		num := min + (i * step)
		if indexOfInt(reserved, num) == -1 {
			pool <- num
		}
	}

	return pool
}

func indexOfInt(slice []int, value int) int {
	for p, v := range slice {
		if v == value {
			return p
		}
	}
	return -1
}
