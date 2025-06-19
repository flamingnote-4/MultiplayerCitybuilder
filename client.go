package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/google/uuid"
)

type CityLine struct {
	Start    rl.Vector2
	End      rl.Vector2
	Type     InfrastructureType
	PlayerID string
}

type PlayerCursor struct {
	Position rl.Vector2
	Name     string
}

type LobbyClient struct {
	conn         net.Conn
	mutex        sync.Mutex
	Connected    bool
	clientID     string
	playerName   string
	OtherCursors map[string]PlayerCursor
	CityLines    []CityLine
	Buildings    []Building
	BusRoutes    []BusRoute
	Buses        []Bus
	Money        float32
}

func (c *LobbyClient) Connect(ip string, port int, playerName string) {
	c.clientID = uuid.NewString()
	c.playerName = playerName
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return
	}
	c.conn = conn
	c.Connected = true

	c.OtherCursors = make(map[string]PlayerCursor)
	c.CityLines = make([]CityLine, 0)
	c.Buildings = make([]Building, 0)
	c.BusRoutes = make([]BusRoute, 0)
	c.Buses = make([]Bus, 0)
	c.Money = 0.0

	joinMsg := fmt.Sprintf("JOIN:%s:%s\n", c.clientID, playerName)
	_, err = c.conn.Write([]byte(joinMsg))
	if err != nil {
		c.Disconnect()
		return
	}

	go c.listen()
	go func() {
		for c.Connected {
			_, err := c.conn.Write([]byte("PING\n"))
			if err != nil {
				c.Disconnect()
				return
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

func (c *LobbyClient) Disconnect() {
	if c.conn != nil {
		c.conn.Close()
	}
	c.Connected = false
}

func (c *LobbyClient) SendCursor(x, y float32) {
	if !c.Connected {
		return
	}
	msg := fmt.Sprintf("C:%s:%.0f:%.0f\n", c.clientID, x, y)
	_, err := c.conn.Write([]byte(msg))
	if err != nil {
		c.Disconnect()
	}
}

func (c *LobbyClient) SendInfrastructure(startX, startY, endX, endY float32, infraType InfrastructureType) {
	if !c.Connected {
		return
	}
	msg := fmt.Sprintf("I:%s:%.0f:%.0f:%.0f:%.0f:%d\n",
		c.clientID, startX, startY, endX, endY, int(infraType))
	_, err := c.conn.Write([]byte(msg))
	if err != nil {
		c.Disconnect()
	}
}

func (c *LobbyClient) SendBuilding(x, y float32, buildingType BuildingType) {
	if !c.Connected {
		return
	}
	msg := fmt.Sprintf("B:%s:%.0f:%.0f:%d\n",
		c.clientID, x, y, int(buildingType))
	_, err := c.conn.Write([]byte(msg))
	if err != nil {
		c.Disconnect()
	}
}

func (c *LobbyClient) SendBusRoute(nodes []rl.Vector2) {
	if !c.Connected || len(nodes) < 2 {
		return
	}

	var sb strings.Builder
	sb.WriteString("R:")
	sb.WriteString(c.clientID)
	for _, node := range nodes {
		sb.WriteString(fmt.Sprintf(":%.0f:%.0f", node.X, node.Y))
	}
	sb.WriteString("\n")
	msg := sb.String()

	_, err := c.conn.Write([]byte(msg))
	if err != nil {
		c.Disconnect()
	}
}

func (c *LobbyClient) SendDelete(x, y float32) {
	if !c.Connected {
		return
	}
	msg := fmt.Sprintf("D:%s:%.0f:%.0f\n", c.clientID, x, y)
	_, err := c.conn.Write([]byte(msg))
	if err != nil {
		c.Disconnect()
	}
}

func (c *LobbyClient) listen() {
	defer func() {
		c.Connected = false
		if r := recover(); r != nil {
		}
	}()

	reader := bufio.NewReader(c.conn)

	for c.Connected {

		c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		lineBytes, err := reader.ReadBytes('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {

				continue
			}

			if c.Connected {
			}
			break
		}

		fullMsg := strings.TrimSpace(string(lineBytes))

		if fullMsg == "" {
			continue
		}

		parts := strings.Split(fullMsg, ":")
		cmd := parts[0]

		c.mutex.Lock()
		switch cmd {
		case "C":
			if len(parts) == 5 {
				playerID, playerName := parts[1], parts[2]
				x, errX := strconv.ParseFloat(parts[3], 32)
				y, errY := strconv.ParseFloat(parts[4], 32)
				if errX == nil && errY == nil {
					c.OtherCursors[playerID] = PlayerCursor{
						Position: rl.NewVector2(float32(x), float32(y)),
						Name:     playerName,
					}
				} else {
				}
			} else {
			}
		case "STATE_RESET":
			c.CityLines = make([]CityLine, 0)
			c.Buildings = make([]Building, 0)
			c.BusRoutes = make([]BusRoute, 0)
			c.Buses = make([]Bus, 0)
		case "I":
			if len(parts) == 7 {
				playerID := parts[1]
				startX, errSX := strconv.ParseFloat(parts[2], 32)
				startY, errSY := strconv.ParseFloat(parts[3], 32)
				endX, errEX := strconv.ParseFloat(parts[4], 32)
				endY, errEY := strconv.ParseFloat(parts[5], 32)
				infraType, errIT := strconv.Atoi(parts[6])

				if !(errSX != nil || errSY != nil || errEX != nil || errEY != nil || errIT != nil) {
					newLine := CityLine{
						Start:    rl.NewVector2(float32(startX), float32(startY)),
						End:      rl.NewVector2(float32(endX), float32(endY)),
						Type:     InfrastructureType(infraType),
						PlayerID: playerID,
					}
					c.CityLines = append(c.CityLines, newLine)
				}
			} else {
			}
		case "B":
			if len(parts) == 5 {
				playerID := parts[1]
				x, errX := strconv.ParseFloat(parts[2], 32)
				y, errY := strconv.ParseFloat(parts[3], 32)
				buildingType, errBT := strconv.Atoi(parts[4])
				if errX == nil && errY == nil && errBT == nil {
					newBuilding := Building{
						Position: rl.NewVector2(float32(x), float32(y)),
						Type:     BuildingType(buildingType),
						PlayerID: playerID,
					}
					c.Buildings = append(c.Buildings, newBuilding)
				} else {
				}
			} else {
			}
		case "R":
			if len(parts) >= 6 && (len(parts)-2)%2 == 0 {
				playerID := parts[1]
				nodes := make([]rl.Vector2, 0, (len(parts)-2)/2)
				allParsed := true
				for i := 2; i < len(parts); i += 2 {
					x, errX := strconv.ParseFloat(parts[i], 32)
					y, errY := strconv.ParseFloat(parts[i+1], 32)
					if errX == nil && errY == nil {
						nodes = append(nodes, rl.NewVector2(float32(x), float32(y)))
					} else {
						allParsed = false
						break
					}
				}
				if allParsed && len(nodes) >= 2 {
					newRoute := BusRoute{Nodes: nodes, PlayerID: playerID, Length: 0}
					c.BusRoutes = append(c.BusRoutes, newRoute)
				} else if !allParsed {
				}
			} else {
			}
		case "BUS":
			if len(parts) == 4 {
				busID, errID := strconv.Atoi(parts[1])
				x, errX := strconv.ParseFloat(parts[2], 32)
				y, errY := strconv.ParseFloat(parts[3], 32)

				if errID == nil && errX == nil && errY == nil {

					for len(c.Buses) <= busID {
						c.Buses = append(c.Buses, Bus{})
					}
					c.Buses[busID].Position = rl.NewVector2(float32(x), float32(y))
					c.Buses[busID].RouteID = busID
				} else {
				}
			} else {
			}
		case "MONEY":
			if len(parts) == 2 {
				moneyVal, err := strconv.ParseFloat(parts[1], 32)
				if err == nil {
					c.Money = float32(moneyVal)
				} else {
				}
			} else {
			}
		case "STATUS":
		case "DISCONNECT":
			if len(parts) == 2 {
				playerID := parts[1]
				delete(c.OtherCursors, playerID)
			} else {
			}
		case "PING":

		default:
		}
		c.mutex.Unlock()
	}
}
