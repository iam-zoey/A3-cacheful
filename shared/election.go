package shared

import (
	"fmt"
	"log"
)

//	=========== Struct ===============
//
// ElectionMessage is used to represent messages during the election process
type ElectionMessage struct {
	InitiatorID int
	FromID      int
	ToID        int
	Visited     []int
	Type        string
	TraderAddr  int
	Leader      string
	Post        int
	List        map[int][]int
}

//  =========== Methods ===============
/*
StartElection initiates the election process for the current node
*/
func (n *Node) StartElection() {
	fmt.Printf("Node %d is starting the election process.\n", n.ID)
	var args ElectionMessage
	var reply ElectionMessage

	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	// Special case: If only one node in the post, become the leader
	if len(n.NeighborID) == 0 {
		fmt.Printf("Node %d is the only node in the post. Automatically becoming the leader.\n", n.ID)
		n.Trader = Int2String(n.ID)
		n.Role = &Trader{
			Post: Int2String(n.Post),
		}
		return
	}

	// Initiate election
	args.InitiatorID = n.ID
	args.ToID = n.NeighborID[0] // Start with the first neighbor
	args.Visited = []int{n.ID}

	n.SendElectionMessage(&args, &reply)
}

/*
Get the next neighbor in the post ring
*/
func (n *Node) getNextNeighbor(currentID int) int {
	for i, neighbor := range n.NeighborID {
		if neighbor == currentID {
			// Wrap around if at the end of the list
			return n.NeighborID[(i+1)%len(n.NeighborID)]
		}
	}
	// Default to the first neighbor if not found (edge case)
	return n.NeighborID[0]
}

/*
If a node does not respond, forward the election message to the next neighbor
*/
func (n *Node) HandleElectionNoResponse(args *ElectionMessage, reply *ElectionMessage) error {
	log.Printf("INFO: Node %d is not responding..\n", n.ID)

	// Configure the message for the next node in the ring
	neighborID := n.NeighborID[1]
	args.ToID = neighborID
	err := n.SendElectionMessage(args, reply)
	return err
}

/*
Inform the coordinator of the election results
*/
func (n *Node) InformCoordinator(args *ElectionMessage, reply *ElectionMessage) error {
	log.Printf("Node %d: Election completed. Coordinator elected: Node %s\n", n.ID, args.Leader)

	// Notify the coordinator
	n.Trader = args.Leader
	for _, visitedNode := range args.Visited {
		client, err := GetClient(visitedNode)
		if err != nil {
			log.Printf("Error connecting to Node %d: %v\n", visitedNode, err)
			continue
		}
		defer client.Close()

		args.Type = "COORDINATOR"
		args.Leader = n.Trader
		args.Post = n.Post
		err = client.Call(fmt.Sprintf("Node%d.ReceiveCoordinator", visitedNode), args, reply)

		if err != nil {
			log.Printf("Error notifying Node %d: %v\n", visitedNode, err)
		} else {
			log.Printf("Successfully notified Node %d about the coordinator.\n", visitedNode)
		}
	}
	return nil
}

/*
Receive the coordinator message and update the node's leader
*/
func (n *Node) ReceiveCoordinator(args *ElectionMessage, reply *ElectionMessage) error {

	// Update the node's leader field if it belongs to the same post
	if n.Post == args.Post {
		n.Trader = args.Leader
		log.Printf("Node %d: Trader assigned as Node %s for Post %d", n.ID, n.Trader, n.Post)
	}

	// If this node is the new Trader, update its role
	intID, _ := String2Int(args.Leader)
	if n.ID == intID {
		n.Role = &Trader{
			Post: Int2String(n.Post),
		}
	}
	return nil
}

/*
Receive the election message and forward it to the next neighbor
*/
func (n *Node) ReceiveElectionMessage(args *ElectionMessage, reply *ElectionMessage) error {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()

	// Add the current node to the visited list
	if !contains(args.Visited, n.ID) {
		args.Visited = append(args.Visited, n.ID)
	}

	// If the election has completed the ring
	if args.InitiatorID == args.ToID {
		LeaderID := n.findHighestID(args.Visited)
		args.Leader = Int2String(LeaderID)
		log.Printf("Node %d: Elected leader is Node %d for Post %d.\n", n.ID, LeaderID, n.Post)
		return n.InformCoordinator(args, reply)
	}

	// Forward the election message to the next neighbor
	args.ToID = n.getNextNeighbor(args.ToID)
	return n.SendElectionMessage(args, reply)
}

/*
Send the election message to the next neighbor
*/
func (n *Node) SendElectionMessage(args *ElectionMessage, reply *ElectionMessage) error {
	// Skip sending election messages if this node is already the Trader
	if n.Trader == Int2String(n.ID) {
		log.Printf("Node %d is already the Trader. Skipping election message forwarding.\n", n.ID)
		return nil
	}

	// Detect if neighbors are misconfigured
	if len(n.NeighborID) == 0 {
		log.Printf("Node %d has no neighbors to send election message.\n", n.ID)
		return fmt.Errorf("node %d has no valid neighbors", n.ID)
	}

	// Determine the next neighbor in the ring
	args.ToID = n.getNextNeighbor(n.ID)
	args.Post = n.Post

	// Check if the message is returning to the initiator
	if args.InitiatorID == args.ToID {
		LeaderID := n.findHighestID(args.Visited)
		args.Leader = Int2String(LeaderID)
		return n.InformCoordinator(args, reply)
	}

	// Attempt to forward the election message
	client, err := GetClient(args.ToID)
	if err != nil {
		log.Printf("Error connecting to Node %d: %v. Attempting next neighbor.\n", args.ToID, err)
		return n.HandleElectionNoResponse(args, reply)
	}

	methodName := fmt.Sprintf("Node%d.ReceiveElectionMessage", args.ToID)
	err = client.Call(methodName, args, reply)
	if err != nil {
		log.Printf("Error forwarding message to Node %d: %v\n", args.ToID, err)
		return n.HandleElectionNoResponse(args, reply)
	}

	return nil
}

// ====================== HELPER FUNCTION for leader selection ==========================

/*
Find the highest ID in the visited list
*/
func (n *Node) findHighestID(visited []int) int {
	highest := -1
	for _, id := range visited {
		if id > highest {
			highest = id
		}
	}
	return highest
}

/*
Check if an item is in the slice
*/
func contains(slice []int, item int) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
