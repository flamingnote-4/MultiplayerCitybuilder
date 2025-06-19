package main

import rl "github.com/gen2brain/raylib-go/raylib"

const (
	GRID_SIZE                  = 32
	UI_HEIGHT                  = 120
	ROAD_COST_PER_UNIT         = 0.5
	BUS_SPEED                  = 200.0
	ROAD_SNAP_DISTANCE         = 8.0
	RESIDENTIAL_BUILDING_COST  = 100.0
	COMMERCIAL_BUILDING_COST   = 250.0
	INDUSTRIAL_BUILDING_COST   = 1000.0
	COMMERCIAL_INCOME_INCREASE = 5.0
	INDUSTRIAL_INCOME_INCREASE = 25.0
)

type InfrastructureType int

const (
	Road InfrastructureType = iota
	Water
)

type BuildingType int

const (
	Residential BuildingType = iota
	Commercial
	Industrial
)

type Building struct {
	Position rl.Vector2
	Type     BuildingType
	PlayerID string
}

type BusRoute struct {
	Nodes    []rl.Vector2
	PlayerID string
	Length   float32
}

type Bus struct {
	Position       rl.Vector2
	RouteID        int
	CurrentSegment int
	Progress       float32
	Direction      int
}
