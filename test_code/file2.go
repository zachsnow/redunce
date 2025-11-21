package main

import "fmt"

func processProduct(title string, price float64) error {
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if price < 0 {
		return fmt.Errorf("price cannot be negative")
	}
	fmt.Printf("Processing product: %s, price: %.2f\n", title, price)
	return nil
}

func checkValue(value string) bool {
	return len(value) > 0 && value != ""
}
