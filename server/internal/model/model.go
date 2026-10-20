// Package model holds plain data types shared by the database, the HTTP API
// and the simulation, so those packages do not depend on each other.
package model

// Character is a playable character as stored in the database.
type Character struct {
	ID        int64
	AccountID int64
	Name      string
	Model     string
	Class     string
	ZoneID    string
	Pos       [3]float64
	Yaw       float64
	Level     int
	Exp       int64
	HP        int
	// Luck (1-20) is fixed at creation; karma (-100..100) changes with deeds.
	Luck  int
	Karma int
	// Item key -> quantity.
	Inventory map[string]int
	// Disciplines and build.
	Progression Progression
}

// Progression is a character's build: unspent discipline experience,
// attribute ranks and the chosen specialization.
type Progression struct {
	XP    map[string]int64 `json:"xp,omitempty"`
	Ranks map[string]int   `json:"ranks,omitempty"`
	Spec  string           `json:"spec,omitempty"`
}
