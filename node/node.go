package main

import (
	"cs677/lab3/shared"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var AvailabileForSales = "inventory.txt"
var PendingSales = "pending.txt"
var LeaderID = 0

// var totalNum = 0

func main() {
	node := configureNode()
	node.StartServer()
	time.Sleep(2 * time.Second)
}

// Wrapper type for shared.Node
type Node struct {
	*shared.Node
	Mutex sync.Mutex
}

// ElectionMessage is used to represent messages during the election process
type ElectionMessage struct {
	InitiatorID int
	FromID      int
	ToID        int
	Clock       int
	Visited     []int
	Type        string
	TraderAddr  int
	Leader      string
}

var SaltPrice = 1
var FishPrice = 2
var BoarPrice = 3

func configureNode() *shared.Node {
	// Check if the correct number of arguments are passed
	if len(os.Args) < 6 {
		fmt.Println("Usage: go run node.go <ID> <Address> <Role> <PostNum>")
		os.Exit(1)
	}

	// Get the ID and address from arguments
	id, err := shared.String2Int(os.Args[1])
	if err != nil {
		fmt.Println("Error at configureNode: ", err)
	}
	address := os.Args[2]
	role := os.Args[3]
	postNum := os.Args[4]
	postGroupStr := os.Args[5] // Assuming postGroupStr is passed as the 6th argument

	// Parse the post group and create neighbors
	postGroup := parsePostGroup(postGroupStr)
	neighbors := createRingNeighbors(postGroup, id)

	intPost, err := shared.String2Int(postNum)
	if err != nil {
		fmt.Println("Error at configureNode: ", err)
	}
	// Initialize the node
	node := &shared.Node{
		ID:         id,
		Address:    address,
		NeighborID: neighbors,
		Post:       intPost,
	}

	// Handle single-node post case
	if len(neighbors) == 0 {
		role = "trader"
		fmt.Printf("  Node %d is the only node in the post. Assigning Trader role.\n", id)
	}
	// Assign role based on input
	node.AssignRole(role, postNum)

	return node
}

func createRingNeighbors(postGroup []int, id int) []int {
	if len(postGroup) == 0 || (len(postGroup) == 1 && postGroup[0] == id) {
		// No neighbors if single-node post
		return []int{}
	}

	// Sort the postGroup for consistent neighbor assignment
	sort.Ints(postGroup)

	// Find the position of the current node
	index := -1
	for i, nodeID := range postGroup {
		if nodeID == id {
			index = i
			break
		}
	}

	if index == -1 {
		fmt.Printf("Error: Node ID %d not found in postGroup.\n", id)
		return []int{}
	}

	// Determine neighbors in a circular ring
	prevNeighbor := postGroup[(index-1+len(postGroup))%len(postGroup)]
	nextNeighbor := postGroup[(index+1)%len(postGroup)]

	// Return neighbors
	return []int{prevNeighbor, nextNeighbor}
}

// func deleteCurrentID(postGroup []int, id int) []int {
// 	result := []int{}
// 	for _, neighborID := range postGroup {
// 		if neighborID != id {
// 			result = append(result, neighborID)
// 		}
// 	}
// 	return result
// }

func parsePostGroup(postGroupStr string) []int {
	strSlice := strings.Split(postGroupStr, ",") // Split string by commas
	intSlice := make([]int, len(strSlice))
	for i, s := range strSlice {
		num, err := strconv.Atoi(s) // Convert string to int
		if err != nil {
			fmt.Printf("Error parsing post group: %v\n", err)
			continue
		}
		intSlice[i] = num
	}
	return intSlice
}
