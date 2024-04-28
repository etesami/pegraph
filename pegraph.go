package pegraph

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/dominikbraun/graph"
	customgraph "github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
)

// PerfData represents performance data
type PerfData struct {
	// Latency as measured in milliseconds
	Latency float64
	// Bandwidth as measured in MB per second
	Bandwidth int
}

// Graph represents a graph data structure
type Graph struct {
	Nodes []*Node
	Edges map[string][]string
}

// Node represents a node in the graph
type Node struct {
	ID       string
	Name     string
	Location *Location
}

// Location represents a location where nodes can be placed
type Location struct {
	Name  string
	Nodes []*Node
}

// policyFunc is a placeholder policy function that always returns true
func policyFunc(u, v interface{}, puv interface{}) bool {
	return true
}

// Pseudo Code:
// Create initial graph
// create a new instance for each location in locationReqList and add to g.Nodes
// Add edges to g.Nodes according to the original graph
// Add rest of nodes and corresponding edges

// Iterate through g.Nodes:
// if the node is a source of connection:
//   if corresponding (destination) node exists in g.Nodes
// 	   if an edge exists between the two nodes: if not, add an edge
//	 else
//     create a new instance and add an edge
// else: (there is no edges with the node as source)
//   check if the node is a destination of any source
//   if yes, add an edge from source to the node
// For newly created nodes
//   if the node is a source of connection:
//     if corresponding (destination) node exists in g.Nodes
//       if an edge exists between the two nodes: if not, add an edge
//     else
//       create a new instance and add an edge
//   else: (there is no edges with the node as source) // this section may never happen
//     check if the node is a destination of any source
//     if yes, add an edge from source to the node

// generateUniqueID generates a unique ID for a node based on its name and location
func generateUniqueID(uuid string, objName string, loc string) string {
	id := "obj"
	if objName != "" {
		id = id + "_" + objName
	}
	if loc != "" {
		id = id + "_" + loc
	}
	return id + "_" + uuid
}

// createInstance creates a new node with a unique ID and assigns it to the given location
func createInstance(name string, loc *Location) *Node {
	nodeUUID := uuid.New().String()
	return &Node{ID: generateUniqueID(nodeUUID, name, loc.Name), Name: name, Location: loc}
}

// createInstances creates instances of nodes for each location in the location registry (Lr)
func createInstances(nodes []*Node, locationReqList map[string][]*Location) []*Node {
	var newNodes []*Node
	for _, node := range nodes {
		for _, loc := range locationReqList[node.Name] {
			newNode := createInstance(node.Name, loc)
			newNodes = append(newNodes, newNode)
			loc.Nodes = append(loc.Nodes, newNode)
		}
	}
	return newNodes
}

// addEdges adds edges to the graph based on the original graph and performance data
func addEdges(graph *Graph, newNodes []*Node, perfData map[string]PerfData) map[string][]string {
	edges := make(map[string][]string)
	for _, node1 := range newNodes {
		for _, node2 := range newNodes {
			if node1.ID != node2.ID {
				if connections, ok := graph.Edges[node1.Name]; ok {
					for _, connectedNode := range connections {
						if connectedNode == node2.Name {
							edges[node1.ID] = append(edges[node1.ID], node2.ID)
						}
					}
				}
			}
		}
	}
	return edges
}

// generateInitialPEAGraph generates the initial policy-enriched application graph
func (g *Graph) GenerateInitialPEAGraph(
	locationReqList map[string][]*Location,
	locationAllowlist map[string][]*Location,
	perfData map[string]PerfData,
	locations []*Location) *Graph {

	newNodes := createInstances(g.Nodes, locationReqList)
	newEdges := addEdges(g, newNodes, perfData)

	return &Graph{
		Nodes: newNodes,
		Edges: newEdges,
	}
}

// generatePEAGraph generates the policy-enriched application graph
func (g *Graph) GeneratePEAGraph(appGraph *Graph, locationAllowlist map[string][]*Location) {
	for _, node := range g.Nodes {
		_ = processNode(g, appGraph, node, locationAllowlist)
	}
}

func processNode(g *Graph, appGraph *Graph, node *Node, locationAllowlist map[string][]*Location) []*Node {
	fmt.Println("Checking", node.ID[:14])
	fmt.Println("--- Find corresponding nodes:")
	var addedNodes []*Node
	if connections, ok := appGraph.Edges[node.Name]; ok {
		addedNodes = processConnections(g, appGraph, node, connections, locationAllowlist)
	} else {
		fmt.Println(" No edge, check any sources")
		checkSources(g, appGraph, node)
	}
	return addedNodes
}

func processConnections(g *Graph, appGraph *Graph, node *Node, connections []string, locationAllowlist map[string][]*Location) []*Node {
	var addedNodes []*Node
	for _, connectedNodeName := range connections {
		fmt.Println("    [Checking if (", connectedNodeName, ") is in g.Nodes] ")
		if idxs := containsNode(g.Nodes, connectedNodeName); len(idxs) > 0 {
			processExistingNode(g, node, idxs)
		} else {
			fmt.Print("      [Not Found] creating a new instance and add edge")
			newNode := createInstance(connectedNodeName, locationAllowlist[connectedNodeName][rand.Intn(len(locationAllowlist[connectedNodeName]))])
			fmt.Println(" [Created]: ", newNode.ID[:14])
			addedNodes = append(addedNodes, newNode)
			g.Nodes = append(g.Nodes, newNode)
			g.Edges[node.ID] = append(g.Edges[node.ID], newNode.ID)
			_ = processNode(g, appGraph, newNode, locationAllowlist)
		}
	}
	return addedNodes
}

func processExistingNode(g *Graph, node *Node, connectedNodeIdxs []int) {
	for _, idx := range connectedNodeIdxs {
		connectedNode := g.Nodes[idx]
		fmt.Print("      [Found] checking edge,")
		if g.Edges[node.ID] == nil || !containsEdge(g.Edges, node.ID, connectedNode.ID) {
			fmt.Println("      [Not Found] adding edge,")
			g.Edges[node.ID] = append(g.Edges[node.ID], connectedNode.ID)
		} else {
			fmt.Println("      [Edge Found],")
		}
	}
}

func checkSources(g *Graph, appGraph *Graph, node *Node) {
	for srcNodeName, destinations := range appGraph.Edges {
		for _, destNodeName := range destinations {
			if destNodeName == node.Name {
				fmt.Println(" [Found] an existing edge")
				for _, srcNode := range g.Nodes {
					if srcNode.Name == srcNodeName {
						g.Edges[srcNode.ID] = append(g.Edges[srcNode.ID], node.ID)
					}
				}
			}
		}
	}
}

// containsNode checks if a node with the given name exists in the node slice
func containsNode(nodes []*Node, name string) []int {
	var idxs []int
	for idx, node := range nodes {
		if node.Name == name {
			idxs = append(idxs, idx)
		}
	}
	return idxs
}

func containsEdge(edges map[string][]string, u, v string) bool {
	if connections, ok := edges[u]; ok {
		for _, connectedNode := range connections {
			if connectedNode == v {
				return true
			}
		}
	}
	return false
}

// printGraph prints the nodes and edges of the graph
func (g *Graph) PrintGraph() {
	uuidTruncateLimit := 15
	fmt.Println("---- Nodes:")
	for _, node := range g.Nodes {
		fmt.Println(node.ID[:uuidTruncateLimit])
	}
	fmt.Println("---- Edges:")
	for u, v := range g.Edges {
		for _, v_ := range v {
			fmt.Println(u[:uuidTruncateLimit] + "-" + v_[:uuidTruncateLimit])
		}
	}
}

func getLocationLable(nodeID string) string {
	// Split the nodeID and get Lx
	if locLabel := strings.Split(nodeID, "_"); len(locLabel) > 2 {
		return locLabel[2]
	} else {
		return ""
	}
}

func generateUniqueInt(input string) string {
	// Create a new hash object
	hasher := fnv.New32a()

	// Write the input string to the hasher
	hasher.Write([]byte(input))

	// Return the hash value mapped to the range 1-12
	return strconv.Itoa(int((hasher.Sum32() % 12) + 1))
}

// DrawGraph visualizes the graph using the dot command
func (g *Graph) DrawGraph() {

	gv := customgraph.New(customgraph.StringHash, customgraph.Directed(), customgraph.Acyclic())
	for _, node := range g.Nodes {
		uuidTruncateLimit := 15
		if len(node.ID) < uuidTruncateLimit {
			uuidTruncateLimit = len(node.ID)
		}
		gv.AddVertex(
			node.ID[:uuidTruncateLimit],
			graph.VertexAttribute("colorscheme", "paired12"),
			graph.VertexAttribute("style", "filled"),
			graph.VertexAttribute("color", "2"),
			graph.VertexAttribute("fillcolor", generateUniqueInt(getLocationLable(node.ID))))
	}

	for u, v := range g.Edges {
		for _, v_ := range v {
			uuidTruncateLimit := 15
			if len(u) < uuidTruncateLimit {
				uuidTruncateLimit = len(u)
			}
			if len(v_) < uuidTruncateLimit {
				uuidTruncateLimit = len(v_)
			}
			gv.AddEdge(u[:uuidTruncateLimit], v_[:uuidTruncateLimit])
		}
	}

	// Write gv into a file
	file, _ := os.Create("graph-visualized.gv")
	_ = draw.DOT(gv, file)

	cmd := exec.Command("dot", "-Tjpg", "-O", "graph-visualized.gv")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Errorf("Error: %v", err)
	}
}
