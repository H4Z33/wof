// Package rules holds progression formulas shared by character creation and
// the simulation. Numbers are placeholders until the balance pass (M6).
package rules

import "math/rand/v2"

const MaxLevel = 50

// MaxHP is a player's maximum health at a level.
func MaxHP(level int) int { return 100 + 20*(level-1) }

// ExpToNext is the experience needed to go from level to level+1.
func ExpToNext(level int) int64 { return int64(100 * level) }

// DamageMultiplier scales player skill damage with level.
func DamageMultiplier(level int) float64 { return 1 + 0.1*float64(level-1) }

// RollLuck gives a new character 3d6+1 luck (4-19, average 11.5): most are
// ordinary, a few are blessed or cursed from birth.
func RollLuck() int { return rand.IntN(6) + rand.IntN(6) + rand.IntN(6) + 4 }
