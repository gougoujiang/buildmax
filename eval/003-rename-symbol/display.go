package calc

import "fmt"

// PrintSum prints the sum of a and b.
func PrintSum(a, b int) {
	fmt.Printf("ComputeSum(%d, %d) = %d\n", a, b, ComputeSum(a, b))
}
