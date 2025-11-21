package main

import "fmt"

func processUser(name string, age int) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if age < 0 {
		return fmt.Errorf("age cannot be negative")
	}
	fmt.Printf("Processing user: %s, age: %d\n", name, age)
	return nil
}

func validateInput(input string) bool {
	return len(input) > 0 && input != ""
}
