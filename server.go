package main

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type StoredBuilding struct {
	X, Y     float32
	Type     BuildingType
	PlayerID string
}

type StoredBusRoute struct {
	Points   []float32
	PlayerID string
	Length   float32
}

type StoredLine struct {
	StartX, StartY, EndX, EndY float32
	Type                       InfrastructureType
	PlayerID                   string
}

type GameState struct {
	Lines     []StoredLine
	Buildings []StoredBuilding
	BusRoutes []StoredBusRoute
	Money     float32
}

type Player struct {
	Conn     net.Conn
	ID       string
	Name     string
	LastSeen time.Time
}

type LobbyServer struct {
	listener    net.Listener
	players     map[string]*Player
	playerConns map[net.Conn]string
	lines       []StoredLine
	buildings   []StoredBuilding
	busRoutes   []StoredBusRoute
	buses       []Bus
	money       float32
	incomeRate  float32
	mutex       sync.Mutex
	running     bool
}

func (s *LobbyServer) Start(port int) error {
	s.players = make(map[string]*Player)
	s.playerConns = make(map[net.Conn]string)
	s.lines = make([]StoredLine, 0)
	s.buildings = make([]StoredBuilding, 0)
	s.busRoutes = make([]StoredBusRoute, 0)
	s.buses = make([]Bus, 0)
	s.money = 1000.0
	s.incomeRate = 0.0

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}
	s.listener = ln
	s.running = true

	go s.cleanupRoutine()
	go s.updateBusesRoutine()
	go s.incomeRoutine()
	go func() {
		for s.running {
			conn, err := ln.Accept()
			if err != nil {
				if !s.running {
					break
				}
				continue
			}
			go s.handleClient(conn)
		}
	}()
	return nil
}

func (s *LobbyServer) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return
	}
	s.running = false
	if s.listener != nil {
		s.listener.Close()
	}

	for conn := range s.playerConns {
		conn.Close()
	}
	s.players = make(map[string]*Player)
	s.playerConns = make(map[net.Conn]string)
}

func (s *LobbyServer) cleanupRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !s.running {
			break
		}
		s.mutex.Lock()
		now := time.Now()
		for _, player := range s.players {
			if now.Sub(player.LastSeen) > 15*time.Second {
				player.Conn.Close()
			}
		}
		s.mutex.Unlock()
	}
}

// New: incomeRoutine periodically adds money based on incomeRate
func (s *LobbyServer) incomeRoutine() {
	ticker := time.NewTicker(10 * time.Second) // Income every 10 seconds
	defer ticker.Stop()

	for range ticker.C {
		if !s.running {
			break
		}
		s.mutex.Lock()
		if s.incomeRate > 0 { // Only add if there's positive income
			s.money += s.incomeRate
			s.broadcastMoney()
		}
		s.mutex.Unlock()
	}
}

func (s *LobbyServer) updateBusesRoutine() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	const frameTime = 50.0 / 1000.0

	for range ticker.C {
		if !s.running {
			break
		}

		s.mutex.Lock()
		for i := range s.buses {
			bus := &s.buses[i]

			if bus.RouteID >= len(s.busRoutes) {
				continue
			}

			storedRoute := s.busRoutes[bus.RouteID]
			routeNodes := make([]rl.Vector2, len(storedRoute.Points)/2)
			for j := 0; j < len(storedRoute.Points); j += 2 {
				routeNodes[j/2] = rl.NewVector2(storedRoute.Points[j], storedRoute.Points[j+1])
			}
			currentRoute := BusRoute{Nodes: routeNodes, Length: storedRoute.Length}

			if len(currentRoute.Nodes) < 2 {
				continue
			}

			var startNode, endNode rl.Vector2

			if bus.Direction == 1 {
				startNode = currentRoute.Nodes[bus.CurrentSegment]
				endNode = currentRoute.Nodes[bus.CurrentSegment+1]
			} else {
				startNode = currentRoute.Nodes[bus.CurrentSegment+1]
				endNode = currentRoute.Nodes[bus.CurrentSegment]
			}

			distance := rl.Vector2Distance(startNode, endNode)
			if distance > 0 {
				bus.Progress += (BUS_SPEED / distance) * frameTime
			} else {
				bus.Progress = 1.0
			}

			if bus.Progress >= 1.0 {
				bus.Progress = 0.0
				if bus.Direction == 1 {
					bus.CurrentSegment++
					if bus.CurrentSegment >= len(currentRoute.Nodes)-1 {
						bus.Direction = -1
						bus.CurrentSegment = len(currentRoute.Nodes) - 2

						if currentRoute.Length > 0 {
							reward := currentRoute.Length / 16
							s.money += reward
						}
						s.broadcastMoney()
					}
				} else {
					bus.CurrentSegment--
					if bus.CurrentSegment < 0 {
						bus.Direction = 1
						bus.CurrentSegment = 0

						if currentRoute.Length > 0 {
							reward := currentRoute.Length / 16
							s.money += reward
						}
						s.broadcastMoney()
					}
				}
			}

			if bus.Direction == 1 {
				startNode = currentRoute.Nodes[bus.CurrentSegment]
				endNode = currentRoute.Nodes[bus.CurrentSegment+1]
			} else {
				startNode = currentRoute.Nodes[bus.CurrentSegment+1]
				endNode = currentRoute.Nodes[bus.CurrentSegment]
			}
			bus.Position = rl.Vector2Lerp(startNode, endNode, bus.Progress)

			s.broadcastToAll(fmt.Sprintf("BUS:%d:%.0f:%.0f", i, bus.Position.X, bus.Position.Y))
		}
		s.mutex.Unlock()
	}
}

func (s *LobbyServer) handleClient(conn net.Conn) {
	var playerID string
	defer func() {
		s.mutex.Lock()
		if pID, exists := s.playerConns[conn]; exists {
			if _, pExists := s.players[pID]; pExists {
				s.broadcastToOthers(fmt.Sprintf("DISCONNECT:%s", pID), conn)
			}
			delete(s.players, pID)
			delete(s.playerConns, conn)
		}
		s.mutex.Unlock()
		conn.Close()
	}()

	buf := make([]byte, 1024)
	leftover := ""

	for s.running {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if !s.running {
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
		leftover += string(buf[:n])

		for {
			idx := strings.Index(leftover, "\n")
			if idx == -1 {
				break
			}
			fullMsg := strings.TrimSpace(leftover[:idx])
			leftover = leftover[idx+1:]

			if playerID == "" {
				parts := strings.Split(fullMsg, ":")
				if len(parts) > 1 && parts[0] == "JOIN" {
					playerID = parts[1]
				}
			}
			s.handleMessage(fullMsg, conn)
		}
	}
}

func (s *LobbyServer) handleMessage(msg string, conn net.Conn) {
	parts := strings.Split(msg, ":")
	if len(parts) == 0 {
		return
	}
	msgType := parts[0]

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if pID, exists := s.playerConns[conn]; exists {
		if player, pExists := s.players[pID]; pExists {
			player.LastSeen = time.Now()
		}
	}

	switch msgType {
	case "PING":
		return
	case "JOIN":
		if len(parts) == 3 {
			playerID, playerName := parts[1], parts[2]
			player := &Player{Conn: conn, ID: playerID, Name: playerName, LastSeen: time.Now()}
			s.players[playerID] = player
			s.playerConns[conn] = playerID
			s.sendFullState(conn)
			s.broadcastMoney()
		} else {
		}
	case "C":
		if len(parts) == 4 {
			playerID := parts[1]
			if player, exists := s.players[playerID]; exists {
				cursorMsg := fmt.Sprintf("C:%s:%s:%s:%s", playerID, player.Name, parts[2], parts[3])
				s.broadcastToOthers(cursorMsg, conn)
			}
		} else {
		}
	case "I":
		if len(parts) == 7 {
			s.addInfrastructure(msg, parts)
		} else {
		}
	case "B":
		if len(parts) == 5 {
			s.addBuilding(msg, parts)
		} else {
		}
	case "R":
		if len(parts) >= 6 && (len(parts)-2)%2 == 0 {
			s.addBusRoute(msg, parts)
		} else {
		}
	case "D":
		if len(parts) == 4 {
			s.deleteObject(parts)
		} else {
		}
	default:
	}
}

func (s *LobbyServer) isPointOnRoad(px, py float32) bool {
	p := rl.NewVector2(px, py)

	for _, line := range s.lines {
		if line.Type != Road {
			continue
		}

		roadStart := rl.NewVector2(line.StartX, line.StartY)
		roadEnd := rl.NewVector2(line.EndX, line.EndY)

		dist := pointSegmentDistance(p, roadStart, roadEnd)

		if dist <= ROAD_SNAP_DISTANCE {
			return true
		}
	}
	return false
}

func (s *LobbyServer) sendFullState(conn net.Conn) {
	for _, line := range s.lines {
		lineMsg := fmt.Sprintf("I:%s:%.0f:%.0f:%.0f:%.0f:%d\n",
			line.PlayerID, line.StartX, line.StartY, line.EndX, line.EndY, int(line.Type))
		conn.Write([]byte(lineMsg))
	}
	for _, b := range s.buildings {
		bMsg := fmt.Sprintf("B:%s:%.0f:%.0f:%d\n", b.PlayerID, b.X, b.Y, int(b.Type))
		conn.Write([]byte(bMsg))
	}
	for _, r := range s.busRoutes {
		routeMsg := fmt.Sprintf("R:%s", r.PlayerID)
		for _, p := range r.Points {
			routeMsg += fmt.Sprintf(":%.0f", p)
		}
		routeMsg += "\n"
		conn.Write([]byte(routeMsg))
	}
	for i, bus := range s.buses {
		busPosMsg := fmt.Sprintf("BUS:%d:%.0f:%.0f\n", i, bus.Position.X, bus.Position.Y)
		conn.Write([]byte(busPosMsg))
	}

	conn.Write([]byte("STATE_SYNCED\n"))
}

func (s *LobbyServer) addInfrastructure(msg string, parts []string) {
	playerID := parts[1]
	startX, _ := strconv.ParseFloat(parts[2], 32)
	startY, _ := strconv.ParseFloat(parts[3], 32)
	endX, _ := strconv.ParseFloat(parts[4], 32)
	endY, _ := strconv.ParseFloat(parts[5], 32)
	infraType, _ := strconv.Atoi(parts[6])

	if InfrastructureType(infraType) == Road {
		roadStart := rl.NewVector2(float32(startX), float32(startY))
		roadEnd := rl.NewVector2(float32(endX), float32(endY))
		roadLength := rl.Vector2Distance(roadStart, roadEnd)
		cost := roadLength * ROAD_COST_PER_UNIT

		if s.money < cost {
			s.broadcastToPlayer(playerID, "STATUS:Not enough money to build road!")
			return
		}
		s.money -= cost
		s.broadcastMoney()
	}

	newLine := StoredLine{
		StartX: float32(startX), StartY: float32(startY),
		EndX: float32(endX), EndY: float32(endY),
		Type: InfrastructureType(infraType), PlayerID: playerID,
	}
	s.lines = append(s.lines, newLine)
	s.broadcastToAll(msg)
}

func (s *LobbyServer) addBuilding(msg string, parts []string) {
	playerID := parts[1]
	x, _ := strconv.ParseFloat(parts[2], 32)
	y, _ := strconv.ParseFloat(parts[3], 32)
	buildingTypeInt, _ := strconv.Atoi(parts[4])
	buildingType := BuildingType(buildingTypeInt)

	var cost float32
	var incomeIncrease float32

	switch buildingType {
	case Residential:
		cost = RESIDENTIAL_BUILDING_COST
	case Commercial:
		cost = COMMERCIAL_BUILDING_COST
		incomeIncrease = COMMERCIAL_INCOME_INCREASE
	case Industrial:
		cost = INDUSTRIAL_BUILDING_COST
		incomeIncrease = INDUSTRIAL_INCOME_INCREASE
	default:
		s.broadcastToPlayer(playerID, "STATUS:Unknown building type!")
		return
	}

	if s.money < cost {
		s.broadcastToPlayer(playerID, fmt.Sprintf("STATUS:Not enough money to build %s! Cost: %.2f", getBuildingName(buildingType), cost))
		return
	}

	s.money -= cost
	s.incomeRate += incomeIncrease // Add income for Commercial/Industrial
	s.broadcastMoney()

	newBuilding := StoredBuilding{
		X: float32(x), Y: float32(y),
		Type: buildingType, PlayerID: playerID,
	}
	s.buildings = append(s.buildings, newBuilding)
	s.broadcastToAll(msg)
}

func (s *LobbyServer) addBusRoute(msg string, parts []string) {
	playerID := parts[1]
	points := make([]float32, 0, len(parts)-2)
	nodes := make([]rl.Vector2, 0, (len(parts)-2)/2)
	for i := 2; i < len(parts); i += 2 {
		x, errX := strconv.ParseFloat(parts[i], 32)
		y, errY := strconv.ParseFloat(parts[i+1], 32)
		if errX == nil && errY == nil {
			points = append(points, float32(x), float32(y))
			nodes = append(nodes, rl.NewVector2(float32(x), float32(y)))
		} else {
			s.broadcastToPlayer(playerID, "STATUS:Invalid coordinates for bus route node.")
			return
		}
	}

	if len(nodes) < 2 {
		s.broadcastToPlayer(playerID, "STATUS:Bus route needs at least 2 nodes!")
		return
	}

	var totalLength float32
	allSegmentsOnRoad := true
	const intermediatePointsPerSegment = 4
	for i := 0; i < len(nodes)-1; i++ {
		segmentStart := nodes[i]
		segmentEnd := nodes[i+1]
		segmentLength := rl.Vector2Distance(segmentStart, segmentEnd)
		totalLength += segmentLength

		for p := 0; p <= intermediatePointsPerSegment; p++ {
			t := float32(p) / float32(intermediatePointsPerSegment)
			intermediatePoint := rl.Vector2Lerp(segmentStart, segmentEnd, t)
			if !s.isPointOnRoad(intermediatePoint.X, intermediatePoint.Y) {
				allSegmentsOnRoad = false
				break
			}
		}
		if !allSegmentsOnRoad {
			break
		}
	}

	if !allSegmentsOnRoad {
		s.broadcastToPlayer(playerID, "STATUS:Bus route must be fully on roads!")
		return
	}

	newRoute := StoredBusRoute{
		Points:   points,
		PlayerID: playerID,
		Length:   totalLength,
	}
	s.busRoutes = append(s.busRoutes, newRoute)

	newBus := Bus{
		RouteID:        len(s.busRoutes) - 1,
		Position:       nodes[0],
		CurrentSegment: 0,
		Progress:       0.0,
		Direction:      1,
	}
	s.buses = append(s.buses, newBus)

	s.broadcastToAll(msg)
	s.broadcastToAll(fmt.Sprintf("BUS:%d:%.0f:%.0f", len(s.buses)-1, newBus.Position.X, newBus.Position.Y))
}

func (s *LobbyServer) deleteObject(parts []string) {
	playerID := parts[1]
	x, _ := strconv.ParseFloat(parts[2], 32)
	y, _ := strconv.ParseFloat(parts[3], 32)
	deleteRadius := float64(GRID_SIZE * 0.75)
	deletedSomething := false

	deletedRoadIndices := make(map[int]bool)
	var deletedBuildingType BuildingType = -1 // Store type of deleted building if any

	for i := len(s.buildings) - 1; i >= 0; i-- {
		b := s.buildings[i]
		dist := math.Sqrt(math.Pow(float64(b.X)-x, 2) + math.Pow(float64(b.Y)-y, 2))
		if dist <= deleteRadius {
			deletedBuildingType = b.Type // Store type before deletion
			s.buildings = append(s.buildings[:i], s.buildings[i+1:]...)
			deletedSomething = true
			break
		}
	}

	// Adjust income if a building was deleted
	if deletedSomething && deletedBuildingType != -1 {
		switch deletedBuildingType {
		case Commercial:
			s.incomeRate -= COMMERCIAL_INCOME_INCREASE
		case Industrial:
			s.incomeRate -= INDUSTRIAL_INCOME_INCREASE
		}
		if s.incomeRate < 0 { // Ensure income rate doesn't go below zero
			s.incomeRate = 0
		}
		s.broadcastMoney()
	}

	if !deletedSomething {
		for i := len(s.lines) - 1; i >= 0; i-- {
			l := s.lines[i]
			distStart := math.Sqrt(math.Pow(float64(l.StartX)-x, 2) + math.Pow(float64(l.StartY)-y, 2))
			distEnd := math.Sqrt(math.Pow(float64(l.EndX)-x, 2) + math.Pow(float64(l.EndY)-y, 2))
			lineSegmentDist := pointSegmentDistance(rl.NewVector2(float32(x), float32(y)), rl.NewVector2(l.StartX, l.StartY), rl.NewVector2(l.EndX, l.EndY))

			if distStart <= deleteRadius || distEnd <= deleteRadius || lineSegmentDist <= float32(deleteRadius) {
				if l.Type == Road {
					roadStart := rl.NewVector2(l.StartX, l.StartY)
					roadEnd := rl.NewVector2(l.EndX, l.EndY)
					roadLength := rl.Vector2Distance(roadStart, roadEnd)
					refund := roadLength * ROAD_COST_PER_UNIT
					s.money += refund
					s.broadcastMoney()
					deletedRoadIndices[i] = true
				}
				s.lines = append(s.lines[:i], s.lines[i+1:]...)
				deletedSomething = true
				break
			}
		}
	}

	if !deletedSomething {
		for i := len(s.busRoutes) - 1; i >= 0; i-- {
			r := s.busRoutes[i]
			routeDeleted := false
			for j := 0; j < len(r.Points); j += 2 {
				nodeX, nodeY := r.Points[j], r.Points[j+1]
				dist := math.Sqrt(math.Pow(float64(nodeX)-x, 2) + math.Pow(float64(nodeY)-y, 2))
				if dist <= deleteRadius {
					s.busRoutes = append(s.busRoutes[:i], s.busRoutes[i+1:]...)
					s.removeBusesForRoute(i)
					deletedSomething = true
					routeDeleted = true
					break
				}
			}
			if routeDeleted {
				break
			}
		}
	}

	if len(deletedRoadIndices) > 0 {
		routesToPrune := []int{}
		for i := 0; i < len(s.busRoutes); i++ {
			route := s.busRoutes[i]
			routeIsValid := true
			nodes := make([]rl.Vector2, len(route.Points)/2)
			for j := 0; j < len(route.Points); j += 2 {
				nodes[j/2] = rl.NewVector2(route.Points[j], route.Points[j+1])
			}

			const intermediatePointsPerSegment = 4
			for j := 0; j < len(nodes)-1; j++ {
				segmentStart := nodes[j]
				segmentEnd := nodes[j+1]

				for p := 0; p <= intermediatePointsPerSegment; p++ {
					t := float32(p) / float32(intermediatePointsPerSegment)
					intermediatePoint := rl.Vector2Lerp(segmentStart, segmentEnd, t)
					if !s.isPointOnRoad(intermediatePoint.X, intermediatePoint.Y) {
						routeIsValid = false
						break
					}
				}
				if !routeIsValid {
					break
				}
			}

			if !routeIsValid {
				routesToPrune = append(routesToPrune, i)
			}
		}

		for k := len(routesToPrune) - 1; k >= 0; k-- {
			idxToRemove := routesToPrune[k]
			s.busRoutes = append(s.busRoutes[:idxToRemove], s.busRoutes[idxToRemove+1:]...)
			s.removeBusesForRoute(idxToRemove)
			deletedSomething = true
		}
	}

	if deletedSomething {
		s.broadcastToAll("STATE_RESET")
		for clientConn := range s.playerConns {
			s.sendFullState(clientConn)
		}
	} else {
		s.broadcastToPlayer(playerID, "STATUS:No deletable object found here.")
	}
}

func pointSegmentDistance(p, a, b rl.Vector2) float32 {
	l2 := rl.Vector2DistanceSqr(a, b)
	if l2 == 0.0 {
		return rl.Vector2Distance(p, a)
	}
	t := rl.Vector2DotProduct(rl.Vector2Subtract(p, a), rl.Vector2Subtract(b, a)) / l2
	t = float32(math.Max(0, math.Min(1, float64(t))))
	projection := rl.Vector2Add(a, rl.Vector2Scale(rl.Vector2Subtract(b, a), t))
	return rl.Vector2Distance(p, projection)
}

func (s *LobbyServer) removeBusesForRoute(routeID int) {
	newBuses := make([]Bus, 0)
	for _, bus := range s.buses {
		if bus.RouteID != routeID {
			if bus.RouteID > routeID {
				bus.RouteID--
			}
			newBuses = append(newBuses, bus)
		}
	}
	s.buses = newBuses
}

func (s *LobbyServer) broadcastMoney() {
	s.broadcastToAll(fmt.Sprintf("MONEY:%.2f", s.money))
}

func (s *LobbyServer) broadcastToAll(msg string) {
	for conn := range s.playerConns {
		conn.Write([]byte(msg + "\n"))
	}
}

func (s *LobbyServer) broadcastToOthers(msg string, exclude net.Conn) {
	for conn := range s.playerConns {
		if conn != exclude {
			conn.Write([]byte(msg + "\n"))
		}
	}
}

func (s *LobbyServer) broadcastToPlayer(playerID string, msg string) {
	if player, exists := s.players[playerID]; exists {
		player.Conn.Write([]byte(msg + "\n"))
	}
}
