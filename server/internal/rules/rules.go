// Package rules holds progression formulas shared by character creation and
// the simulation. Numbers are placeholders until the balance pass (M6).
package rules

const MaxLevel = 50

// MaxHP is a player's maximum health at a level.
func MaxHP(level int) int { return 100 + 20*(level-1) }

// ExpToNext is the experience needed to go from level to level+1.
func ExpToNext(level int) int64 { return int64(100 * level) }

// DamageMultiplier scales player skill damage with level.
func DamageMultiplier(level int) float64 { return 1 + 0.1*float64(level-1) }
