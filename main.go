package main

import (
	"bufio"
	"cs677/lab3/shared"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"os"
	"os/exec"
	"strings"
	"time"
)

var postNum int
var configFile = "config.txt"
var cmd *exec.Cmd
var postGroup = make(map[int][]int)
var sellerNodes = make([]int, 0)    // An empty slice of integers
var buyerNodes = make([]int, 0)     // An empty slice of buyer node IDs
var traderNodes = make(map[int]int) // Map of postID -> traderNodeID

var debug = true

// var wg sync.WaitGroup

func main() {
	// Ask user for the number of posts
	fmt.Print("Provide the number of posts: ")
	_, err := fmt.Scan(&postNum)
	if err != nil {
		log.Fatal("Error:", err)
	}

	fmt.Println("\n==== Starting Warehouse ====")
	startWarehouse()
	time.Sleep(5 * time.Second)

	fmt.Println("\n======= Starting nodes =======")
	startNodes()
	time.Sleep(5 * time.Second)

	fmt.Println("\n======== Starting election =======")

	for postID, nodes := range postGroup {
		if len(nodes) > 1 {
			fmt.Printf("INFO: Triggering election for Post %d...\n", postID)
			triggerElectionForGroup(postID)
			time.Sleep(2 * time.Second)
		} else {
			log.Printf("Post %d has %d node(s). Election not required.\n", postID, len(nodes))
		}
	}

	time.Sleep(10 * time.Second)
	fmt.Println("\n=========== Display Nodes  ===========")
	CallDisplayNodes(4)
	time.Sleep(5 * time.Second)
	CallDisplayNodes(3)
	time.Sleep(5 * time.Second)
	CallDisplayNodes(2)
	time.Sleep(5 * time.Second)
	CallDisplayNodes(1)
	time.Sleep(5 * time.Second)

	fmt.Println("\n=========== Initializing Traders =========")
	for postID, traderID := range traderNodes {
		fmt.Printf("Initializing Trader Node %d for Post %d...\n", traderID, postID)
		initializeTrader(traderID)
	}

	fmt.Println("\n=========== Checking the roles  =========")

	for _, sellerID := range sellerNodes {
		startSeller(sellerID)
		time.Sleep(2 * time.Second)

	}
	for _, buyerID := range buyerNodes {
		startBuyer(buyerID)
		time.Sleep(2 * time.Second) // Stagger the startup
	}

}

/*
Spawn the nodes on different processor based on the configuration file and randomly assign roles and posts
*/
func startNodes() {
	file, err := os.Open(configFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Split the line into components
		parts := strings.Fields(line)
		if len(parts) != 2 {
			fmt.Println("Invalid config line:", line)
			continue
		}

		id := parts[0] // Node ID

		// Assign the node to a random post
		randomPost := rand.Intn(postNum) + 1 // Random post in range [1, postNum]
		randomNum := rand.Intn(2)            // Random number: [0, 1]

		// Add the node to the postGroup
		nodeID, err := shared.String2Int(id)
		if err != nil {
			fmt.Println("Error converting node ID:", err)
			continue
		}
		postGroup[randomPost] = append(postGroup[randomPost], nodeID)

		// Assign role
		if randomNum == 0 {
			sellerNodes = append(sellerNodes, nodeID)
		} else {
			buyerNodes = append(buyerNodes, nodeID) // Assign as buyer
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Launch nodes on different processes
	for randomPost, nodes := range postGroup {
		for _, nodeID := range nodes {
			role := "buyer" // Default role
			for _, sellerID := range sellerNodes {
				if nodeID == sellerID {
					role = "seller" // Assign seller role
					break
				}
			}

			// Convert the complete `postGroup` for the post to a string
			postGroupStr := sliceToString(postGroup[randomPost])

			// Start the node process
			address := fmt.Sprintf("localhost:800%d", nodeID) // Example address logic
			cmd = exec.Command("go", "run", "node/node.go", shared.Int2String(nodeID), address, role, shared.Int2String(randomPost), postGroupStr)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err = cmd.Start() // Start the process (non-blocking)
			if err != nil {
				fmt.Printf("Error starting node %d: %v\n", nodeID, err)
				continue
			}
			if debug {
				fmt.Printf("INFO: Started Node %d with Role %s in Post %d on process %d\n", nodeID, role, randomPost, cmd.Process.Pid)
			}
		}
	}
}

/*
Start the warehouse on different processor
*/
func startWarehouse() {
	// Set up the command to run warehouse.go
	cmd := exec.Command("go", "run", "warehouse/warehouse.go", "0", "localhost:8000")

	// Redirect stdout and stderr to capture the output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute the command and wait for it to finish
	err := cmd.Start() // Start the process (non-blocking)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
}

/*
Trigger Buyer process to request to the trader
*/
func startBuyer(buyer int) {
	fmt.Println("Buyer", buyer)
	client, err := shared.GetClient(buyer)
	if err != nil {
		log.Printf("Cannot connect to Node %d: %v", buyer, err)
		return // Exit the function if the client is nil
	}
	defer client.Close()

	var req shared.Message
	var res shared.Message

	// Prepare the request
	req.Type = "START_BUYER"
	req.From = buyer
	req.To = 1 // Assuming Trader ID is 1 for this example

	err = client.Call(fmt.Sprintf("Node%d.StartBuyer", buyer), &req, &res)
	if err != nil {
		log.Printf("Error calling StartBuyer on Node %d: %v", buyer, err)
		return
	}

	fmt.Printf("Buyer %d: Response: %s", buyer, res.Message)
	fmt.Println(res.Message)
}

/*
Trigger seller process to send the requests to the trader
*/
func startSeller(seller int) {
	fmt.Println("Seller", seller)
	client, err := shared.GetClient(seller)
	if err != nil {
		log.Printf("Cannot connect to Node %d: %v", seller, err)
		return // Exit the function if the client is nil
	}
	defer client.Close()

	var req shared.Message
	var res shared.Message
	// call rpc method to start sending seller request to the trader
	err = client.Call(fmt.Sprintf("Node%d.StartSeller", seller), &req, &res)
	if err != nil {
		log.Printf("Error calling StartSeller on Node %d: %v", seller, err)
		return
	}
	fmt.Println(res.Message)
}

/*
Trigger to display the nodes
*/
func CallDisplayNodes(nodeID int) {
	client, err := shared.GetClient(nodeID)
	if err != nil {
		log.Printf("Cannot connect to Node %d: %v", nodeID, err)
		return // Exit the function if the client is nil
	}
	defer client.Close()

	var req shared.DisplayNodesRequest
	var res shared.DisplayNodesResponse

	err = client.Call(fmt.Sprintf("Node%d.DisplayNodes", nodeID), &req, &res)
	if err != nil {
		log.Printf("Error calling DisplayNodes on Node %d: %v", nodeID, err)
		return
	}

	fmt.Println(res.Message)
	fmt.Println(res.Details)
}

/*
To select trader for each group, trigger the election
*/
func triggerElectionForGroup(groupID int) error {
	if len(postGroup[groupID]) <= 1 {
		fmt.Printf("Post %d has %d node(s). Skipping election.\n", groupID, len(postGroup[groupID]))
		return nil
	}

	port := 8000 + groupID
	nodeAddr := fmt.Sprintf("localhost:%d", port)
	client, err := rpc.Dial("tcp", nodeAddr)
	if err != nil {
		fmt.Printf("Error connecting to node %s: %v\n", nodeAddr, err)
		return err
	}
	defer client.Close()

	VisitedArr := make([]int, 0)
	VisitedArr = append(VisitedArr, groupID)

	args := &shared.ElectionMessage{
		InitiatorID: groupID,
		Type:        "ELECTION",
		Post:        groupID,
		List:        make(map[int][]int),
		Visited:     VisitedArr,
	}
	reply := &shared.ElectionMessage{}
	err = client.Call(fmt.Sprintf("Node%d.SendElectionMessage", groupID), args, reply)
	if err != nil {
		fmt.Printf("Error triggering election on node %s: %v\n", nodeAddr, err)
		return nil
	}

	fmt.Printf("Election triggered for group %d on node %s\n", groupID, nodeAddr)

	// Store the Trader for this group
	traderNodes[groupID], err = shared.String2Int(reply.Leader) // Assuming `reply.Leader` contains Trader ID
	return nil
}

// ================== Helper Functions ==================
/*
Convert a slice of integers to a comma-separated string
*/
func sliceToString(slice []int) string {
	strSlice := make([]string, len(slice))
	for i, num := range slice {
		strSlice[i] = fmt.Sprintf("%d", num) // Convert each int to string
	}
	return strings.Join(strSlice, ",") // Join the strings with commas
}
func initializeTrader(traderID int) {
	client, err := shared.GetClient(traderID)
	if err != nil {
		log.Printf("Cannot connect to Node %d: %v", traderID, err)
		return
	}
	defer client.Close()

	var req shared.Message
	var res shared.Message

	// Call RPC to initialize the Trader
	req.Type = "INITIALIZE_TRADER"
	req.From = traderID

	err = client.Call(fmt.Sprintf("Node%d.InitializeTrader", traderID), &req, &res)
	if err != nil {
		log.Printf("Error calling InitializeTrader on Node %d: %v", traderID, err)
		return
	}

}
