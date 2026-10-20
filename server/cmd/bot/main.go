// Command bot runs headless clients against a server. Each bot registers (or
// logs in), creates a character if it has none, joins the world and either
// walks randomly or, with -fight, hunts the nearest NPC with its skills.
// Used for protocol regression and load tests.
//
//	go run ./cmd/bot -n 50 -duration 20s -fight
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	pb "convergence/server/gen/convergence/v1"
)

var prefix string

type result struct {
	name      string
	err       error
	snapshots int
	seen      int
	ackLag    uint32
	hits      int // damage events this bot caused
	kills     int
	deaths    int
	rejected  map[string]int
	level     uint32
	fates     map[string]int // fate actions received
}

func main() {
	base := flag.String("server", "http://localhost:7777", "server base URL")
	n := flag.Int("n", 10, "number of bots")
	duration := flag.Duration("duration", 15*time.Second, "how long each bot plays")
	fight := flag.Bool("fight", false, "hunt NPCs instead of walking randomly")
	flag.StringVar(&prefix, "prefix", "bot", "account/character name prefix (new prefix = fresh characters)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *duration+60*time.Second)
	defer cancel()

	results := make([]result, *n)
	var wg sync.WaitGroup
	for i := range *n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = runBot(ctx, *base, i, *duration, *fight)
		}()
		time.Sleep(20 * time.Millisecond) // stagger logins
	}
	wg.Wait()

	failed, totalHits, totalKills := 0, 0, 0
	totalFates := map[string]int{}
	minSnaps := int(duration.Seconds() * 20 * 0.9)
	for _, r := range results {
		status := "ok"
		switch {
		case r.err != nil:
			status = "error: " + r.err.Error()
		case r.snapshots < minSnaps:
			status = fmt.Sprintf("only %d snapshots, want >= %d", r.snapshots, minSnaps)
		case r.ackLag > 2:
			status = fmt.Sprintf("ack lag %d", r.ackLag)
		}
		if status != "ok" {
			failed++
		}
		totalHits += r.hits
		totalKills += r.kills
		for k, v := range r.fates {
			totalFates[k] += v
		}
		fmt.Printf("%s snapshots=%d seen=%d ack_lag=%d hits=%d kills=%d deaths=%d level=%d rejected=%v %s\n",
			r.name, r.snapshots, r.seen, r.ackLag, r.hits, r.kills, r.deaths, r.level, r.rejected, status)
	}
	if *fight && totalHits == 0 {
		fmt.Println("FAIL: fight mode but no bot landed a hit")
		failed++
	}
	fmt.Printf("%d/%d bots passed, %d hits, %d kills, fate events %v\n", *n-failed, *n, totalHits, totalKills, totalFates)
	if failed > 0 {
		os.Exit(1)
	}
}

// api calls a JSON endpoint and decodes the reply into out.
func api(ctx context.Context, method, u, session string, body, out any) (int, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

type character struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// enterWorld returns a play ticket and the WebSocket path.
func enterWorld(ctx context.Context, base string, i int) (string, string, error) {
	user := fmt.Sprintf("%s_%03d", prefix, i)
	const password = "bot-password-123"
	if code, err := api(ctx, "POST", base+"/api/v1/register", "", map[string]string{"username": user, "password": password}, nil); err != nil {
		return "", "", err
	} else if code != http.StatusCreated && code != http.StatusConflict {
		return "", "", fmt.Errorf("register: HTTP %d", code)
	}
	var login struct {
		Session    string      `json:"session"`
		Characters []character `json:"characters"`
	}
	if code, err := api(ctx, "POST", base+"/api/v1/login", "", map[string]string{"username": user, "password": password}, &login); err != nil || code != http.StatusOK {
		return "", "", fmt.Errorf("login: HTTP %d %v", code, err)
	}
	if len(login.Characters) == 0 {
		models := []string{"mannequin_m", "mannequin_f", "zuo_mo"}
		classes := []string{"sword_cultivator", "rail_gunner", "mystic"}
		var created struct {
			Character character `json:"character"`
		}
		body := map[string]string{"name": fmt.Sprintf("%s%03d", strings.ToUpper(prefix[:1])+prefix[1:], i), "model": models[i%len(models)], "class": classes[i%len(classes)]}
		if code, err := api(ctx, "POST", base+"/api/v1/characters", login.Session, body, &created); err != nil || code != http.StatusCreated {
			return "", "", fmt.Errorf("create character: HTTP %d %v", code, err)
		}
		login.Characters = append(login.Characters, created.Character)
	}
	var play struct {
		Ticket string `json:"ticket"`
		WSPath string `json:"ws_path"`
	}
	if code, err := api(ctx, "POST", base+"/api/v1/play", login.Session, map[string]int64{"character_id": login.Characters[0].ID}, &play); err != nil || code != http.StatusOK {
		return "", "", fmt.Errorf("play: HTTP %d %v", code, err)
	}
	return play.Ticket, play.WSPath, nil
}

type seenEntity struct {
	kind  pb.EntityKind
	x, z  float64
	alive bool
}

func runBot(ctx context.Context, base string, i int, duration time.Duration, fight bool) (res result) {
	res = result{name: fmt.Sprintf("bot_%03d", i), rejected: map[string]int{}, fates: map[string]int{}}
	ticket, wsPath, err := enterWorld(ctx, base, i)
	if err != nil {
		res.err = err
		return
	}

	u, _ := url.Parse(base)
	u.Scheme = map[string]string{"http": "ws", "https": "wss"}[u.Scheme]
	u.Path = wsPath
	ws, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		res.err = err
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(1 << 20)

	var writeMu sync.Mutex
	send := func(m *pb.ClientMessage) error {
		b, _ := proto.Marshal(m)
		writeMu.Lock()
		defer writeMu.Unlock()
		return ws.Write(ctx, websocket.MessageBinary, b)
	}
	if err := send(&pb.ClientMessage{Payload: &pb.ClientMessage_Hello{Hello: &pb.Hello{
		Token: ticket, ProtocolVersion: uint32(pb.ProtocolVersion_PROTOCOL_VERSION_CURRENT),
	}}}); err != nil {
		res.err = err
		return
	}

	var mu sync.Mutex
	var skills []*pb.SkillInfo // enemy-target skills from Welcome
	var myID, lastAck uint32
	var myX, myZ float64
	var dead bool
	entities := map[uint32]*seenEntity{}
	everSeen := map[uint32]bool{}
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
			case *pb.ServerMessage_Welcome:
				myID = p.Welcome.EntityId
				for _, sk := range p.Welcome.Skills {
					if !sk.TargetSelf {
						skills = append(skills, sk)
					}
				}
			case *pb.ServerMessage_Snapshot:
				s := p.Snapshot
				res.snapshots++
				lastAck = s.AckInputSeq
				if s.Status != nil {
					res.level = s.Status.Level
					dead = s.Status.Hp == 0
				}
				apply := func(st *pb.EntityState, kind pb.EntityKind) {
					e := entities[st.Id]
					if e == nil {
						e = &seenEntity{kind: kind}
						entities[st.Id] = e
					}
					e.x, e.z, e.alive = float64(st.Pos.GetX()), float64(st.Pos.GetZ()), st.Hp > 0
					if st.Id == myID {
						myX, myZ = e.x, e.z
					}
				}
				for _, sp := range s.Spawns {
					everSeen[sp.Id] = true
					apply(sp.State, sp.Kind)
				}
				for _, st := range s.Entities {
					if entities[st.Id] != nil {
						apply(st, entities[st.Id].kind)
					}
				}
				for _, id := range s.Despawns {
					delete(entities, id)
				}
				for _, ev := range s.Events {
					switch {
					case ev.Kind == pb.CombatEventKind_COMBAT_EVENT_KIND_DAMAGE && ev.SourceId == myID:
						res.hits++
					case ev.Kind == pb.CombatEventKind_COMBAT_EVENT_KIND_DEATH && ev.SourceId == myID:
						res.kills++
					case ev.Kind == pb.CombatEventKind_COMBAT_EVENT_KIND_DEATH && ev.TargetId == myID:
						res.deaths++
					}
				}
			case *pb.ServerMessage_Fate:
				res.fates[p.Fate.Action]++
			case *pb.ServerMessage_SkillRejected:
				res.rejected[p.SkillRejected.Reason]++
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
	ready := map[uint32]time.Time{}
	for {
		select {
		case err := <-readErr:
			if errors.Is(err, context.Canceled) {
				return
			}
			res.err = err
			return
		case <-deadline:
			time.Sleep(500 * time.Millisecond) // let the last inputs be acknowledged
			mu.Lock()
			res.ackLag = seq - lastAck
			res.seen = len(everSeen)
			mu.Unlock()
			ws.Close(websocket.StatusNormalClosure, "done")
			return
		case <-tick.C:
			mu.Lock()
			dirX, dirZ := math.Cos(angle), math.Sin(angle)
			var skill, target uint32
			if fight && !dead {
				// Hunt the nearest living NPC.
				best, bestD := uint32(0), math.Inf(1)
				for id, e := range entities {
					if e.kind == pb.EntityKind_ENTITY_KIND_NPC && e.alive {
						if d := math.Hypot(e.x-myX, e.z-myZ); d < bestD {
							best, bestD = id, d
						}
					}
				}
				if best != 0 {
					e := entities[best]
					dirX, dirZ = (e.x-myX)/bestD, (e.z-myZ)/bestD
					// Close in to the basic attack's range, then use whatever is ready.
					if len(skills) > 0 && bestD <= float64(skills[0].Range)*0.8 {
						dirX, dirZ = 0, 0
					}
					now := time.Now()
					for _, sk := range skills {
						if bestD <= float64(sk.Range) && now.After(ready[sk.Id]) {
							skill, target = sk.Id, best
							ready[sk.Id] = now.Add(time.Duration(sk.CooldownMs+100) * time.Millisecond)
							break
						}
					}
				}
			}
			mu.Unlock()
			if !fight {
				angle += (rand.Float64() - 0.5) * 0.6
			}
			seq++
			if err := send(&pb.ClientMessage{Payload: &pb.ClientMessage_Move{Move: &pb.MoveInput{
				Seq: seq, DirX: float32(dirX), DirZ: float32(dirZ), Yaw: float32(math.Atan2(dirX, dirZ)),
			}}}); err != nil {
				res.err = err
				return
			}
			if skill != 0 {
				if err := send(&pb.ClientMessage{Payload: &pb.ClientMessage_UseSkill{UseSkill: &pb.UseSkill{SkillId: skill, TargetId: target}}}); err != nil {
					res.err = err
					return
				}
			}
		}
	}
}
