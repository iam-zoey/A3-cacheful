package shared

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"os"
	"time"
)

/**
 * SendLoadRequestToTrader sends a load request from a seller to a trader.
 * It randomly selects an item and quantity from the seller's inventory to send to the trader.
 */
func (n *Node) SendLoadRequestToTrader() {
	if seller, ok := n.Role.(*Seller); ok {
		traderID, err := String2Int(n.Trader)
		if err != nil {
			log.Printf("ERROR - Node %d (Seller): Error converting Trader ID: %v", n.ID, err)
			return
		}
		client, err := GetClient(traderID)
		if err != nil {
			log.Printf("ERROR - Node %d (Seller): Unable to connect to Trader %d: %v", n.ID, traderID, err)
			return
		}
		defer client.Close()

		var req Message
		var reply Message

		req.From = n.ID
		req.To = traderID
		req.Type = "LOAD"
		req.Item = ItemList[rand.Intn(len(ItemList))]
		req.Quantity = seller.ItemsForSale[req.Item]

		err = client.Call(fmt.Sprintf("Node%d.HandleLoadRequest", traderID), &req, &reply)
		if err != nil {
			log.Printf("ERROR - Node %d (Seller): Failed to send load request to Trader %d: %v", n.ID, traderID, err)
			return
		}

		if reply.Type == "SUCCESS" {
			log.Printf("SUCCESS - Node %d (Seller from Post %d): Loaded %d units of %s to Trader %d", n.ID, n.Post, req.Quantity, req.Item, traderID)
			delete(seller.ItemsForSale, req.Item)
		} else {
			log.Printf("REJECTED - Node %d (Seller from Post %d): Failed to load goods to Trader %d: %s", n.ID, n.Post, traderID, reply.Message)
		}
	}
}

/*
 * HandleLoadRequest processes load requests received by a trader from a seller.
 * It forwards the request to the warehouse for inventory update.
 */
func (n *Node) HandleLoadRequest(req *Message, reply *Message) error {
	// Forward the load request to the Warehouse
	client, err := GetClient(0) // Warehouse is Node 0
	if err != nil {
		log.Printf("Trader %d: Unable to connect to Warehouse for LOAD request: %v", n.ID, err)
		reply.Type = "FAILURE"
		reply.Message = "Failed to connect to Warehouse"
		return err
	}
	defer client.Close()

	err = client.Call("Node0.UpdateInventory", req, reply)
	if err != nil {
		log.Printf("Trader %d: Failed to forward LOAD request to Warehouse: %v", n.ID, err)
		reply.Type = "FAILURE"
		reply.Message = "Failed to forward LOAD request to Warehouse"
		return err
	}

	log.Printf("Trader %d: Successfully forwarded LOAD request to Warehouse", n.ID)
	return nil
}

/**
 * UpdateInventory updates the inventory in the warehouse node based on the request type (LOAD or BUY).
 */
func (w *Node) UpdateInventory(req *Message, reply *Message) error {
	warehouse := w.Role.(*Warehouse)
	warehouse.GlobalLock.Lock()         // Acquire global lock
	defer warehouse.GlobalLock.Unlock() // Release global lock

	// Load the inventory from file before processing
	w.LoadInventoryFromFile("inventory.txt")

	switch req.Type {
	case "LOAD":
		w.Role.(*Warehouse).Items[req.Item] += req.Quantity
		reply.Type = "SUCCESS"
		reply.Message = fmt.Sprintf("Added %d units of %s to inventory", req.Quantity, req.Item)
		log.Printf("SUCCESS - Warehouse: Added %d units of %s. Current inventory: %v", req.Quantity, req.Item, w.Role.(*Warehouse).Items)

	case "BUY":
		if w.Role.(*Warehouse).Items[req.Item] >= req.Quantity {
			w.Role.(*Warehouse).Items[req.Item] -= req.Quantity
			reply.Type = "SUCCESS"
			reply.Message = fmt.Sprintf("Shipped %d units of %s", req.Quantity, req.Item)
			log.Printf("SUCCESS - Warehouse: Shipped %d units of %s. Current inventory: %v", req.Quantity, req.Item, w.Role.(*Warehouse).Items)
		} else {
			reply.Type = "FAILURE"
			reply.Message = fmt.Sprintf("Not enough %s in inventory. Available: %d", req.Item, w.Role.(*Warehouse).Items[req.Item])
			log.Printf("REJECTED - Warehouse: Insufficient %s in inventory. Available: %d", req.Item, w.Role.(*Warehouse).Items[req.Item])
		}

	default:
		reply.Type = "FAILURE"
		reply.Message = "Unknown request type"
		log.Printf("Warehouse: ERROR - Unknown request type: %s", req.Type)
	}

	// Save the updated inventory to file
	err := w.SaveInventoryToFile("inventory.txt")
	if err != nil {
		reply.Type = "FAILURE"
		reply.Message = "Failed to save inventory to file"
		log.Printf("ERROR - Warehouse: Error saving inventory: %v", err)
		return err
	}

	return nil
}

/*
saveInventoryToFile saves the warehouse's inventory to a file.
*/
func (w *Node) SaveInventoryToFile(filename string) error {
	w.Role.(*Warehouse).Mutex.Lock()         // Acquire local lock for file operations
	defer w.Role.(*Warehouse).Mutex.Unlock() // Release local lock

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("ERROR - Warehouse: Error opening inventory file for writing: %v", err)
		return err
	}
	defer file.Close()

	for item, quantity := range w.Role.(*Warehouse).Items {
		_, err := fmt.Fprintf(file, "%s %d\n", item, quantity)
		if err != nil {
			log.Printf(" ERROR - Warehouse: error writing inventory data: %v", err)
			return err
		}
	}
	return nil
}

/*
LoadInventoryFromFile loads the warehouse's inventory from a file.
*/
func (n *Node) LoadInventoryFromFile(filename string) {
	if w, ok := n.Role.(*Warehouse); ok {
		w.Mutex.Lock()
		defer w.Mutex.Unlock()

		file, err := os.Open(filename)
		if err != nil {
			log.Printf("ERROR - error opening inventory file: %v", err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var item string
			var quantity int
			fmt.Sscanf(scanner.Text(), "%s %d", &item, &quantity)
			w.Items[item] = quantity
		}
		log.Printf("SUCCESS - Warehouse: Loaded inventory from file: %v", w.Items)
	}
}

/*
 * StartBuyer starts the buyer node to send buy requests to the trader.
 */
func (buyer *Node) StartBuyer(args *Message, reply *Message) error {
	if _, ok := buyer.Role.(*Buyer); ok {
		// Start sending buy requests every 10 seconds
		go func() {
			ticker := time.NewTicker(10 * time.Second) // Adjust interval as needed
			defer ticker.Stop()

			for range ticker.C {
				// Randomly pick an item to buy and a random quantity
				item := ItemList[rand.Intn(len(ItemList))]
				quantity := rand.Intn(10) + 1 // Random quantity between 1 and 10
				traderID, err := String2Int(buyer.Trader)

				// Log the buyer's activity
				log.Printf("Node %d (Buyer from Post %s): Sending buy request for %d units of %s to Trader %d", buyer.ID, buyer.Trader, quantity, item, traderID)

				if err != nil {
					log.Printf("Node %d: Error converting Trader ID: %v", buyer.ID, err)
					return
				}
				// Make the buy request
				buyer.BuyFromTrader(traderID, item, quantity)
			}
		}()

		reply.Type = "SUCCESS"
		reply.Message = fmt.Sprintf("Node %d (Buyer) timer started", buyer.ID)
		return nil
	}
	reply.Type = "FAILURE"
	reply.Message = "Node is not or no longer a buyer"
	return nil
}

/*
 * BuyFromTrader sends a buy request from a buyer to a trader.
 */
func (buyer *Node) BuyFromTrader(traderID int, item string, quantity int) {
	var request Message
	var reply Message

	// Prepare buy request
	request.From = buyer.ID
	request.To = traderID
	request.Type = "BUY"
	request.Item = item
	request.Quantity = quantity

	// Send buy request to the trader
	client, err := GetClient(traderID)
	if err != nil {
		log.Printf("Buyer %d: ERROR - Unable to connect to Trader %d: %v\n", buyer.ID, traderID, err)
		return
	}

	err = client.Call(fmt.Sprintf("Node%d.HandleBuyRequest", traderID), &request, &reply)
	if err != nil {
		log.Printf("Buyer %d: ERROR -  Error sending buy request to Trader %d: %v\n", buyer.ID, traderID, err)
	} else {
		log.Printf("Buyer %d: Buy request response: %s\n", buyer.ID, reply.Message)
	}
	client.Close()
}

/*
 * HandleBuyRequest processes buy requests received by a trader from a buyer.
 */
func (n *Node) HandleBuyRequest(req *Message, reply *Message) error {
	trader, ok := n.Role.(*Trader)
	if !ok {
		return fmt.Errorf("Node %d is not a Trader", n.ID)
	}

	// Lock Trader's cache during processing
	trader.CacheLock.Lock()
	defer trader.CacheLock.Unlock()

	// Check if the Trader has enough stock
	available, exists := trader.Cache[req.Item]
	if !exists || available < req.Quantity {
		// Trader rejects based on its cache
		reply.Type = "FAILURE"
		reply.Message = fmt.Sprintf("Insufficient stock for %s. Available: %d", req.Item, available)
		log.Printf("Trader %d: Insufficient stock for %s. Available: %d, Requested: %d", n.ID, req.Item, available, req.Quantity)

		// Forward to Warehouse for verification (to check for underselling)
		go n.ForwardBuyRequestToWarehouse(req, "CHECK_UNDERSELLING")
		return nil
	}

	// Trader accepts based on its cache
	trader.Cache[req.Item] -= req.Quantity
	reply.Type = "SUCCESS"
	reply.Message = fmt.Sprintf("Request for %d %s accepted. Processing with Warehouse.", req.Quantity, req.Item)
	log.Printf("Trader %d: Accepted request for %d %s. Remaining cache: %d", n.ID, req.Quantity, req.Item, trader.Cache[req.Item])

	// Forward request to Warehouse
	go n.ForwardBuyRequestToWarehouse(req, "CHECK_OVERSELLING")
	return nil
}

/*
forward the buy request to the Warehouse and handle potential rejection
*/
func (n *Node) ForwardBuyRequestToWarehouse(req *Message, checkType string) {
	client, err := GetClient(0) // Connect to Warehouse (Node 0)
	if err != nil {
		log.Printf("Trader %d: ERROR - Unable to connect to Warehouse: %v", n.ID, err)
		return
	}
	defer client.Close()

	var warehouseReply Message
	err = client.Call("Node0.BuyProduct", req, &warehouseReply)
	if err != nil {
		log.Printf("Trader %d: ERROR - Failed to forward buy request to Warehouse: %v", n.ID, err)
		return
	}

	// Check for discrepancies
	if checkType == "CHECK_OVERSELLING" && warehouseReply.Type == "FAILURE" {
		log.Printf("OVERSELLING DETECTED - Trader %d: Warehouse rejected request for %d %s: %s", n.ID, req.Quantity, req.Item, warehouseReply.Message)
		// Update Trader cache to revert the deduction
		trader := n.Role.(*Trader)
		trader.CacheLock.Lock()
		trader.Cache[req.Item] += req.Quantity
		trader.CacheLock.Unlock()
	} else if checkType == "CHECK_UNDERSELLING" && warehouseReply.Type == "SUCCESS" {
		log.Printf("UNDERSELLING DETECTED - Trader %d: Warehouse has sufficient stock for %d %s but request was rejected by cache.", n.ID, req.Quantity, req.Item)
	}

	// Notify Buyer with final decision
	go n.NotifyBuyerOfFinalDecision(req.From, warehouseReply)
}
func (n *Node) NotifyBuyerOfFinalDecision(buyerID int, warehouseReply Message) {
	client, err := GetClient(buyerID)
	if err != nil {
		log.Printf("Trader %d: ERROR - Unable to notify Buyer %d: %v", n.ID, buyerID, err)
		return
	}
	defer client.Close()

	err = client.Call(fmt.Sprintf("Node%d.HandleRejection", buyerID), &warehouseReply, nil)
	if err != nil {
		log.Printf("Trader %d: ERROR - Failed to notify Buyer %d: %v", n.ID, buyerID, err)
	} else {
		log.Printf("Trader %d: Notified Buyer %d with final decision: %s", n.ID, buyerID, warehouseReply.Message)
	}
}

/*
For over-sellling: notify the Buyer when the Warehouse rejects the request.
*/
func (n *Node) NotifyBuyerOfRejection(buyerID int, item string, quantity int, reason string) {
	client, err := GetClient(buyerID)
	if err != nil {
		log.Printf("Trader %d: Unable to notify Buyer %d about rejection: %v", n.ID, buyerID, err)
		return
	}
	defer client.Close()

	var rejectionMsg Message
	rejectionMsg.Type = "FAILURE"
	rejectionMsg.Message = fmt.Sprintf("Request for %d %s rejected: %s", quantity, item, reason)

	err = client.Call(fmt.Sprintf("Node%d.HandleRejection", buyerID), &rejectionMsg, nil)
	if err != nil {
		log.Printf("Trader %d: Failed to notify Buyer %d about rejection: %v", n.ID, buyerID, err)
	} else {
		log.Printf("Trader %d: Notified Buyer %d about rejection of %d %s", n.ID, buyerID, quantity, item)
	}
}

/*
forwards a request from a trader to the warehouse node.
*/
func (t *Trader) ForwardToWarehouse(req *Message, reply *Message) error {
	// Connect to Warehouse
	client, err := GetClient(0) // Assuming Node 0 is the Warehouse
	if err != nil {
		log.Printf("ERROR - Unable to connect to Warehouse: %v", err)
		reply.Type = "FAILURE"
		reply.Message = "Failed to connect to Warehouse."
		return err
	}
	defer client.Close()

	// Forward the request
	err = client.Call("Node0.HandleInventoryRequest", req, reply)
	if err != nil {
		log.Printf("ERROR - Warehouse communication failed: %v", err)
		reply.Type = "FAILURE"
		reply.Message = "Failed to communicate with Warehouse."
		return err
	}

	log.Printf("INFO - Forwarded request to Warehouse. Response: %s", reply.Message)
	return nil
}

/*
Buyer’s node to handle rejection notifications.
*/
func (buyer *Node) HandleRejection(req *Message, reply *Message) error {
	log.Printf("RE-CONFIRM: Buyer %d: Received rejection notification: %s", buyer.ID, req.Message)
	reply.Type = "SUCCESS"
	reply.Message = "Rejection acknowledged"
	return nil
}

/*
 * DepositItems processes deposit requests received by a trader from a seller.
 */
func (trader *Node) DepositItems(request *Message, reply *Message) error {
	trader.Mutex.Lock()
	defer trader.Mutex.Unlock()

	fmt.Printf("  Trader (Node %d) received deposit request from Seller (Node %d): %d %s\n",
		trader.ID, request.From, request.Quantity, request.Item)

	// Process the request
	reply.Type = "SUCCESS" // Example: Accept all deposits
	fmt.Printf("  Trader (Node %d) accepted the deposit: %d %s\n", trader.ID, request.Quantity, request.Item)
	return nil
}

/*
StartSeller starts the seller node to generate items every 10 seconds.
*/
func (n *Node) StartSeller(args *Message, reply *Message) error {
	if seller, ok := n.Role.(*Seller); ok {
		// Start generating items every 5 seconds (Tg)
		// and generate 15 units (Ng) of items
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				item := ItemList[rand.Intn(len(ItemList))]
				seller.ItemsForSale[item] += 15
				fmt.Printf("INFO - Node %d (Seller from Post %d): Generated 10 units of %s. Total inventory: %v\n", n.ID, n.Post, item, seller.ItemsForSale)
				n.SendLoadRequestToTrader()

			}
		}()
		reply.Type = "SUCCESS"
		reply.Message = fmt.Sprintf("Node %d (Seller) timer started", n.ID)
		return nil
	}
	reply.Type = "FAILURE"
	reply.Message = "Node is not or no longer a seller"
	return nil
}

/*
 * BuyProduct processes the buy request received by the warehouse. This rpc method is triggered by the trader.
 */
func (w *Node) BuyProduct(req *Message, reply *Message) error {
	warehouse, ok := w.Role.(*Warehouse)
	if !ok {
		reply.Type = "FAILURE"
		reply.Message = "Node is not a Warehouse"
		return fmt.Errorf("Node %d is not a Warehouse", w.ID)
	}

	warehouse.GlobalLock.Lock()
	defer warehouse.GlobalLock.Unlock()

	available, exists := warehouse.Items[req.Item]
	if !exists || available < req.Quantity {
		reply.Type = "FAILURE"
		reply.Message = fmt.Sprintf("Not enough %s in inventory. Available: %d", req.Item, available)
		log.Printf("Warehouse %d: Insufficient stock for %s. Requested: %d, Available: %d", w.ID, req.Item, req.Quantity, available)
		return nil
	}

	warehouse.Items[req.Item] -= req.Quantity
	reply.Type = "SUCCESS"
	reply.Message = fmt.Sprintf("Shipped %d units of %s", req.Quantity, req.Item)
	log.Printf("Warehouse %d: Shipped %d units of %s. Remaining inventory: %v", w.ID, req.Quantity, req.Item, warehouse.Items)
	return nil
}

/*
SellProduct processes the sell request received by the warehouse. This rpc method is triggered by the trader.
*/
func (w *Node) SellProduct(req *Message, reply *Message) error {
	warehouse, ok := w.Role.(*Warehouse)
	if !ok {
		reply.Type = "FAILURE"
		reply.Message = "Node is not a warehouse"
		log.Printf("Node %d: Role is not a warehouse", w.ID)
		return fmt.Errorf("node %d: role is not a warehouse", w.ID)
	}

	warehouse.GlobalLock.Lock()
	defer warehouse.GlobalLock.Unlock()

	// Add the item to the inventory
	warehouse.Items[req.Item] += req.Quantity
	reply.Type = "SUCCESS"
	reply.Message = fmt.Sprintf("Added %d units of %s to inventory", req.Quantity, req.Item)
	log.Printf("SUCCESS IN LOADING - Warehouse Added %d units of %s. Current inventory: %v", req.Quantity, req.Item, warehouse.Items)

	// Save the updated inventory
	err := w.SaveInventoryToFile("inventory.txt")
	if err != nil {
		reply.Type = "FAILURE"
		reply.Message = "Failed to save inventory to file"
		log.Printf("ERROR - Warehouse can not save in the inventory: %v", err)
		return err
	}

	return nil
}

/*
StartBuying starts the buyer node to send buy requests to the trader in every 10 seconds
*/
func (buyer *Node) StartBuying(traderID int) {
	go func() {
		ticker := time.NewTicker(10 * time.Second) // Adjust the frequency as needed
		defer ticker.Stop()

		for range ticker.C {
			item := ItemList[rand.Intn(len(ItemList))] // Randomly pick an item
			quantity := rand.Intn(10) + 1              // Random quantity (1-10)
			fmt.Printf("INFO - Buyer %d from Post %d: Sending buy request for %d units of %s\n", buyer.ID, buyer.Post, quantity, item)
			buyer.BuyFromTrader(traderID, item, quantity)
		}
	}()

}

/*
Initialize Cache for the Trader.
*/
func (trader *Trader) InitializeCache() {
	trader.CacheLock.Lock()
	defer trader.CacheLock.Unlock()

	trader.Cache = make(map[string]int)

	// Fetch initial inventory from the warehouse
	client, err := GetClient(0) // Warehouse is Node 0
	if err != nil {
		log.Printf("Trader: ERROR - Unable to connect to Warehouse: %v", err)
		return
	}
	defer client.Close()

	var req Message
	var reply InventoryResponse // Custom response containing inventory data

	req.Type = "FETCH_INVENTORY"

	// Fetch inventory from Warehouse
	err = client.Call("Node0.GetInventory", &req, &reply)
	if err != nil {
		log.Printf("Trader: ERROR - Unable to fetch inventory from Warehouse: %v", err)
		return
	}

	for item, quantity := range reply.Inventory {
		trader.Cache[item] = quantity
	}
	// Update the last sync time
	trader.LastUpdated = time.Now()
	log.Printf("Trader: Initialized cache with inventory: %v", trader.Cache)
}

/*
Fetch the inventory from the Warehouse.
*/
func (w *Node) GetInventory(req *Message, reply *InventoryResponse) error {
	warehouse, ok := w.Role.(*Warehouse)
	if !ok {
		reply.Type = "FAILURE"
		return fmt.Errorf("Node is not a warehouse")
	}

	warehouse.GlobalLock.Lock()
	defer warehouse.GlobalLock.Unlock()

	reply.Inventory = warehouse.Items
	reply.Type = "SUCCESS"
	return nil
}

/*
Trigger Trader to sync its cache with the Warehouse's inventory.
*/
func (n *Node) StartPeriodicSync() {
	_, ok := n.Role.(*Trader)
	if !ok {
		log.Printf("Node %d is not a Trader", n.ID)
		return
	}

	ticker := time.NewTicker(60 * time.Second) // Sync every 45 seconds
	defer ticker.Stop()

	for range ticker.C {
		n.SyncCache()
	}
}

// SyncCache synchronizes the Trader's cache with the Warehouse's inventory.
func (n *Node) SyncCache() {
	trader, ok := n.Role.(*Trader)
	if !ok {
		log.Printf("Node %d is not a Trader", n.ID)
		return
	}

	trader.CacheLock.Lock()
	defer trader.CacheLock.Unlock()
	// Connect to Warehouse
	client, err := GetClient(0)
	if err != nil {
		log.Printf("ERROR - Trader %d: Unable to connect to Warehouse for cache sync: %v", n.ID, err)
		return
	}
	defer client.Close()

	var req Message
	var reply InventoryResponse

	// Fetch inventory from Warehouse
	req.Type = "FETCH_INVENTORY"
	err = client.Call("Node0.GetInventory", &req, &reply)
	if err != nil {
		log.Printf("ERROR - Trader %d: Cache sync failed: %v", n.ID, err)
		return
	}

	trader.Cache = reply.Inventory
	log.Printf("SYNC SUCCESS - Trader %d: Cache synchronized with Warehouse: %v", n.ID, trader.Cache)
}

/*
Set the Trader to start periodic sync.
*/
func (n *Node) InitializeTrader(req *Message, reply *Message) error {
	go n.StartPeriodicSync() // Start periodic sync in the background
	log.Printf("Trader %d: Initialized successfully. Starting periodic sync.", n.ID)

	reply.Type = "SUCCESS"
	return nil
}

/*
RetryRPC retries an RPC call for a given number of times.
*/
func RetryRPC(client *rpc.Client, method string, args interface{}, reply interface{}, retries int) error {
	var err error
	for i := 0; i < retries; i++ {
		err = client.Call(method, args, reply)
		if err == nil {
			return nil
		}
		log.Printf("RPC Error: %v. Retrying (%d/%d)...", err, i+1, retries)
		time.Sleep(2 * time.Second) // Backoff
	}
	return err
}

func (n *Node) HandleRequest(req *Message, reply *Message) error {
	switch req.Type {
	case "LOAD":
		return n.HandleLoadRequest(req, reply)
	case "BUY":
		return n.HandleBuyRequest(req, reply)
	default:
		reply.Type = "FAILURE"
		reply.Message = "Unknown request type"
		return fmt.Errorf("Trader %d received unknown request type: %s", n.ID, req.Type)
	}
}
