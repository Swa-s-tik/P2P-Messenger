package src

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const defaultuser = "newuser"
const defaultroom = "lobby"

type ChatRoom struct {
	Host     *P2P
	Inbound  chan chatmessage
	Outbound chan string
	Logs     chan chatlog
	RoomName string
	UserName string
	selfid   peer.ID

	psctx    context.Context
	pscancel context.CancelFunc

	pstopic *pubsub.Topic

	psub *pubsub.Subscription
}

type chatmessage struct {
	Message    string `json:"message"`
	SenderID   string `json:"senderid"`
	SenderName string `json:"sendername"`
}

type chatlog struct {
	logprefix string
	logmsg    string
}

func JoinChatRoom(p2phost *P2P, username string, roomname string) (*ChatRoom, error) {
	topic, err := p2phost.PubSub.Join(fmt.Sprintf("room-peerchat-%s", roomname))
	if err != nil {
		return nil, err
	}
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}
	if username == "" {
		username = defaultuser
	}
	if roomname == "" {
		roomname = defaultroom
	}

	pubsubctx, cancel := context.WithCancel(context.Background())

	chatroom := &ChatRoom{
		Host: p2phost,

		Inbound:  make(chan chatmessage),
		Outbound: make(chan string),
		Logs:     make(chan chatlog),

		psctx:    pubsubctx,
		pscancel: cancel,
		pstopic:  topic,
		psub:     sub,

		RoomName: roomname,
		UserName: username,
		selfid:   p2phost.Host.ID(),
	}
	go chatroom.SubLoop()
	go chatroom.PubLoop()
	return chatroom, nil
}
func (cr *ChatRoom) PubLoop() {
	for {
		select {
		case <-cr.psctx.Done():
			return

		case message := <-cr.Outbound:
			m := chatmessage{
				Message:    message,
				SenderID:   cr.selfid.Pretty(),
				SenderName: cr.UserName,
			}
			messagebytes, err := json.Marshal(m)
			if err != nil {
				cr.Logs <- chatlog{logprefix: "puberr", logmsg: "could not marshal JSON"}
				continue
			}
			err = cr.pstopic.Publish(cr.psctx, messagebytes)
			if err != nil {
				cr.Logs <- chatlog{logprefix: "puberr", logmsg: "could not publish to topic"}
				continue
			}
		}
	}
}

func (cr *ChatRoom) SubLoop() {
	for {
		select {
		case <-cr.psctx.Done():
			return

		default:
			message, err := cr.psub.Next(cr.psctx)
			// Check error
			if err != nil {
				close(cr.Inbound)
				cr.Logs <- chatlog{logprefix: "suberr", logmsg: "subscription has closed"}
				return
			}

			if message.ReceivedFrom == cr.selfid {
				continue
			}
			cm := &chatmessage{}
			err = json.Unmarshal(message.Data, cm)
			if err != nil {
				cr.Logs <- chatlog{logprefix: "suberr", logmsg: "could not unmarshal JSON"}
				continue
			}

			cr.Inbound <- *cm
		}
	}
}

func (cr *ChatRoom) PeerList() []peer.ID {
	return cr.pstopic.ListPeers()
}

func (cr *ChatRoom) Exit() {
	defer cr.pscancel()
	cr.psub.Cancel()
	cr.pstopic.Close()
}
func (cr *ChatRoom) UpdateUser(username string) {
	cr.UserName = username
}
