package main

import (
	"cs677/lab3/shared"
	"fmt"
	"os"
	"sync"
)

func main() {
	node := configureWarehouse()
	fmt.Println("Starting a Warehouse node on process", os.Getpid())
	node.StartServer() // Uncomment when StartServer is implemented
}

func configureWarehouse() *shared.Node {
	// node := new(shared.Node)
	// Check if the correct number of arguments are passed
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <ID> <Address>")
		os.Exit(1)
	}

	// Get the ID and address from arguments
	id := os.Args[1]
	address := os.Args[2]

	// Create and return the node
	intID, err := shared.String2Int(id)
	if err != nil {
		fmt.Println("Error at configureWarehouse: ", err)
	}
	node := &shared.Node{
		ID:      intID,
		Address: address,
		Role: &shared.Warehouse{
			Items:      make(map[string]int),
			GlobalLock: sync.Mutex{},
			Mutex:      sync.Mutex{},
		},
	}

	return node
}
