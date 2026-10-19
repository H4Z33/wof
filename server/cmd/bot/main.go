// Command bot runs headless clients against a server: each bot logs in, walks
// randomly and checks that it sees every other bot. Used for protocol
// regression and load tests.
//
//	go run ./cmd/bot -n 50 -duration 20s
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	pb "convergence/server/gen/convergence/v1"
)

type result struct {
	name      string
	err       error
	snapshots int
	seen      map[uint32]bool
	ackLag    uint32 // inputs sent but not yet acknowledged at the end
}

func main() {
	base := flag.String("server", "http://localhost:7777", "server base URL")
	n := flag.Int("n", 10, "number of bots")
	duration := flag.Duration("duration", 15*time.Second, "how long each bot walks")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *duration+30*time.Second)
	defer cancel()

	results := make([]result, *n)
	var wg sync.WaitGroup
	for i := range *n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = runBot(ctx, *base, fmt.Sprintf("bot_%03d", i), *duration)
		}()
		time.Sleep(20 * time.Millisecond) // stagger logins
	}
	wg.Wait()

	failed := 0
	for _, r := range results {
		status := "ok"
		switch {
		case r.err != nil:
			status = "error: " + r.err.Error()
		case len(r.seen) < *n:
			status = fmt.Sprintf("saw only %d/%d entities", len(r.seen), *n)
		}
		if status != "ok" {
			failed++
		}
		fmt.Printf("%s snapshots=%d seen=%d ack_lag=%d %s\n", r.name, r.snapshots, len(r.seen), r.ackLag, status)
	}
	fmt.Printf("%d/%d bots passed\n", *n-failed, *n)
	if failed > 0 {
		os.Exit(1)
	}
}

func runBot(ctx context.Context, base, name string, duration time.Duration) (res result) {
	res = result{name: name, seen: map[uint32]bool{}}

	models := []string{"mannequin_m", "mannequin_f", "zuo_mo"}
	body, _ := json.Marshal(map[string]string{"name": name, "model": models[rand.IntN(len(models))]})
	resp, err := http.Post(base+"/api/v1/dev-login", "application/json", bytes.NewReader(body))
	if err != nil {
		res.err = err
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		res.err = fmt.Errorf("login: %s", resp.Status)
		return
	}
	var login struct {
		Token  string `json:"token"`
		WSPath string `json:"ws_path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&login); err != nil {
		res.err = err
		return
	}

	u, _ := url.Parse(base)
	u.Scheme = map[string]string{"http": "ws", "https": "wss"}[u.Scheme]
	u.Path = login.WSPath
	ws, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		res.err = err
		return
	}
	defer ws.CloseNow()

	send := func(m *pb.ClientMessage) error {
		b, _ := proto.Marshal(m)
		return ws.Write(ctx, websocket.MessageBinary, b)
	}
	if err := send(&pb.ClientMessage{Payload: &pb.ClientMessage_Hello{Hello: &pb.Hello{
		Token:           login.Token,
		ProtocolVersion: uint32(pb.ProtocolVersion_PROTOCOL_VERSION_CURRENT),
	}}}); err != nil {
		res.err = err
		return
	}

	var mu sync.Mutex
	var lastAck uint32
	readErr := make(chan error, 1)
	go func() {
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				readErr <- err
				return
			}
			var msg pb.ServerMessage
			if err := proto.Unmarshal(data, &msg); err != nil {
				readErr <- err
				return
			}
			mu.Lock()
			switch p := msg.Payload.(type) {
			case *pb.ServerMessage_Snapshot:
				res.snapshots++
				lastAck = p.Snapshot.AckInputSeq
				for _, s := range p.Snapshot.Spawns {
					res.seen[s.Id] = true
				}
			case *pb.ServerMessage_Error:
				readErr <- fmt.Errorf("server error %s: %s", p.Error.Code, p.Error.Message)
			}
			mu.Unlock()
		}
	}()

	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(duration)
	angle := rand.Float64() * 2 * math.Pi
	var seq uint32
	for {
		select {
		case err := <-readErr:
			res.err = err
			return
		case <-deadline:
			// Give the last inputs time to be acknowledged, then report.
			time.Sleep(500 * time.Millisecond)
			mu.Lock()
			res.ackLag = seq - lastAck
			res.seen = maps.Clone(res.seen)
			mu.Unlock()
			ws.Close(websocket.StatusNormalClosure, "done")
			return
		case <-tick.C:
			angle += (rand.Float64() - 0.5) * 0.6
			seq++
			if err := send(&pb.ClientMessage{Payload: &pb.ClientMessage_Move{Move: &pb.MoveInput{
				Seq:  seq,
				DirX: float32(math.Cos(angle)),
				DirZ: float32(math.Sin(angle)),
				Yaw:  float32(angle),
			}}}); err != nil {
				res.err = err
				return
			}
		}
	}
}
