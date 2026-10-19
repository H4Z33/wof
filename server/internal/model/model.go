// Package model holds plain data types shared by the database, the HTTP API
// and the simulation, so those packages do not depend on each other.
package model

// Character is a playable character as stored in the database.
type Character struct {
	ID        int64
	AccountID int64
	Name      string
	Model     string
	ZoneID    string
	Pos       [3]float64
	Yaw       float64
	Level     int
	Exp       int64
	HP        int
}
