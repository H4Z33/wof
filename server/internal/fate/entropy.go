package fate

// Entropy is a per-player random stream fed by the player's own inputs
// (movement, skills, timing), so every roll depends on what they actually
// did. It starts from a server secret: without it a player who knew the
// algorithm could choose inputs to steer fate.
type Entropy struct{ state uint64 }

func NewEntropy(secret uint64) Entropy { return Entropy{state: splitmix64(secret)} }

// Mix folds values into the stream.
func (e *Entropy) Mix(vals ...uint64) {
	for _, v := range vals {
		e.state = splitmix64(e.state ^ v)
	}
}

// Float returns a number in [0, 1).
func (e *Entropy) Float() float64 {
	e.state = splitmix64(e.state)
	return float64(e.state>>11) / (1 << 53)
}

// IntN returns a number in [0, n).
func (e *Entropy) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	return int(e.Float() * float64(n))
}

// splitmix64 is a fast, well-distributed 64-bit mixer (Steele et al.).
func splitmix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}
