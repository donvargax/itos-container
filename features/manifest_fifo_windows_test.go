package features

import "fmt"

func manifestFIFO(path string) error {
	return fmt.Errorf("FIFO fixture is unavailable on Windows: %s", path)
}
