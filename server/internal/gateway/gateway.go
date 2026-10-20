// Package gateway owns client WebSocket connections: it authenticates the
// Hello, rate-limits and decodes client frames, and forwards them to the
// zone. It contains no game logic.
package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/auth"
	"convergence/server/internal/model"
	"convergence/server/internal/zone"
)

const (
	helloTimeout = 5 * time.Second
	writeTimeout = 5 * time.Second
	maxFrame     = 4096
	outBuffer    = 128
	// Clients send 20 inputs/s plus occasional pings.
	msgRate  = 40
	msgBurst = 80
)

type Gateway struct {
	tickets *auth.Store[model.Character]
	zone    *zone.Zone
	log     *slog.Logger
}

func New(tickets *auth.Store[model.Character], z *zone.Zone, log *slog.Logger) *Gateway {
	return &Gateway{tickets: tickets, zone: z, log: log.With("component", "gateway")}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Auth is the ticket inside Hello, not cookies, so cross-origin
	// connections are harmless.
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	ws.SetReadLimit(maxFrame)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	c := newConn(ws)
	go c.writeLoop(ctx)
	defer c.Close("bye")

	h, err := g.handshake(ctx, c)
	if err != nil {
		g.log.Info("handshake failed", "remote", r.RemoteAddr, "err", err)
		return
	}
	defer h.Leave()

	lim := rate.NewLimiter(msgRate, msgBurst)
	for {
		typ, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageBinary {
			c.Close("binary frames only")
			return
		}
		if !lim.Allow() {
			c.sendError("rate_limited", "too many messages")
			c.Close("rate limited")
			return
		}
		var msg pb.ClientMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			c.Close("malformed message")
			return
		}
		switch p := msg.Payload.(type) {
		case *pb.ClientMessage_Move:
			h.Input(p.Move)
		case *pb.ClientMessage_UseSkill:
			h.UseSkill(p.UseSkill)
		case *pb.ClientMessage_RaiseAttribute:
			h.RaiseAttribute(p.RaiseAttribute.Key)
		case *pb.ClientMessage_ChooseSpec:
			h.ChooseSpec(p.ChooseSpec.Key)
		case *pb.ClientMessage_ChooseTitle:
			h.ChooseTitle(p.ChooseTitle.Key)
		case *pb.ClientMessage_Interact:
			h.Interact(p.Interact.NpcId)
		case *pb.ClientMessage_QuestAction:
			h.QuestAction(p.QuestAction)
		case *pb.ClientMessage_Ping:
			c.sendMsg(&pb.ServerMessage{Payload: &pb.ServerMessage_Pong{Pong: &pb.Pong{
				ClientTimeMs: p.Ping.ClientTimeMs,
				ServerTick:   g.zone.Tick(),
			}}})
		}
	}
}

func (g *Gateway) handshake(ctx context.Context, c *conn) (*zone.Handle, error) {
	hctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	typ, data, err := c.ws.Read(hctx)
	if err != nil {
		return nil, err
	}
	var msg pb.ClientMessage
	if typ != websocket.MessageBinary || proto.Unmarshal(data, &msg) != nil || msg.GetHello() == nil {
		c.sendError("bad_hello", "first message must be Hello")
		return nil, errors.New("bad hello")
	}
	hello := msg.GetHello()
	if hello.ProtocolVersion != uint32(pb.ProtocolVersion_PROTOCOL_VERSION_CURRENT) {
		c.sendError("protocol_mismatch", "client is out of date")
		return nil, errors.New("protocol mismatch")
	}
	char, ok := g.tickets.Redeem(hello.Token)
	if !ok {
		c.sendError("bad_token", "play ticket invalid or expired")
		return nil, errors.New("bad token")
	}
	h, err := g.zone.Join(ctx, char, c)
	if err != nil {
		c.sendError("join_failed", err.Error())
		return nil, err
	}
	return h, nil
}

// conn implements zone.Conn on top of a WebSocket. Sends are queued and
// written by a single goroutine; a client that cannot keep up is dropped
// rather than slowing down the zone.
type conn struct {
	ws     *websocket.Conn
	out    chan []byte
	done   chan struct{}
	once   sync.Once
	reason string
}

func newConn(ws *websocket.Conn) *conn {
	return &conn{ws: ws, out: make(chan []byte, outBuffer), done: make(chan struct{})}
}

func (c *conn) Send(b []byte) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.out <- b:
		return true
	default:
		c.Close("send buffer full")
		return false
	}
}

func (c *conn) Close(reason string) {
	c.once.Do(func() {
		c.reason = reason
		close(c.done)
	})
}

func (c *conn) sendMsg(m *pb.ServerMessage) {
	if b, err := proto.Marshal(m); err == nil {
		c.Send(b)
	}
}

func (c *conn) sendError(code, text string) {
	c.sendMsg(&pb.ServerMessage{Payload: &pb.ServerMessage_Error{Error: &pb.ServerError{Code: code, Message: text}}})
}

func (c *conn) writeLoop(ctx context.Context) {
	for {
		select {
		case b := <-c.out:
			if !c.write(ctx, b) {
				c.Close("write failed")
				c.ws.CloseNow()
				return
			}
		case <-ctx.Done():
			c.Close("connection ended")
			c.ws.CloseNow()
			return
		case <-c.done:
			// Flush what is already queued (e.g. a final Error) before closing.
			for {
				select {
				case b := <-c.out:
					if !c.write(ctx, b) {
						c.ws.CloseNow()
						return
					}
				default:
					c.ws.Close(websocket.StatusNormalClosure, c.reason)
					return
				}
			}
		}
	}
}

func (c *conn) write(ctx context.Context, b []byte) bool {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return c.ws.Write(wctx, websocket.MessageBinary, b) == nil
}
