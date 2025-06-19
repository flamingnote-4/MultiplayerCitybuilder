package main

import (
	"fmt"
	"strconv"

	gui "github.com/gen2brain/raylib-go/raygui"
	rl "github.com/gen2brain/raylib-go/raylib"
)

type GameScreen int

const (
	MainMenu GameScreen = iota
	ServerChooser
	InGame
)

type BuildMode int

const (
	InfrastructureMode BuildMode = iota
	BuildingMode
	BusRouteMode
	DeleteMode
)

var (
	currentScreen = MainMenu
	status        = "Not connected yet!"
	hosting       = false
	sendTimer     float32
	ipInput       = "127.0.0.1"
	portInput     = "7777"
	playerName    = "Player"

	localServer LobbyServer
	client      LobbyClient
	titleFont   rl.Font
	textFont    rl.Font

	isBuilding          bool
	buildStart          rl.Vector2
	currentInfraType    InfrastructureType
	currentBuildingType BuildingType
	currentBuildMode    BuildMode = InfrastructureMode
	showGrid            bool      = true
	cameraOffset        rl.Vector2
	zoom                float32 = 1.0

	isCreatingBusRoute bool
	currentRouteNodes  []rl.Vector2
)

var ipBox = CustomTextBox{
	Rect:      rl.NewRectangle(200, 200, 250, 30),
	Text:      ipInput,
	MaxLength: 20,
}
var portBox = CustomTextBox{
	Rect:      rl.NewRectangle(200, 240, 250, 30),
	Text:      portInput,
	MaxLength: 6,
}
var nameBox = CustomTextBox{
	Rect:      rl.NewRectangle(200, 160, 250, 30),
	Text:      playerName,
	MaxLength: 15,
}

func main() {
	rl.InitWindow(1024, 768, "Multiplayer Citybuilder")
	rl.SetTargetFPS(60)
	rl.SetExitKey(0)

	titleFont = rl.LoadFontEx("fonts/Unageo-Medium.ttf", 72, nil)
	textFont = rl.LoadFontEx("fonts/Unageo-Medium.ttf", 24, nil)

	// fmt.Println("[Main] Application starting.")

	defer func() {
		rl.CloseWindow()
		// fmt.Println("[Main] Disconnecting client.")
		client.Disconnect()
		// fmt.Println("[Main] Stopping local server.")
		localServer.Stop()
		rl.UnloadFont(titleFont)
		rl.UnloadFont(textFont)
	}()

	for !rl.WindowShouldClose() {
		delta := rl.GetFrameTime()
		update(delta)
		draw()
	}
}

func snapToGrid(pos rl.Vector2) rl.Vector2 {
	return rl.NewVector2(
		float32(int(pos.X/GRID_SIZE))*GRID_SIZE+GRID_SIZE/2,
		float32(int(pos.Y/GRID_SIZE))*GRID_SIZE+GRID_SIZE/2,
	)
}

func screenToWorld(screenPos rl.Vector2) rl.Vector2 {
	return rl.NewVector2(
		(screenPos.X/zoom - cameraOffset.X),
		(screenPos.Y/zoom - cameraOffset.Y),
	)
}

func worldToScreen(worldPos rl.Vector2) rl.Vector2 {
	return rl.NewVector2(
		(worldPos.X+cameraOffset.X)*zoom,
		(worldPos.Y+cameraOffset.Y)*zoom,
	)
}

func update(delta float32) {
	switch currentScreen {
	case MainMenu:
		if gui.Button(rl.NewRectangle(400, 300, 200, 40), "Multiplayer") {
			currentScreen = ServerChooser
		}
		if gui.Button(rl.NewRectangle(400, 360, 200, 40), "Exit") {
			rl.CloseWindow()
		}
	case ServerChooser:
		if rl.IsKeyPressed(rl.KeyEscape) {
			currentScreen = MainMenu
		}

		if gui.Button(rl.NewRectangle(200, 80, 200, 30), "Host Game") {
			// fmt.Println("[Main] Hosting game...")
			err := localServer.Start(7777)
			if err != nil {
				status = "Failed to host server."
				// fmt.Printf("[Main] Host failed: %v\n", err)
				return
			}
			playerName = nameBox.Text
			if playerName == "" {
				playerName = "Host"
			}
			client.Connect("127.0.0.1", 7777, playerName)
			hosting = true
			status = "Hosting game..."
			currentScreen = InGame
		}

		if gui.Button(rl.NewRectangle(200, 120, 200, 30), "Join Game") {
			// fmt.Println("[Main] Joining game...")
			ipInput = ipBox.Text
			portInput = portBox.Text
			playerName = nameBox.Text
			if playerName == "" {
				playerName = "Player"
			}
			port, err := strconv.Atoi(portInput)
			if err == nil {
				client.Connect(ipInput, port, playerName)
				status = "Joining " + ipInput
				currentScreen = InGame
			} else {
				// fmt.Printf("[Main] Invalid port number: %v\n", err)
				status = "Invalid port!"
			}
		}

		nameBox.Update()
		ipBox.Update()
		portBox.Update()
	case InGame:
		mousePos := rl.GetMousePosition()

		if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
			cameraOffset.X += 1000 * delta
		}
		if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
			cameraOffset.X -= 1000 * delta
		}
		if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
			cameraOffset.Y += 1000 * delta
		}
		if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
			cameraOffset.Y -= 1000 * delta
		}

		wheel := rl.GetMouseWheelMove()
		if wheel != 0 {
			zoom += wheel * 0.1
			if zoom < 0.5 {
				zoom = 0.5
			}
			if zoom > 3.0 {
				zoom = 3.0
			}
		}

		if rl.IsKeyPressed(rl.KeyG) {
			showGrid = !showGrid
		}

		if mousePos.Y > UI_HEIGHT {
			worldPos := screenToWorld(mousePos)
			snappedPos := snapToGrid(worldPos)

			if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
				switch currentBuildMode {
				case InfrastructureMode:
					isBuilding = true
					buildStart = snappedPos
				case BuildingMode:
					if client.Connected {
						client.SendBuilding(snappedPos.X, snappedPos.Y, currentBuildingType)
					}
				case BusRouteMode:
					currentRouteNodes = append(currentRouteNodes, snappedPos)
					isCreatingBusRoute = true
				case DeleteMode:
					if client.Connected {
						client.SendDelete(snappedPos.X, snappedPos.Y)
					}
				}
			}

			if rl.IsMouseButtonReleased(rl.MouseLeftButton) && isBuilding && currentBuildMode == InfrastructureMode {
				isBuilding = false
				buildEnd := snappedPos

				if buildStart.X != buildEnd.X || buildStart.Y != buildEnd.Y {
					if client.Connected {
						client.SendInfrastructure(buildStart.X, buildStart.Y, buildEnd.X, buildEnd.Y, currentInfraType)
					}
				}
			}
		}

		sendTimer += delta
		if sendTimer >= 0.04 {
			sendTimer = 0
			worldPos := screenToWorld(mousePos)
			client.SendCursor(worldPos.X, worldPos.Y)
		}

		if !client.Connected {
			status = "Disconnected from server"
			currentScreen = MainMenu
		}

		if rl.IsKeyPressed(rl.KeyEscape) {
			client.Disconnect()
			if hosting {
				localServer.Stop()
			}
			currentScreen = MainMenu
			hosting = false
		}
	}
}

func getInfrastructureColor(infraType InfrastructureType) rl.Color {
	switch infraType {
	case Road:
		return rl.Black
	case Water:
		return rl.Blue
	default:
		return rl.Green
	}
}

func getInfrastructureName(infraType InfrastructureType) string {
	switch infraType {
	case Road:
		return "Road"
	case Water:
		return "Water"
	default:
		return "Unknown"
	}
}

func getInfrastructureThickness(infraType InfrastructureType) float32 {
	switch infraType {
	case Road:
		return 4
	case Water:
		return 8
	default:
		return 6
	}
}

func getBuildingColor(buildingType BuildingType) rl.Color {
	switch buildingType {
	case Residential:
		return rl.Green
	case Commercial:
		return rl.Blue
	case Industrial:
		return rl.Red
	default:
		return rl.Gray
	}
}

func getBuildingName(buildingType BuildingType) string {
	switch buildingType {
	case Residential:
		return "Residential"
	case Commercial:
		return "Commercial"
	case Industrial:
		return "Industrial"
	default:
		return "Unknown"
	}
}

func drawGrid() {
	if !showGrid {
		return
	}

	screenWidth := float32(rl.GetScreenWidth())
	screenHeight := float32(rl.GetScreenHeight())

	startX := int((-cameraOffset.X)/GRID_SIZE) - 2
	endX := int((-cameraOffset.X+screenWidth/zoom)/GRID_SIZE) + 2
	startY := int((-cameraOffset.Y+UI_HEIGHT/zoom)/GRID_SIZE) - 2
	endY := int((-cameraOffset.Y+screenHeight/zoom)/GRID_SIZE) + 2

	gridColor := rl.NewColor(200, 200, 200, 100)

	for x := startX; x <= endX; x++ {
		worldX := float32(x * GRID_SIZE)
		screenStart := worldToScreen(rl.NewVector2(worldX, float32(startY*GRID_SIZE)))
		screenEnd := worldToScreen(rl.NewVector2(worldX, float32(endY*GRID_SIZE)))
		rl.DrawLineV(screenStart, screenEnd, gridColor)
	}

	for y := startY; y <= endY; y++ {
		worldY := float32(y * GRID_SIZE)
		screenStart := worldToScreen(rl.NewVector2(float32(startX*GRID_SIZE), worldY))
		screenEnd := worldToScreen(rl.NewVector2(float32(endX*GRID_SIZE), worldY))
		rl.DrawLineV(screenStart, screenEnd, gridColor)
	}
}

func draw() {
	if rl.WindowShouldClose() {
		return
	}
	rl.BeginDrawing()
	rl.ClearBackground(rl.RayWhite)

	switch currentScreen {
	case MainMenu:
		gui.SetFont(titleFont)
		gui.SetStyle(gui.DEFAULT, gui.TEXT_SIZE, 72)
		gui.Label(rl.NewRectangle(350, 200, 400, 80), "Citybuilder")
		gui.SetFont(textFont)
		gui.SetStyle(gui.DEFAULT, gui.TEXT_SIZE, 24)
	case ServerChooser:
		gui.Label(rl.NewRectangle(200, 50, 400, 30), "Server Connection")
		gui.Label(rl.NewRectangle(50, 160, 140, 30), "Your Name:")
		gui.Label(rl.NewRectangle(50, 200, 140, 30), "Server IP:")
		gui.Label(rl.NewRectangle(50, 240, 140, 30), "Port:")
		gui.Label(rl.NewRectangle(200, 300, 400, 30), status)
		nameBox.Draw()
		ipBox.Draw()
		portBox.Draw()
	case InGame:
		drawGrid()

		if client.Connected {
			client.mutex.Lock()

			for _, line := range client.CityLines {
				color := getInfrastructureColor(line.Type)
				thickness := getInfrastructureThickness(line.Type)
				screenStart := worldToScreen(line.Start)
				screenEnd := worldToScreen(line.End)
				rl.DrawLineEx(screenStart, screenEnd, thickness*zoom, color)
				rl.DrawCircleV(screenStart, 4*zoom, color)
				rl.DrawCircleV(screenEnd, 4*zoom, color)
			}

			for _, building := range client.Buildings {
				color := getBuildingColor(building.Type)
				screenPos := worldToScreen(building.Position)
				size := GRID_SIZE * zoom * 0.8
				rect := rl.NewRectangle(screenPos.X-size/2, screenPos.Y-size/2, size, size)
				rl.DrawRectangleRec(rect, color)
				rl.DrawRectangleLinesEx(rect, 2, rl.Black)
			}

			if currentBuildMode == BusRouteMode {
				for _, route := range client.BusRoutes {
					for j := 0; j < len(route.Nodes)-1; j++ {
						screenStart := worldToScreen(route.Nodes[j])
						screenEnd := worldToScreen(route.Nodes[j+1])
						rl.DrawLineEx(screenStart, screenEnd, 2*zoom, rl.Orange)
					}
					for _, node := range route.Nodes {
						screenNode := worldToScreen(node)
						rl.DrawCircleV(screenNode, 6*zoom, rl.Orange)
					}
				}
			}

			for _, bus := range client.Buses {
				busScreenPos := worldToScreen(bus.Position)
				busSize := 8 * zoom
				rect := rl.NewRectangle(busScreenPos.X-busSize/2, busScreenPos.Y-busSize/2,
					busSize, busSize)
				rl.DrawRectangleRec(rect, rl.Orange)
				rl.DrawRectangleLinesEx(rect, zoom, rl.Black)
			}
			client.mutex.Unlock()
		}

		if isBuilding && currentBuildMode == InfrastructureMode {
			mousePos := rl.GetMousePosition()
			if mousePos.Y > UI_HEIGHT {
				worldPos := screenToWorld(mousePos)
				snappedEnd := snapToGrid(worldPos)
				color := getInfrastructureColor(currentInfraType)
				thickness := getInfrastructureThickness(currentInfraType)

				screenStart := worldToScreen(buildStart)
				screenEnd := worldToScreen(snappedEnd)

				rl.DrawLineEx(screenStart, screenEnd, thickness*zoom, rl.NewColor(color.R, color.G, color.B, 128))
			}
		}

		if isCreatingBusRoute {

			for i := 0; i < len(currentRouteNodes); i++ {
				screenNode := worldToScreen(currentRouteNodes[i])
				rl.DrawCircleV(screenNode, 5*zoom, rl.NewColor(255, 165, 0, 128))
				if i > 0 {
					prevScreenNode := worldToScreen(currentRouteNodes[i-1])
					rl.DrawLineEx(prevScreenNode, screenNode, 2*zoom, rl.NewColor(255, 165, 0, 128))
				}
			}

			if len(currentRouteNodes) > 0 {
				mousePos := rl.GetMousePosition()
				if mousePos.Y > UI_HEIGHT {
					worldPos := screenToWorld(mousePos)
					snappedEnd := snapToGrid(worldPos)
					screenStart := worldToScreen(currentRouteNodes[len(currentRouteNodes)-1])
					screenEnd := worldToScreen(snappedEnd)
					rl.DrawLineEx(screenStart, screenEnd, 2*zoom, rl.NewColor(255, 165, 0, 128))
				}
			}
		}

		if client.Connected {
			client.mutex.Lock()
			for _, cursor := range client.OtherCursors {
				screenPos := worldToScreen(cursor.Position)
				rl.DrawCircleV(screenPos, 8*zoom, rl.Red)
				namePos := rl.NewVector2(screenPos.X-30, screenPos.Y-25)
				gui.Label(rl.NewRectangle(namePos.X, namePos.Y-10, 200, 30), cursor.Name)
			}
			client.mutex.Unlock()
		}

		rl.DrawRectangle(0, 0, int32(rl.GetScreenWidth()), UI_HEIGHT, rl.RayWhite)
		rl.DrawLine(0, UI_HEIGHT, int32(rl.GetScreenWidth()), UI_HEIGHT, rl.Black)

		if gui.Button(rl.NewRectangle(10, 10, 150, 25), "Infrastructure") {
			currentBuildMode = InfrastructureMode
		}
		if gui.Button(rl.NewRectangle(170, 10, 80, 25), "Houses") {
			currentBuildMode = BuildingMode
		}
		if gui.Button(rl.NewRectangle(260, 10, 80, 25), "Routes") {
			currentBuildMode = BusRouteMode
		}
		if gui.Button(rl.NewRectangle(350, 10, 80, 25), "Delete") {
			currentBuildMode = DeleteMode
		}

		if currentBuildMode == InfrastructureMode {
			if gui.Button(rl.NewRectangle(10, 40, 60, 25), "Road") {
				currentInfraType = Road
			}
			if gui.Button(rl.NewRectangle(80, 40, 70, 25), "River") {
				currentInfraType = Water
			}
			currentTypeName := getInfrastructureName(currentInfraType)
			currentColor := getInfrastructureColor(currentInfraType)
			gui.Label(rl.NewRectangle(36, 70, 200, 20), "Building: "+currentTypeName)
			rl.DrawRectangle(10, 72, 16, 16, currentColor)
		}

		if currentBuildMode == BuildingMode {
			rect := rl.NewRectangle(10, 72, 16, 16)
			if gui.Button(rl.NewRectangle(10, 40, 120, 25), "Residential") {
				currentBuildingType = Residential
			}
			if gui.Button(rl.NewRectangle(140, 40, 90, 25), "Business") {
				currentBuildingType = Commercial
			}
			if gui.Button(rl.NewRectangle(240, 40, 100, 25), "Industrial") {
				currentBuildingType = Industrial
			}
			currentBuildingName := getBuildingName(currentBuildingType)
			currentBuildingColor := getBuildingColor(currentBuildingType)
			gui.Label(rl.NewRectangle(36, 70, 200, 20), "Building: "+currentBuildingName)
			rl.DrawRectangleRec(rect, currentBuildingColor)
			rl.DrawRectangleLinesEx(rect, 2, rl.Black)
		}

		if currentBuildMode == BusRouteMode {
			gui.Label(rl.NewRectangle(10, 40, 400, 20), "Click to place bus route nodes.")
			if isCreatingBusRoute {

				if gui.Button(rl.NewRectangle(10, 70, 160, 25), "Finish Route") {
					if client.Connected && len(currentRouteNodes) >= 2 {
						client.SendBusRoute(currentRouteNodes)
					}
					isCreatingBusRoute = false
					currentRouteNodes = []rl.Vector2{}
				}

				if gui.Button(rl.NewRectangle(180, 70, 150, 25), "Cancel Route") {
					isCreatingBusRoute = false
					currentRouteNodes = []rl.Vector2{}
				}
			}
		}

		if currentBuildMode == DeleteMode {
			gui.Label(rl.NewRectangle(10, 40, 400, 20), "Click near objects to delete them.")
		}

		gui.Label(rl.NewRectangle(float32(rl.GetScreenWidth()-620), 95, 610, 20), "WASD / Arrows: Move | Mouse Wheel: Zoom | G: Grid | ESC: Menu")
		zoomText := fmt.Sprintf("Zoom: %.1fx", zoom)
		gui.Label(rl.NewRectangle(float32(rl.GetScreenWidth()-120), 10, 100, 20), zoomText)

		moneyText := fmt.Sprintf("Money: $%.2f", client.Money)
		gui.Label(rl.NewRectangle(10, 95, 400, 20), moneyText)

	}
	rl.EndDrawing()
}
